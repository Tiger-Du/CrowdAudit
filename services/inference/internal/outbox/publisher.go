package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"
)

type Publisher struct {
	DB     *pgxpool.Pool
	Writer *kafka.Writer
	// Tune these
	BatchSize int
	PollEvery time.Duration
}

type outboxRow struct {
	ID        int64
	Topic     string
	Key       string
	EventType string
	Payload   []byte
}

func (p *Publisher) Run(ctx context.Context) error {
	if p.BatchSize <= 0 {
		p.BatchSize = 100
	}
	if p.PollEvery <= 0 {
		p.PollEvery = 300 * time.Millisecond
	}

	t := time.NewTicker(p.PollEvery)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil // So that shutdown is not logged as an error
		case <-t.C:
			if err := p.publishOnce(ctx); err != nil {
				// Don’t die on transient issues; log and continue.
				log.Printf("outbox publish error: %v", err)
			}
		}
	}
}

func (p *Publisher) publishOnce(ctx context.Context) error {
	rows, err := p.DB.Query(ctx, `
		select id, topic, key, event_type, payload
		from outbox_events
		where published_at is null
		order by id
		limit $1
	`, p.BatchSize)
	if err != nil {
		return err
	}
	defer rows.Close()

	batch := make([]outboxRow, 0, p.BatchSize)
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.ID, &r.Topic, &r.Key, &r.EventType, &r.Payload); err != nil {
			return err
		}
		batch = append(batch, r)
	}
	if rows.Err() != nil {
		return rows.Err()
	}
	if len(batch) == 0 {
		return nil
	}

	// Produce messages (at-least-once). If this errors part-way through,
	// some messages may have been produced; we only mark DB as published on success.
	msgs := make([]kafka.Message, 0, len(batch))
	for _, r := range batch {
		envelope := map[string]any{
			"event_type": r.EventType,
			"payload":    json.RawMessage(r.Payload),
		}
		b, err := json.Marshal(envelope)
		if err != nil {
			return fmt.Errorf("marshal envelope: %w", err)
		}
		msgs = append(msgs, kafka.Message{
			Topic: r.Topic,
			Key:   []byte(r.Key),
			Value: b,
		})
	}

	// WriteMessages is atomic-ish in that it returns an error if some failed,
	// but the broker may have accepted some. That’s fine: we’re at-least-once.
	if err := p.Writer.WriteMessages(ctx, msgs...); err != nil {
		// record attempt + error for each row
		_ = p.markFailed(ctx, batch, err.Error())
		return err
	}

	// Mark published
	return p.markPublished(ctx, batch)
}

func (p *Publisher) markPublished(ctx context.Context, batch []outboxRow) error {
	ids := make([]int64, 0, len(batch))
	for _, r := range batch {
		ids = append(ids, r.ID)
	}
	_, err := p.DB.Exec(ctx, `
		update outbox_events
		set published_at = now(), last_error = null
		where id = any($1)
	`, ids)
	return err
}

func (p *Publisher) markFailed(ctx context.Context, batch []outboxRow, errMsg string) error {
	ids := make([]int64, 0, len(batch))
	for _, r := range batch {
		ids = append(ids, r.ID)
	}
	_, err := p.DB.Exec(ctx, `
		update outbox_events
		set attempts = attempts + 1, last_error = $2
		where id = any($1)
	`, ids, errMsg)
	return err
}

func NewWriter(brokers []string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		RequiredAcks: kafka.RequireAll,
		Balancer:     &kafka.Hash{}, // IMPORTANT: per-key ordering
	}
}
