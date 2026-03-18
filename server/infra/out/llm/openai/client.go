package openai

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
var _ agent.LogAnalyzer = (*Client)(nil)

const defaultBaseURL = "https://api.openai.com/v1"

// Client implements agent.LogAnalyzer for OpenAI GPT.
type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		http:    &http.Client{},
	}
}

func (c *Client) Role() llm.Role { return llm.RoleLogAnalyzer }
func (c *Client) Name() string   { return "openai/" + c.model }

func (c *Client) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 2048
	}
	temp := req.Temperature
	if temp == 0 {
		temp = 0.3
	}

	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.UserPrompt},
		},
		"max_tokens":  maxTokens,
		"temperature": temp,
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: marshal error: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(jsonBytes))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: request error: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: http error: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return llm.CompletionResponse{}, fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: decode error: %w", err)
	}

	content := ""
	finishReason := ""
	if len(result.Choices) > 0 {
		content = result.Choices[0].Message.Content
		finishReason = result.Choices[0].FinishReason
	}

	return llm.CompletionResponse{
		Content:      content,
		FinishReason: finishReason,
		TokensUsed:   result.Usage.TotalTokens,
		Model:        c.model,
	}, nil
}

// AnalyzeLogs implements the llm.LogAnalyzer port.
func (c *Client) AnalyzeLogs(ctx context.Context, logs string) (string, error) {
	resp, err := c.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt.LogAnalysis,
		UserPrompt:  "Analyze these MAAL logs:\n\n" + logs,
		MaxTokens:   2048,
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
