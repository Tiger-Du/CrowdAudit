package search_conversations

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

func (s *CommunityService) AddFeedbackScore(ctx context.Context, conversationID int64, delta int) (int, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("db is nil")
	}
	if delta < -10 || delta > 10 {
		// sanity guard; your UI uses -2..+2 at most
		return 0, fmt.Errorf("delta out of range")
	}

	const q = `
		UPDATE community_alignment_conversations
		SET feedback_score = feedback_score + $1
		WHERE conversation_id = $2
		RETURNING feedback_score
	`

	var newScore int
	err := s.DB.QueryRow(ctx, q, delta, conversationID).Scan(&newScore)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, fmt.Errorf("conversation not found: %d", conversationID)
		}
		log.Printf("[community.db] AddFeedbackScore ERROR conv_id=%d delta=%d err=%v", conversationID, delta, err)
		return 0, err
	}

	return newScore, nil
}
