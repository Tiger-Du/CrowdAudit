package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/dispatcher"
)

func TestInfer_MethodNotAllowed(t *testing.T) {
	// Dispatcher can exist but won't be used by this test.
	provider := func(ctx context.Context, req dispatcher.InferenceRequest) (string, string, int, error) {
		return "unused", "test", 0, nil
	}
	disp := dispatcher.New(10, 1, provider)
	defer disp.Shutdown()

	h := New(disp)
	handler := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/infer", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d got %d body=%s", http.StatusMethodNotAllowed, rr.Code, rr.Body.String())
	}
}

func TestInfer_BadJSON(t *testing.T) {
	provider := func(ctx context.Context, req dispatcher.InferenceRequest) (string, string, int, error) {
		return "unused", "test", 0, nil
	}
	disp := dispatcher.New(10, 1, provider)
	defer disp.Shutdown()

	h := New(disp)
	handler := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/api/infer", strings.NewReader("nope"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d body=%s", http.StatusBadRequest, rr.Code, rr.Body.String())
	}
}

func TestInfer_OK(t *testing.T) {
	// fast deterministic provider
	provider := func(ctx context.Context, req dispatcher.InferenceRequest) (string, string, int, error) {
		return "hi " + req.Prompt, "test", 7, nil
	}
	disp := dispatcher.New(10, 1, provider)
	defer disp.Shutdown()

	h := New(disp, WithRequestTimeout(2*time.Second))
	handler := h.Routes()

	body := `{"prompt":"hello","model":"stub"}`
	req := httptest.NewRequest(http.MethodPost, "/api/infer", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad response json: %v body=%s", err, rr.Body.String())
	}
	if out["provider"] != "test" {
		t.Fatalf("expected provider=test got %#v", out["provider"])
	}
}

func TestInfer_QueueFull_Returns429(t *testing.T) {
	// workers=0 ensures queue doesn't drain
	provider := func(ctx context.Context, req dispatcher.InferenceRequest) (string, string, int, error) {
		return "unused", "test", 0, nil
	}
	disp := dispatcher.New(1, 0, provider)
	defer disp.Shutdown()

	// Pre-fill the queue via public API (TryEnqueue) with a dummy job so it's definitely full.
	dummyReply := make(chan dispatcher.InferenceResult, 1)
	dummyJob := dispatcher.InferenceJob{
		Req:        dispatcher.InferenceRequest{Prompt: "dummy", Model: "stub"},
		Ctx:        context.Background(),
		ReplyCh:    dummyReply,
		EnqueuedAt: time.Now(),
	}
	_, _ = disp.TryEnqueue(dummyJob)

	h := New(disp, WithRequestTimeout(2*time.Second))
	handler := h.Routes()

	body := `{"prompt":"hello","model":"stub"}`
	req := httptest.NewRequest(http.MethodPost, "/api/infer", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected %d got %d body=%s", http.StatusTooManyRequests, rr.Code, rr.Body.String())
	}
}

func TestInfer_Timeout_Returns504(t *testing.T) {
	provider := func(ctx context.Context, req dispatcher.InferenceRequest) (string, string, int, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return "late", "test", 0, nil
		case <-ctx.Done():
			return "", "test", 0, ctx.Err()
		}
	}
	disp := dispatcher.New(10, 1, provider)
	defer disp.Shutdown()

	h := New(disp, WithRequestTimeout(50*time.Millisecond))
	handler := h.Routes()

	body := `{"prompt":"hello","model":"stub"}`
	req := httptest.NewRequest(http.MethodPost, "/api/infer", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 got %d body=%s", rr.Code, rr.Body.String())
	}
}
