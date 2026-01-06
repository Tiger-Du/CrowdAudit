package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/voting"
)

func (h *HTTP) handleGetRandomPair(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var promptID *int64
	if q := r.URL.Query().Get("promptId"); q != "" {
		v, err := strconv.ParseInt(q, 10, 64)
		if err != nil {
			http.Error(w, "invalid promptId", http.StatusBadRequest)
			return
		}
		promptID = &v
	}

	pair, err := h.V.GetRandomPair(ctx, promptID)
	if err != nil {
		if errors.Is(err, voting.ErrNotFound) {
			http.Error(w, "no pairs available", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, pair, http.StatusOK)
}

type createVoteReq struct {
	PairID  int64  `json:"pairId"`
	VoterID string `json:"voterId"`
	Choice  string `json:"choice"` // "A" | "B" | "TIE"
}

func (h *HTTP) handleCreateVote(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var req createVoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.PairID <= 0 || req.VoterID == "" {
		http.Error(w, "pairId and voterId required", http.StatusBadRequest)
		return
	}

	code, err := choiceToCode(req.Choice)
	if err != nil {
		http.Error(w, "choice must be A, B, or TIE", http.StatusBadRequest)
		return
	}

	status, err := h.V.CreateVote(ctx, req.PairID, req.VoterID, code)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": status}, http.StatusOK)
}

func choiceToCode(c string) (int16, error) {
	switch c {
	case "A":
		return 1, nil
	case "B":
		return 2, nil
	case "TIE":
		return 3, nil
	default:
		return 0, errors.New("invalid choice")
	}
}

func writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
