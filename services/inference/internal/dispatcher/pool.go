package dispatcher

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"
)

type InferenceRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

type InferenceResult struct {
	Text       string
	Err        error
	Provider   string
	TokenUsage int // fill if you track it

	StartedAt  time.Time
	FinishedAt time.Time

	QueueWait time.Duration
	ExecTime  time.Duration
}

type InferenceJob struct {
	Req        InferenceRequest
	Ctx        context.Context
	ReplyCh    chan InferenceResult
	EnqueuedAt time.Time
}

// ProviderFunc lets you swap real providers / stubs / test doubles.
type ProviderFunc func(ctx context.Context, req InferenceRequest) (text, provider string, tokenUsage int, err error)

type Server struct {
	// Concurrency / lifecycle
	jobQueue chan InferenceJob
	wg       sync.WaitGroup

	// Transport
	client *http.Client

	// Request behavior
	provider ProviderFunc
}

type QueueStats struct {
	Len int
	Cap int
}

func (s *Server) QueueStats() QueueStats {
	return QueueStats{Len: len(s.jobQueue), Cap: cap(s.jobQueue)}
}

var ErrQueueFull = errors.New("queue full")

// TryEnqueue enforces backpressure and returns queue stats for observability.
func (s *Server) TryEnqueue(job InferenceJob) (QueueStats, error) {
	select {
	case s.jobQueue <- job:
		return s.QueueStats(), nil
	default:
		return s.QueueStats(), ErrQueueFull
	}
}

func New(queueSize, workers int, provider ProviderFunc) *Server {
	if provider == nil {
		panic("provider must not be nil")
	}

	s := &Server{
		jobQueue: make(chan InferenceJob, queueSize),
		client: &http.Client{
			Timeout: 60 * time.Second, // upper bound; prefer per-request ctx too
		},
		provider: provider,
	}

	// Start workers
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
	return s
}

func (s *Server) worker(id int) {
	defer s.wg.Done()

	for job := range s.jobQueue {
		startedAt := time.Now()
		queueWait := startedAt.Sub(job.EnqueuedAt)

		// Respect cancellation before starting work
		select {
		case <-job.Ctx.Done():
			job.ReplyCh <- InferenceResult{Err: job.Ctx.Err()}
			continue
		default:
		}

		// Provider call (stubbed here)
		text, provider, tokenUsage, err := s.provider(job.Ctx, job.Req)

		finishedAt := time.Now()

		if err != nil {
			// Note: reqID currently lives in handler; easiest is to include it in the job.
			// Minimal version: just log without reqID:
			log.Printf(`msg="provider call failed" worker=%d model=%q err=%q`, id, job.Req.Model, err.Error())
		}

		job.ReplyCh <- InferenceResult{
			Text:       text,
			Provider:   provider,
			TokenUsage: tokenUsage,
			Err:        err,

			StartedAt:  startedAt,
			FinishedAt: finishedAt,

			QueueWait: queueWait,
			ExecTime:  finishedAt.Sub(startedAt),
		}
	}
}

func (s *Server) Shutdown() {
	close(s.jobQueue) // stop workers
	s.wg.Wait()
}
