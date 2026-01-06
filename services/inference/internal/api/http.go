package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/dispatcher"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/obs"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/search"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/voting"
	// "github.com/Tiger-Du/CrowdAudit/services/inference/internal/authmw"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/search_conversations"
)

func logf(reqID, msg string, args ...any) {
	// Example: req_id=abc123 msg="queue full" queue_size=200
	log.Printf("req_id=%s "+msg, append([]any{reqID}, args...)...)
}

func newReqID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func incReq(status int, provider, model string) {
	if provider == "" {
		provider = "unknown"
	}
	if model == "" {
		model = "unknown"
	}
	obs.InferRequests.WithLabelValues(
		fmt.Sprintf("%d", status),
		provider,
		model,
	).Inc()
}

type HTTP struct {
	S              *dispatcher.Server
	V              *voting.Service
	requestTimeout time.Duration
	Search         *search.Service

	inferMW   func(http.Handler) http.Handler // optional
	Community *search_conversations.CommunityService
}

type Option func(*HTTP)

func WithRequestTimeout(d time.Duration) Option {
	return func(h *HTTP) { h.requestTimeout = d }
}

func WithVoting(v *voting.Service) Option {
	return func(h *HTTP) { h.V = v }
}

func WithSearch(svc *search.Service) Option {
	return func(h *HTTP) { h.Search = svc }
}

func WithInferMiddleware(mw func(http.Handler) http.Handler) Option {
	return func(h *HTTP) { h.inferMW = mw }
}

func New(s *dispatcher.Server, opts ...Option) *HTTP {
	h := &HTTP{
		S:              s,
		requestTimeout: 120 * time.Second, // default
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *HTTP) Routes() http.Handler {
	mux := http.NewServeMux()

	// Infra endpoints (root-level)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/metrics", promhttp.Handler()) // scrape endpoint

	// --- Auth middleware (provider-agnostic OIDC) ---
	// auth, err := authmw.Middleware(authmw.Config{
	// 	Providers: []authmw.ProviderConfig{
	// 		// Auth0 / Okta (Auth0-style issuer)
	// 		{
	// 			Name:     "auth0",
	// 			Issuer:   "https://YOUR_TENANT.us.auth0.com/",
	// 			Audience: "https://api.crowdaudit.example", // your API identifier in Auth0
	// 		},
	// 		// Cognito
	// 		{
	// 			Name:     "cognito",
	// 			Issuer:   "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXXXXXX",
	// 			Audience: "YOUR_COGNITO_APP_CLIENT_ID", // commonly the client id your access token is minted for
	// 		},
	// 	},
	// 	Optional: false,
	// })
	// if err != nil {
	// 	panic(err)
	// }

	// API endpoints (namespaced)
	if h.S != nil {
		infer := http.Handler(http.HandlerFunc(h.handleInfer))
		if h.inferMW != nil {
			infer = h.inferMW(infer)
		}
		mux.Handle("POST /api/infer", infer)
	}
	// mux.Handle("POST /api/infer", auth(http.Handler(infer)))

	if h.V != nil {
		mux.HandleFunc("GET /api/pairs/random", h.handleGetRandomPair)
		mux.HandleFunc("POST /api/votes", h.handleCreateVote)
		// mux.Handle("GET /api/pairs/random", auth(http.HandlerFunc(h.handleGetRandomPair)))
		// mux.Handle("POST /api/votes", auth(http.HandlerFunc(h.handleCreateVote)))
	}

	if h.Search != nil {
		mux.HandleFunc("GET /api/search/pairs", h.handleSearchPairs)
	}

	if h.Community != nil {
		mux.HandleFunc("GET /api/community/conversations", h.handleGetCommunityConversations)
		mux.HandleFunc("POST /api/community/conversations/vote", h.handleVoteCommunityConversation)
	}

	return mux
}

///////////////////////////////////////////////////////////////////////////////

// Handlers

// type CommunityConversation struct {
// 	PairID    int64     `json:"pair_id"`
// 	Title     string    `json:"title"`
// 	Prompt    string    `json:"prompt"`
// 	CreatedAt time.Time `json:"created_at"`
// 	// Add whatever you want to show in the feed:
// 	// vote counts, model names, etc.
// }

// type CommunityConversationsResponse struct {
// 	Items      []CommunityConversation `json:"items"`
// 	NextCursor string                 `json:"next_cursor,omitempty"`
// }

// type CommunityService interface {
// 	ListCommunityConversations(ctx context.Context, cursor string, limit int) (CommunityConversationsResponse, error)
// }

func (h *HTTP) handleGetCommunityConversations(w http.ResponseWriter, r *http.Request) {
	reqID := newReqID()

	log.Printf(
		"[community] start req_id=%s path=%s raw_query=%s",
		reqID,
		r.URL.Path,
		r.URL.RawQuery,
	)

	if h.Community == nil {
		log.Printf("[community] req_id=%s ERROR community service is nil", reqID)
		http.Error(w, "community disabled", http.StatusNotImplemented)
		return
	}

	log.Printf(
		"[community] req_id=%s community_db_nil=%v",
		reqID,
		h.Community.DB == nil,
	)

	ctx := r.Context()

	cursor := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	limit := 20
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil {
			limit = n
		}
	}

	log.Printf(
		"[community] req_id=%s parsed limit=%d cursor=%q",
		reqID,
		limit,
		cursor,
	)

	res, err := h.Community.ListCommunityConversations(ctx, cursor, limit)
	if err != nil {
		log.Printf(
			"[community] req_id=%s ERROR ListCommunityConversations err=%v",
			reqID,
			err,
		)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	log.Printf(
		"[community] req_id=%s result_count=%d next_cursor=%q",
		reqID,
		len(res.Items),
		res.NextCursor,
	)

	w.Header().Set("Cache-Control", "public, max-age=10")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *HTTP) handleSearchPairs(w http.ResponseWriter, r *http.Request) {
	if h.Search == nil {
		http.Error(w, "search disabled", http.StatusNotImplemented)
		return
	}

	ctx := r.Context()

	q := r.URL.Query().Get("q")
	cursor := r.URL.Query().Get("cursor")
	sort := r.URL.Query().Get("sort")
	visibility := r.URL.Query().Get("visibility")
	limitStr := r.URL.Query().Get("limit")

	limit := 20
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil {
			limit = n
		}
	}

	res, err := h.Search.SearchPairs(ctx, search.SearchParams{
		Q:          q,
		Cursor:     cursor,
		Sort:       search.SortMode(sort),
		Visibility: visibility,
		Limit:      limit,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *HTTP) handleInfer(w http.ResponseWriter, r *http.Request) {
	reqID := newReqID()

	if r.Method != http.MethodPost {
		logf(reqID, `msg="method not allowed" method=%q path=%q`, r.Method, r.URL.Path)
		http.Error(w, "use POST with JSON body", http.StatusMethodNotAllowed)
		return
	}

	var req dispatcher.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logf(reqID, `msg="bad json" err=%q remote=%q`, err.Error(), r.RemoteAddr)
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// Optional: basic validation to avoid weird empty prompts
	if req.Prompt == "" {
		logf(reqID, `msg="validation error" err="empty prompt"`)
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	// Per-request timeout (tune as you like)
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	replyCh := make(chan dispatcher.InferenceResult, 1)
	job := dispatcher.InferenceJob{
		Req:        req,
		Ctx:        ctx,
		ReplyCh:    replyCh,
		EnqueuedAt: time.Now(),
	}

	// Enqueue with backpressure

	stats, err := h.S.TryEnqueue(job)
	if err != nil {
		if errors.Is(err, dispatcher.ErrQueueFull) {
			// QUEUE FULL → 429
			logf(reqID, `msg="queue full" status=429 model=%q cap=%d len=%d`, req.Model, stats.Cap, stats.Len)
			incReq(http.StatusTooManyRequests, "unknown", req.Model)
			http.Error(w, "busy; try again", http.StatusTooManyRequests)
			return
		}
		logf(reqID, `msg="enqueue failed" err=%q model=%q`, err.Error(), req.Model)
		incReq(http.StatusInternalServerError, "unknown", req.Model)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// queued ok

	logf(reqID, `msg="enqueued" model=%q queue_cap=%d queue_len=%d`, req.Model, stats.Cap, stats.Len)

	// Wait for worker result or cancellation/timeout
	select {
	case res := <-replyCh:
		// PROVIDER ERROR
		if res.Err != nil {
			code := http.StatusBadGateway
			if errors.Is(res.Err, context.DeadlineExceeded) {
				code = http.StatusGatewayTimeout
				// CONTEXT TIMEOUT
				logf(reqID, `msg="request timeout" status=%d err=%q model=%q`, code, res.Err.Error(), req.Model)
			} else if errors.Is(res.Err, context.Canceled) {
				code = http.StatusGatewayTimeout
				// CONTEXT CANCELLATION
				logf(reqID, `msg="request cancelled" status=%d err=%q model=%q`, code, res.Err.Error(), req.Model)
			} else {
				logf(reqID, `msg="provider error" status=%d err=%q provider=%q model=%q`, code, res.Err.Error(), res.Provider, req.Model)
			}
			incReq(code, res.Provider, req.Model)
			http.Error(w, res.Err.Error(), code)
			return
		}

		logf(reqID,
			`msg="ok" status=200 provider=%q model=%q queue_wait_ms=%d exec_ms=%d total_ms=%d token_usage=%d`,
			res.Provider,
			req.Model,
			res.QueueWait.Milliseconds(),
			res.ExecTime.Milliseconds(),
			(res.QueueWait + res.ExecTime).Milliseconds(),
			res.TokenUsage,
		)
		incReq(http.StatusOK, res.Provider, req.Model)
		total := res.QueueWait + res.ExecTime
		obs.QueueWait.WithLabelValues(res.Provider, req.Model).Observe(res.QueueWait.Seconds())
		obs.ExecTime.WithLabelValues(res.Provider, req.Model).Observe(res.ExecTime.Seconds())
		obs.TotalTime.WithLabelValues(res.Provider, req.Model).Observe(total.Seconds())

		// JSON Encoding
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":        res.Text,
			"provider":    res.Provider,
			"token_usage": res.TokenUsage,
		})

	case <-ctx.Done():
		// If the client disconnects or we timeout before worker replies
		code := http.StatusGatewayTimeout
		logf(reqID, `msg="ctx done before result" status=%d err=%q model=%q`, code, ctx.Err().Error(), req.Model)
		incReq(code, "unknown", req.Model)
		http.Error(w, "request cancelled/timeout", code)
		return
	}
}
