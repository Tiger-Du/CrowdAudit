package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
)

type Envelope struct {
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

type PairIDPayload struct {
	PairID    int64     `json:"pair_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ResponseIDPayload struct {
	ResponseID int64 `json:"response_id"`
}

func HandleMessage(ctx context.Context, pg *pgxpool.Pool, osClient *opensearch.Client, value []byte) error {
	var env Envelope
	if err := json.Unmarshal(value, &env); err != nil {
		return fmt.Errorf("bad envelope: %w", err)
	}

	switch env.EventType {
	case "pair.upsert":
		// payload expected: {"pair_id":..., "updated_at":"..."} (or just pair_id)
		var p PairIDPayload
		_ = json.Unmarshal(env.Payload, &p)
		if p.PairID == 0 {
			return fmt.Errorf("pair.upsert missing pair_id")
		}
		doc, err := buildPairDoc(ctx, pg, p.PairID)
		if err != nil {
			return err
		}
		return upsertDoc(ctx, osClient, "pairs_v1", fmt.Sprintf("%d", p.PairID), doc)

	case "pair.stats.recompute":
		var p PairIDPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return fmt.Errorf("bad payload: %w", err)
		}
		doc, err := buildPairDoc(ctx, pg, p.PairID)
		if err != nil {
			return err
		}
		return upsertDoc(ctx, osClient, "pairs_v1", fmt.Sprintf("%d", p.PairID), doc)

	case "response.upsert":
		// Optional: If you keep only pairs_v1, you can ignore this.
		// But if you later add responses_v1, you'd fetch response & index it here.
		var r ResponseIDPayload
		_ = json.Unmarshal(env.Payload, &r)
		return nil

	default:
		// unknown events are fine
		return nil
	}
}

type PairDoc struct {
	PairID     string    `json:"pair_id"`
	PromptID   string    `json:"prompt_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Visibility string    `json:"visibility"`

	PromptTitle string `json:"prompt_title"`
	PromptBody  string `json:"prompt_body"`

	ResponseAID string `json:"response_a_id"`
	ResponseBID string `json:"response_b_id"`

	AProvider string `json:"a_provider"`
	AModel    string `json:"a_model"`
	AContent  string `json:"a_content"`

	BProvider string `json:"b_provider"`
	BModel    string `json:"b_model"`
	BContent  string `json:"b_content"`

	VotesTotal        int     `json:"votes_total"`
	VotesA            int     `json:"votes_a"`
	VotesB            int     `json:"votes_b"`
	VotesTie          int     `json:"votes_tie"`
	DisagreementScore float64 `json:"disagreement_score"`
}

// Build a full document from Postgres (source of truth).
func buildPairDoc(ctx context.Context, pg *pgxpool.Pool, pairID int64) (*PairDoc, error) {
	// 1) fetch pair + prompt + responses
	var (
		promptID  int64
		title     string
		body      string
		createdAt time.Time

		raID                       int64
		raProv, raModel, raContent string

		rbID                       int64
		rbProv, rbModel, rbContent string
	)

	err := pg.QueryRow(ctx, `
select
  rp.prompt_id, rp.created_at,
  p.title, p.body,
  ra.id, ra.provider, ra.model, ra.content,
  rb.id, rb.provider, rb.model, rb.content
from response_pairs rp
join prompts p on p.id = rp.prompt_id
join responses ra on ra.id = rp.response_a_id
join responses rb on rb.id = rp.response_b_id
where rp.id = $1
`, pairID).Scan(
		&promptID, &createdAt,
		&title, &body,
		&raID, &raProv, &raModel, &raContent,
		&rbID, &rbProv, &rbModel, &rbContent,
	)
	if err != nil {
		return nil, fmt.Errorf("pair fetch: %w", err)
	}

	// 2) compute vote stats
	var votesTotal, votesA, votesB, votesTie int
	err = pg.QueryRow(ctx, `
select
  count(*)::int as votes_total,
  sum(case when choice = 1 then 1 else 0 end)::int as votes_a,
  sum(case when choice = 2 then 1 else 0 end)::int as votes_b,
  sum(case when choice = 3 then 1 else 0 end)::int as votes_tie
from votes
where pair_id = $1
`, pairID).Scan(&votesTotal, &votesA, &votesB, &votesTie)
	if err != nil {
		return nil, fmt.Errorf("vote stats: %w", err)
	}

	score := disagreementScore(votesA, votesB, votesTotal)

	doc := &PairDoc{
		PairID:     fmt.Sprintf("%d", pairID),
		PromptID:   fmt.Sprintf("%d", promptID),
		CreatedAt:  createdAt,
		UpdatedAt:  time.Now().UTC(),
		Visibility: "public",

		PromptTitle: title,
		PromptBody:  body,

		ResponseAID: fmt.Sprintf("%d", raID),
		ResponseBID: fmt.Sprintf("%d", rbID),

		AProvider: raProv,
		AModel:    raModel,
		AContent:  raContent,

		BProvider: rbProv,
		BModel:    rbModel,
		BContent:  rbContent,

		VotesTotal:        votesTotal,
		VotesA:            votesA,
		VotesB:            votesB,
		VotesTie:          votesTie,
		DisagreementScore: score,
	}
	return doc, nil
}

// Simple, good “controversy” metric.
// - peaks at 50/50 split for A vs B
// - scales with log(1+votes_total)
func disagreementScore(votesA, votesB, votesTotal int) float64 {
	ab := votesA + votesB
	if ab <= 0 {
		return 0
	}
	p := float64(votesA) / float64(ab)
	disagree := 1.0 - math.Abs(2*p-1) // 0..1
	return disagree * math.Log1p(float64(votesTotal))
}

func upsertDoc(ctx context.Context, osClient *opensearch.Client, index, id string, doc any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(doc); err != nil {
		return err
	}

	req := opensearchapi.IndexRequest{
		Index:      index,
		DocumentID: id,
		Body:       &buf,
		Refresh:    "false",
	}
	res, err := req.Do(ctx, osClient)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("index status=%d", res.StatusCode)
	}
	return nil
}

func EnsurePairsIndex(ctx context.Context, osClient *opensearch.Client, index string) error {
	// Check existence
	existsReq := opensearchapi.IndicesExistsRequest{Index: []string{index}}
	existsRes, err := existsReq.Do(ctx, osClient)
	if err != nil {
		return err
	}
	existsRes.Body.Close()
	if existsRes.StatusCode == 200 {
		return nil
	}

	// Create with mapping
	mapping := `{
	  "settings": { "number_of_shards": 1, "number_of_replicas": 0 },
	  "mappings": {
	    "properties": {
	      "pair_id": { "type": "keyword" },
	      "prompt_id": { "type": "keyword" },
	      "created_at": { "type": "date" },
	      "updated_at": { "type": "date" },
	      "visibility": { "type": "keyword" },

	      "prompt_title": { "type": "text", "fields": { "keyword": { "type": "keyword", "ignore_above": 256 } } },
	      "prompt_body": { "type": "text" },

	      "response_a_id": { "type": "keyword" },
	      "response_b_id": { "type": "keyword" },

	      "a_provider": { "type": "keyword" },
	      "a_model": { "type": "keyword" },
	      "a_content": { "type": "text" },

	      "b_provider": { "type": "keyword" },
	      "b_model": { "type": "keyword" },
	      "b_content": { "type": "text" },

	      "votes_total": { "type": "integer" },
	      "votes_a": { "type": "integer" },
	      "votes_b": { "type": "integer" },
	      "votes_tie": { "type": "integer" },
	      "disagreement_score": { "type": "double" }
	    }
	  }
	}`

	createReq := opensearchapi.IndicesCreateRequest{
		Index: index,
		Body:  strings.NewReader(mapping),
	}
	createRes, err := createReq.Do(ctx, osClient)
	if err != nil {
		return err
	}
	defer createRes.Body.Close()
	if createRes.StatusCode >= 300 {
		return fmt.Errorf("create index failed status=%d", createRes.StatusCode)
	}
	return nil
}
