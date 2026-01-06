package search_conversations

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CommunityConversation struct {
	ConversationID     int64  `json:"conversation_id,string"` // <- IMPORTANT
	AssignedLang       string `json:"assigned_lang"`
	FirstTurnPrompt    string `json:"first_turn_prompt"`
	FirstTurnResponseA string `json:"first_turn_response_a"`
	FirstTurnResponseB string `json:"first_turn_response_b"`
	FirstTurnFeedback  string `json:"first_turn_feedback,omitempty"`
	FeedbackScore      int    `json:"feedback_score"`
}

type ListResponse struct {
	Items      []CommunityConversation `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type CommunityService struct {
	DB *pgxpool.Pool
}

// Cursor is base64url("unixnano|pair_id")
func encodeCursor(createdAt time.Time, pairID int64) string {
	raw := fmt.Sprintf("%d|%d", createdAt.UnixNano(), pairID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cur string) (time.Time, int64, bool) {
	if cur == "" {
		return time.Time{}, 0, false
	}
	b, err := base64.RawURLEncoding.DecodeString(cur)
	if err != nil {
		return time.Time{}, 0, false
	}
	parts := strings.Split(string(b), "|")
	if len(parts) != 2 {
		return time.Time{}, 0, false
	}
	ns, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	return time.Unix(0, ns).UTC(), id, true
}

func (s *CommunityService) ListCommunityConversations(
	ctx context.Context,
	cursor string,
	limit int,
) (ListResponse, error) {

	log.Printf(
		"[community.db] start cursor=%q limit=%d db_nil=%v",
		cursor,
		limit,
		s.DB == nil,
	)

	if s.DB == nil {
		return ListResponse{}, fmt.Errorf("db is nil")
	}

	const q = `
		SELECT
			conversation_id,
			assigned_lang,
			first_turn_prompt,
			first_turn_response_a,
			first_turn_response_b,
			first_turn_feedback,
			feedback_score
		FROM community_alignment_conversations
		ORDER BY conversation_id DESC
		LIMIT $1
	`

	log.Printf("[community.db] sql=%s args=[%d]", q, limit)

	rows, err := s.DB.Query(ctx, q, limit)
	if err != nil {
		log.Printf("[community.db] QUERY ERROR err=%v", err)
		return ListResponse{}, err
	}
	defer rows.Close()

	items := []CommunityConversation{}
	rowCount := 0

	for rows.Next() {
		rowCount++

		var c CommunityConversation
		if err := rows.Scan(
			&c.ConversationID,
			&c.AssignedLang,
			&c.FirstTurnPrompt,
			&c.FirstTurnResponseA,
			&c.FirstTurnResponseB,
			&c.FirstTurnFeedback,
			&c.FeedbackScore,
		); err != nil {
			log.Printf("[community.db] SCAN ERROR row=%d err=%v", rowCount, err)
			return ListResponse{}, err
		}

		items = append(items, c)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[community.db] ROWS ERROR err=%v", err)
		return ListResponse{}, err
	}

	log.Printf(
		"[community.db] done rows=%d returned=%d",
		rowCount,
		len(items),
	)

	return ListResponse{
		Items: items,
	}, nil
}
