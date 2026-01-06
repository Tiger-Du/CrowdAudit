package voting

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Service struct {
	db  *pgxpool.Pool
	rng *rand.Rand
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{
		db:  db,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type PairDTO struct {
	PairID   int64       `json:"pairId"`
	PromptID int64       `json:"promptId"`
	Title    string      `json:"title"`
	Prompt   string      `json:"prompt"`
	A        ResponseDTO `json:"a"`
	B        ResponseDTO `json:"b"`
}

type ResponseDTO struct {
	ResponseID int64  `json:"responseId"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Content    string `json:"content"`
}

func (s *Service) GetRandomPair(ctx context.Context, promptID *int64) (*PairDTO, error) {
	var count int
	if promptID == nil {
		if err := s.db.QueryRow(ctx, `select count(*) from response_pairs`).Scan(&count); err != nil {
			return nil, err
		}
	} else {
		if err := s.db.QueryRow(ctx, `select count(*) from response_pairs where prompt_id=$1`, *promptID).Scan(&count); err != nil {
			return nil, err
		}
	}
	if count == 0 {
		return nil, ErrNotFound
	}

	offset := s.rng.Intn(count)

	// NOTE: for large tables, replace OFFSET sampling later
	query := `
select
  p.id, p.title, p.body,
  rp.id,
  ra.id, ra.provider, ra.model, ra.content,
  rb.id, rb.provider, rb.model, rb.content
from response_pairs rp
join prompts p on p.id = rp.prompt_id
join responses ra on ra.id = rp.response_a_id
join responses rb on rb.id = rp.response_b_id
`
	args := []any{}
	if promptID != nil {
		query += ` where rp.prompt_id = $1`
		args = append(args, *promptID)
	}
	query += ` order by rp.id limit 1 offset ` + strconv.Itoa(offset)

	var dto PairDTO
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&dto.PromptID, &dto.Title, &dto.Prompt,
		&dto.PairID,
		&dto.A.ResponseID, &dto.A.Provider, &dto.A.Model, &dto.A.Content,
		&dto.B.ResponseID, &dto.B.Provider, &dto.B.Model, &dto.B.Content,
	)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *Service) CreateVote(ctx context.Context, pairID int64, voterID string, choice int16) (status string, err error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cmd, err := tx.Exec(ctx, `
insert into votes (pair_id, voter_id, choice)
values ($1, $2, $3)
on conflict (pair_id, voter_id) do nothing
`, pairID, voterID, choice)
	if err != nil {
		return "", err
	}
	if cmd.RowsAffected() == 0 {
		// Duplicate vote -> no indexing event needed.
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return "duplicate", nil
	}

	// Enqueue async indexing work: recompute stats for this pair.
	// Key ensures all updates for this pair stay ordered in Kafka partitioning.
	// Payload keeps it small; indexer will query Postgres for full stats.
	_, err = tx.Exec(ctx, `
insert into outbox_events (topic, key, event_type, payload)
values ($1, $2, $3, $4::jsonb)
`,
		"search-index",
		"pair:"+strconv.FormatInt(pairID, 10),
		"pair.stats.recompute",
		`{"pair_id":`+strconv.FormatInt(pairID, 10)+`,"updated_at":"`+time.Now().UTC().Format(time.RFC3339Nano)+`"}`,
	)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return "recorded", nil
}
