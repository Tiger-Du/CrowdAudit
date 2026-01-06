package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type Event struct {
	Topic     string
	Key       string
	EventType string
	Payload   any
}

func InsertEvent(ctx context.Context, tx pgx.Tx, e Event) error {
	b, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	_, err = tx.Exec(ctx, `
		insert into outbox_events (topic, key, event_type, payload)
		values ($1, $2, $3, $4::jsonb)
	`, e.Topic, e.Key, e.EventType, string(b))
	if err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}
