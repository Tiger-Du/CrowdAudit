package providers

import (
	"context"
	"time"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/dispatcher"
)

func StubProvider(delay time.Duration) dispatcher.ProviderFunc {
	return func(ctx context.Context, req dispatcher.InferenceRequest) (string, string, int, error) {
		// Replace with real OpenAI/Anthropic HTTP call.
		// The important part: use ctx in the request and handle errors/timeouts.
		select {
		case <-time.After(delay):
			return "stub response for: " + req.Prompt, "stub", 123, nil
		case <-ctx.Done():
			return "", "stub", 0, ctx.Err()
		}
	}
}
