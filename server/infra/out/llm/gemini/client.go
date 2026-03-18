package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"server/internal/app/agent"
	"server/internal/domain/llm"
	"server/infra/out/llm/prompt"
)

// Compile-time interface check.
var _ agent.Summarizer = (*Client)(nil)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Client implements agent.Summarizer for Google Gemini.
type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		http:    &http.Client{},
	}
}

func (c *Client) Role() llm.Role { return llm.RoleSummarizer }
func (c *Client) Name() string   { return "gemini/" + c.model }

func (c *Client) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 2048
	}
	temp := req.Temperature
	if temp == 0 {
		temp = 0.3
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, c.model)

	body := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": req.SystemPrompt + "\n\n" + req.UserPrompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": maxTokens,
			"temperature":     temp,
		},
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: marshal error: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: request error: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: http error: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			TotalTokenCount int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: decode error: %w", err)
	}

	content := ""
	finishReason := ""
	if len(result.Candidates) > 0 {
		finishReason = result.Candidates[0].FinishReason
		for _, part := range result.Candidates[0].Content.Parts {
			content += part.Text
		}
	}

	return llm.CompletionResponse{
		Content:      content,
		FinishReason: finishReason,
		TokensUsed:   result.UsageMetadata.TotalTokenCount,
		Model:        c.model,
	}, nil
}

// Summarize implements the llm.Summarizer port.
func (c *Client) Summarize(ctx context.Context, content string) (string, error) {
	resp, err := c.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt.Summarization,
		UserPrompt:  "Summarize this agent's execution output in detail:\n\n" + content,
		MaxTokens:   4096,
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// GenerateFrontierCommand implements the llm.Summarizer port.
func (c *Client) GenerateFrontierCommand(ctx context.Context, accumulatedSummary string, childTask string) (string, error) {
	resp, err := c.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt.FrontierCommand,
		UserPrompt:  fmt.Sprintf("## Accumulated Context from Parent Tasks:\n%s\n\n## Child Task to Generate Frontier For:\n%s", accumulatedSummary, childTask),
		MaxTokens:   4096,
		Temperature: 0.3,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
