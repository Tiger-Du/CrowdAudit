package api

import (
	"encoding/json"
	"log"
	"net/http"
)

type voteReq struct {
	ConversationID int64 `json:"conversation_id,string"` // expects a JSON string
	Delta          int   `json:"delta"`
}

type voteRes struct {
	ConversationID int64 `json:"conversation_id"`
	FeedbackScore  int   `json:"feedback_score"`
}

func (h *HTTP) handleVoteCommunityConversation(w http.ResponseWriter, r *http.Request) {
	if h.Community == nil {
		http.Error(w, "community disabled", http.StatusNotImplemented)
		return
	}

	var req voteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.ConversationID == 0 {
		http.Error(w, "conversation_id required", http.StatusBadRequest)
		return
	}
	if req.Delta == 0 {
		http.Error(w, "delta must be non-zero", http.StatusBadRequest)
		return
	}
	// guardrail: UI uses -2..+2
	if req.Delta < -2 || req.Delta > 2 {
		http.Error(w, "delta out of range", http.StatusBadRequest)
		return
	}

	newScore, err := h.Community.AddFeedbackScore(r.Context(), req.ConversationID, req.Delta)
	if err != nil {
		log.Printf("[community] vote ERROR conv_id=%d delta=%d err=%v", req.ConversationID, req.Delta, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(voteRes{
		ConversationID: req.ConversationID,
		FeedbackScore:  newScore,
	})
}
