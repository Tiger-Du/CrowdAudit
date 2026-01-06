package providers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/dispatcher"

	"google.golang.org/genai"
)

func NewGeminiClient(ctx context.Context) (*genai.Client, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is not set")
	}

	return genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
}

// GeminiProvider returns a dispatcher.ProviderFunc that calls Gemini.
// defaultModel is used if req.Model is empty or "stub".
func GeminiProvider(client *genai.Client, defaultModel string) dispatcher.ProviderFunc {
	return func(ctx context.Context, req dispatcher.InferenceRequest) (string, string, int, error) {
		model := strings.TrimSpace(req.Model)
		if model == "" || model == "stub" {
			model = defaultModel
		}

		// Use the request context (ctx) so your timeouts/cancellation apply.
		result, err := client.Models.GenerateContent(
			ctx,
			model,
			genai.Text(req.Prompt),
			nil,
		)
		if err != nil {
			return "", "gemini", 0, err
		}

		// Token usage: depends on which fields you have on the response.
		// Start with 0; later you can extract from usage metadata if available.
		return result.Text(), "gemini", 0, nil
	}
}
