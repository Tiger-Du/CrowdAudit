package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/infra/dburl"
)

type outboxRow struct {
	ID        int64
	Topic     string
	Key       string
	EventType string
	Payload   []byte
}

var (
	once    sync.Once
	initErr error

	pg       *pgxpool.Pool
	sqsCli   *sqs.Client
	queueURL string

	batchSize int
)

func initOnce() {
	ctx := context.Background()

	// log.Printf("env DATABASE_URL=%q", os.Getenv("DATABASE_URL"))
	// log.Printf("env PGHOST=%q PGUSER=%q PGDATABASE=%q", os.Getenv("PGHOST"), os.Getenv("PGUSER"), os.Getenv("PGDATABASE"))

	// --- config ---
	queueURL = os.Getenv("INDEX_QUEUE_URL")
	if queueURL == "" {
		initErr = fmt.Errorf("INDEX_QUEUE_URL is required")
		return
	}

	batchSize = 100
	if s := os.Getenv("BATCH_SIZE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			batchSize = n
		}
	}

	// --- db ---
	dbURL, err := dburl.Load(ctx)
	// log.Printf("DB URL = %q", dbURL)
	// log.Printf("dburl.Load() -> %q err=%v", dbURL, err)
	if err != nil {
		initErr = err
		return
	}
	// u, _ := url.Parse(dbURL)
	// log.Printf("dburl.Load: host=%s raw=%q", u.Host, dbURL)

	pg, err = pgxpool.New(ctx, dbURL)
	if err != nil {
		initErr = err
		return
	}

	// --- sqs client ---
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		initErr = fmt.Errorf("load aws config: %w", err)
		return
	}
	sqsCli = sqs.NewFromConfig(awsCfg)
}

type scheduledEvent struct {
	// EventBridge scheduled events have fields, but we don't need them.
	// Keep it permissive.
}

func handler(ctx context.Context, _ scheduledEvent) error {
	log.Printf("PUBLISHER_BUILD_MARKER=2025-12-26T01:40Z")
	// once.Do(initOnce)
	initOnce()
	log.Printf("TEST_PUBLISHER_BUILD_MARKER=2025-12-26T01:40Z")
	if initErr != nil {
		return initErr
	}

	// Process multiple batches in one invocation, but cap runtime.
	// (Keeps up if you have a burst.)
	deadline := time.Now().Add(20 * time.Second)
	totalSent := 0

	for time.Now().Before(deadline) {
		n, err := publishOnce(ctx)
		if err != nil {
			// Returning error causes retry (good), but we already mark published for successful sends.
			return err
		}
		totalSent += n
		if n == 0 {
			break
		}
		// If we sent fewer than the batch size, queue is probably drained.
		if n < batchSize {
			break
		}
	}

	if totalSent > 0 {
		log.Printf("publisher: sent=%d", totalSent)
	}
	return nil
}

func publishOnce(ctx context.Context) (int, error) {
	rows, err := pg.Query(ctx, `
		select id, topic, key, event_type, payload
		from outbox_events
		where published_at is null
		order by id
		limit $1
	`, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	batch := make([]outboxRow, 0, batchSize)
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.ID, &r.Topic, &r.Key, &r.EventType, &r.Payload); err != nil {
			return 0, err
		}
		batch = append(batch, r)
	}
	if rows.Err() != nil {
		return 0, rows.Err()
	}
	if len(batch) == 0 {
		return 0, nil
	}

	// Build SQS entries (10 max per SendMessageBatch)
	type entry struct {
		rowID int64
		req   types.SendMessageBatchRequestEntry
	}
	entries := make([]entry, 0, len(batch))

	for i, r := range batch {
		envelope := map[string]any{
			"event_type": r.EventType,
			"payload":    json.RawMessage(r.Payload),
		}
		b, err := json.Marshal(envelope)
		if err != nil {
			// mark failed and continue
			_ = markFailed(ctx, []int64{r.ID}, fmt.Sprintf("marshal envelope: %v", err))
			continue
		}

		// Use a stable per-batch unique ID for the batch request entry.
		// (SQS requires Id field unique within the batch request.)
		id := fmt.Sprintf("m-%d", i)

		entries = append(entries, entry{
			rowID: r.ID,
			req: types.SendMessageBatchRequestEntry{
				Id:          aws.String(id),
				MessageBody: aws.String(string(b)),
				// Preserve ordering per key only if you later switch to FIFO; for standard queue it’s ignored.
				MessageAttributes: map[string]types.MessageAttributeValue{
					"topic": {
						DataType:    aws.String("String"),
						StringValue: aws.String(r.Topic),
					},
					"key": {
						DataType:    aws.String("String"),
						StringValue: aws.String(r.Key),
					},
				},
			},
		})
	}

	// Send in chunks of 10
	var (
		okIDs   []int64
		failIDs []int64
		failMsg string
	)

	for i := 0; i < len(entries); i += 10 {
		j := i + 10
		if j > len(entries) {
			j = len(entries)
		}
		chunk := entries[i:j]

		reqEntries := make([]types.SendMessageBatchRequestEntry, 0, len(chunk))
		idToRow := make(map[string]int64, len(chunk))
		for _, e := range chunk {
			reqEntries = append(reqEntries, e.req)
			idToRow[*e.req.Id] = e.rowID
		}

		out, err := sqsCli.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{
			QueueUrl: aws.String(queueURL),
			Entries:  reqEntries,
		})
		if err != nil {
			// Entire chunk failed. Mark all as failed.
			for _, e := range chunk {
				failIDs = append(failIDs, e.rowID)
			}
			failMsg = fmt.Sprintf("send batch: %v", err)
			continue
		}

		// Successful ones
		for _, s := range out.Successful {
			if s.Id != nil {
				if rowID, ok := idToRow[*s.Id]; ok {
					okIDs = append(okIDs, rowID)
				}
			}
		}

		// Failed ones
		for _, f := range out.Failed {
			if f.Id != nil {
				if rowID, ok := idToRow[*f.Id]; ok {
					failIDs = append(failIDs, rowID)
				}
			}
			// Keep last error message for logging/DB
			if f.Message != nil {
				failMsg = *f.Message
			}
		}
	}

	// Mark published for successes
	if len(okIDs) > 0 {
		if err := markPublished(ctx, okIDs); err != nil {
			// if this fails, we may resend duplicates on retry (still at-least-once).
			return 0, fmt.Errorf("markPublished: %w", err)
		}
	}

	// Mark failed for failures
	if len(failIDs) > 0 {
		if failMsg == "" {
			failMsg = "send failed"
		}
		_ = markFailed(ctx, failIDs, failMsg)
		// Return error so Lambda retries remaining unpublished items.
		return len(okIDs), fmt.Errorf("some messages failed: failed=%d", len(failIDs))
	}

	return len(okIDs), nil
}

func markPublished(ctx context.Context, ids []int64) error {
	_, err := pg.Exec(ctx, `
		update outbox_events
		set published_at = now(), last_error = null
		where id = any($1)
	`, ids)
	return err
}

func markFailed(ctx context.Context, ids []int64, errMsg string) error {
	_, err := pg.Exec(ctx, `
		update outbox_events
		set attempts = attempts + 1, last_error = $2
		where id = any($1)
	`, ids, errMsg)
	return err
}

func main() {
	lambda.Start(handler)
}
