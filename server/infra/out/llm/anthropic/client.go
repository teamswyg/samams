package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"server/internal/app/agent"
	"server/internal/domain/llm"
	"server/internal/domain/project"
	"server/infra/out/llm/prompt"
)

// Compile-time interface check.
var _ agent.Planner = (*Client)(nil)

const defaultBaseURL = "https://api.anthropic.com/v1"

// Client implements agent.Planner for Anthropic Claude.
type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		http:    &http.Client{},
	}
}

func (c *Client) Role() llm.Role { return llm.RolePlanner }
func (c *Client) Name() string   { return "anthropic/" + c.model }

func (c *Client) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	temp := req.Temperature
	if temp == 0 {
		temp = 0.4
	}

	body := map[string]any{
		"model":      c.model,
		"max_tokens": maxTokens,
		"system":     req.SystemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": req.UserPrompt},
		},
		"temperature": temp,
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: marshal error: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(jsonBytes))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: request error: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: http error: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: status %d: %s (model=%s, maxTokens=%d, promptLen=%d)",
			resp.StatusCode, string(respBody), c.model, maxTokens, len(req.UserPrompt))
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: decode error: %w", err)
	}

	content := ""
	for _, block := range result.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return llm.CompletionResponse{
		Content:      content,
		FinishReason: result.StopReason,
		TokensUsed:   result.Usage.InputTokens + result.Usage.OutputTokens,
		Model:        c.model,
	}, nil
}

// GeneratePlan implements the llm.Planner port.
func (c *Client) GeneratePlan(ctx context.Context, userPrompt string) (string, error) {
	resp, err := c.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt.PlanGeneration,
		UserPrompt:  userPrompt,
		MaxTokens:   16384,
		Temperature: 0.4,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// AnalyzeReview sends code review findings to Claude for APPROVED/NEEDS_WORK decision.
func (c *Client) AnalyzeReview(ctx context.Context, reviewContext string) (string, error) {
	resp, err := c.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt.ReviewAnalysis,
		UserPrompt:   reviewContext,
		MaxTokens:    8192,
		Temperature:  0.3,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ConvertToNodeTree implements the llm.Planner port.
// After receiving the LLM response, it enforces the 3-level hierarchy rule.
func (c *Client) ConvertToNodeTree(ctx context.Context, planDocument string) (string, error) {
	resp, err := c.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt.TreeConversion,
		UserPrompt:  "Convert this planning document to a task tree:\n\n" + planDocument,
		MaxTokens:   16384,
		Temperature: 0.3,
	})
	if err != nil {
		return "", err
	}
	return project.EnforceTreeHierarchy(resp.Content)
}
