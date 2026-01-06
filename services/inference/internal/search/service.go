package search

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	opensearch "github.com/opensearch-project/opensearch-go/v2"
	opensearchapi "github.com/opensearch-project/opensearch-go/v2/opensearchapi"
)

type Service struct {
	OS    *opensearch.Client
	Index string
}

func NewService(os *opensearch.Client, index string) *Service {
	return &Service{OS: os, Index: index}
}

type SortMode string

const (
	SortDisagreement SortMode = "disagreement" // homepage default
	SortVotes        SortMode = "votes"
	SortNew          SortMode = "new"
	SortRelevance    SortMode = "relevance"
)

// Cursor is an opaque search_after payload.
// We keep it simple: just the OpenSearch "sort" array from the last hit.
type Cursor struct {
	Sort []any `json:"sort"`
}

func EncodeCursor(c Cursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func DecodeCursor(s string) (Cursor, error) {
	var c Cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

type PairDoc struct {
	PairID     string `json:"pair_id"`
	PromptID   string `json:"prompt_id"`
	Visibility string `json:"visibility"`

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
	UpdatedAt         string  `json:"updated_at"`
}

type VotesDTO struct {
	Total int `json:"total"`
	A     int `json:"a"`
	B     int `json:"b"`
	Tie   int `json:"tie"`
}

type ResponseDTO struct {
	ResponseID int64  `json:"responseId"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Content    string `json:"content"`
}

type PairDTO struct {
	PairID            int64       `json:"pairId"`
	PromptID          int64       `json:"promptId"`
	Title             string      `json:"title"`
	Prompt            string      `json:"prompt"`
	A                 ResponseDTO `json:"a"`
	B                 ResponseDTO `json:"b"`
	Votes             VotesDTO    `json:"votes"`
	DisagreementScore float64     `json:"disagreementScore"`
	UpdatedAt         string      `json:"updatedAt"`
}

type SearchResult struct {
	Items      []PairDTO `json:"items"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

type SearchParams struct {
	Q          string
	Visibility string
	Sort       SortMode
	Limit      int
	Cursor     string
}

func (s *Service) SearchPairs(ctx context.Context, p SearchParams) (*SearchResult, error) {
	if p.Limit <= 0 {
		p.Limit = 20
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	if p.Visibility == "" {
		p.Visibility = "public"
	}
	if p.Sort == "" {
		// good default:
		// - if query present: relevance
		// - else: disagreement
		if strings.TrimSpace(p.Q) != "" {
			p.Sort = SortRelevance
		} else {
			p.Sort = SortDisagreement
		}
	}

	body, err := buildQuery(p)
	if err != nil {
		return nil, err
	}

	req := opensearchapi.SearchRequest{
		Index: []string{s.Index},
		Body:  bytes.NewReader(body),
	}
	res, err := req.Do(ctx, s.OS)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("opensearch search status=%d body=%s", res.StatusCode, strings.TrimSpace(string(b)))
	}

	var raw struct {
		Hits struct {
			Hits []struct {
				ID     string          `json:"_id"`
				Source json.RawMessage `json:"_source"`
				Sort   []any           `json:"sort"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, err
	}

	out := &SearchResult{Items: make([]PairDTO, 0, len(raw.Hits.Hits))}
	var lastSort []any

	for _, h := range raw.Hits.Hits {
		var d PairDoc
		if err := json.Unmarshal(h.Source, &d); err != nil {
			return nil, fmt.Errorf("decode _source: %w", err)
		}

		dto, err := docToDTO(d)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, dto)
		lastSort = h.Sort
	}

	if len(lastSort) > 0 && len(out.Items) > 0 {
		c, err := EncodeCursor(Cursor{Sort: lastSort})
		if err != nil {
			return nil, err
		}
		out.NextCursor = c
	}

	return out, nil
}

func buildQuery(p SearchParams) ([]byte, error) {
	q := strings.TrimSpace(p.Q)

	must := []any{}
	filter := []any{
		map[string]any{"term": map[string]any{"visibility": p.Visibility}},
	}

	if q != "" {
		must = append(must, map[string]any{
			"multi_match": map[string]any{
				"query":    q,
				"type":     "best_fields",
				"operator": "and",
				"fields": []string{
					"prompt_title^4",
					"prompt_body^2",
					"a_content",
					"b_content",
				},
			},
		})
	}

	sort := []any{}
	switch p.Sort {
	case SortNew:
		sort = append(sort,
			map[string]any{"updated_at": map[string]any{"order": "desc"}},
			map[string]any{"pair_id": map[string]any{"order": "desc"}},
		)
	case SortVotes:
		sort = append(sort,
			map[string]any{"votes_total": map[string]any{"order": "desc"}},
			map[string]any{"disagreement_score": map[string]any{"order": "desc"}},
			map[string]any{"pair_id": map[string]any{"order": "desc"}},
		)
	case SortRelevance:
		// only makes sense if q != ""
		if q != "" {
			sort = append(sort,
				"_score",
				map[string]any{"disagreement_score": map[string]any{"order": "desc"}},
				map[string]any{"votes_total": map[string]any{"order": "desc"}},
				map[string]any{"pair_id": map[string]any{"order": "desc"}},
			)
		} else {
			// fall back
			sort = append(sort,
				map[string]any{"disagreement_score": map[string]any{"order": "desc"}},
				map[string]any{"votes_total": map[string]any{"order": "desc"}},
				map[string]any{"pair_id": map[string]any{"order": "desc"}},
			)
		}
	default: // disagreement
		sort = append(sort,
			map[string]any{"disagreement_score": map[string]any{"order": "desc"}},
			map[string]any{"votes_total": map[string]any{"order": "desc"}},
			map[string]any{"updated_at": map[string]any{"order": "desc"}},
			map[string]any{"pair_id": map[string]any{"order": "desc"}},
		)
	}

	body := map[string]any{
		"size": p.Limit,
		"_source": []string{
			"pair_id", "prompt_id", "visibility",
			"prompt_title", "prompt_body",
			"response_a_id", "response_b_id",
			"a_provider", "a_model", "a_content",
			"b_provider", "b_model", "b_content",
			"votes_total", "votes_a", "votes_b", "votes_tie",
			"disagreement_score", "updated_at",
		},
		"query": map[string]any{
			"bool": map[string]any{
				"must":   must,
				"filter": filter,
			},
		},
		"sort": sort,
	}

	if p.Cursor != "" {
		c, err := DecodeCursor(p.Cursor)
		if err != nil {
			return nil, fmt.Errorf("bad cursor: %w", err)
		}
		if len(c.Sort) > 0 {
			body["search_after"] = c.Sort
		}
	}

	return json.Marshal(body)
}

func docToDTO(d PairDoc) (PairDTO, error) {
	// Your doc stores ids as strings (keyword). Convert to int64 for your DTO.
	parseI64 := func(s string) int64 {
		var x int64
		// fast/loose parse; if it fails you’ll get 0 which is fine for MVP
		_, _ = fmt.Sscan(s, &x)
		return x
	}

	return PairDTO{
		PairID:   parseI64(d.PairID),
		PromptID: parseI64(d.PromptID),
		Title:    d.PromptTitle,
		Prompt:   d.PromptBody,
		A: ResponseDTO{
			ResponseID: parseI64(d.ResponseAID),
			Provider:   d.AProvider,
			Model:      d.AModel,
			Content:    d.AContent,
		},
		B: ResponseDTO{
			ResponseID: parseI64(d.ResponseBID),
			Provider:   d.BProvider,
			Model:      d.BModel,
			Content:    d.BContent,
		},
		Votes: VotesDTO{
			Total: d.VotesTotal,
			A:     d.VotesA,
			B:     d.VotesB,
			Tie:   d.VotesTie,
		},
		DisagreementScore: d.DisagreementScore,
		UpdatedAt:         d.UpdatedAt,
	}, nil
}
