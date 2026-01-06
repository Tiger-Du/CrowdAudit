package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/dispatcher"
)

type openRouterChatReq struct {
	Model    string          `json:"model"`
	Messages []openRouterMsg `json:"messages"`
	// You can add temperature, max_tokens, etc. later.
}

type openRouterMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterChatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func OpenRouterProvider(httpClient *http.Client) dispatcher.ProviderFunc {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		// Fail fast at startup in main() if you prefer; this is a fallback.
		panic("OPENROUTER_API_KEY is not set")
	}

	baseURL := os.Getenv("OPENROUTER_BASE_URL")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	return func(ctx context.Context, req dispatcher.InferenceRequest) (string, string, int, error) {
		model := req.Model
		if model == "" {
			model = "allenai/olmo-3.1-32b-think:free"
		}

		payload := openRouterChatReq{
			Model: model,
			Messages: []openRouterMsg{
				{Role: "user", Content: req.Prompt},
			},
		}

		b, err := json.Marshal(payload)
		if err != nil {
			return "", "openrouter", 0, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(b))
		if err != nil {
			return "", "openrouter", 0, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		// Optional but recommended by OpenRouter for rankings/identification:
		// httpReq.Header.Set("HTTP-Referer", "https://your-site.example")
		// httpReq.Header.Set("X-Title", "CrowdAudit")

		resp, err := httpClient.Do(httpReq)
		if err != nil {
			return "", "openrouter", 0, err
		}
		defer resp.Body.Close()

		var out openRouterChatResp
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", "openrouter", 0, fmt.Errorf("decode response: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			msg := resp.Status
			if out.Error != nil && out.Error.Message != "" {
				msg = out.Error.Message
			}
			return "", "openrouter", 0, fmt.Errorf("openrouter error: %s", msg)
		}

		if len(out.Choices) == 0 {
			return "", "openrouter", 0, fmt.Errorf("openrouter: empty choices")
		}

		tokens := 0
		if out.Usage != nil {
			tokens = out.Usage.TotalTokens
		}

		return out.Choices[0].Message.Content, "openrouter", tokens, nil
	}
}

// Helper for main.go (nice default http client)
func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}
