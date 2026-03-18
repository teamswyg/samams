package agent

import (
	"context"

	"server/internal/domain/llm"
)

// AIProvider is the outbound port for AI completions.
type AIProvider interface {
	Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)
	Role() llm.Role
	Name() string
}

// LogAnalyzer analyzes real-time MAAL logs (OpenAI GPT).
type LogAnalyzer interface {
	AnalyzeLogs(ctx context.Context, logs string) (string, error)
}

// Planner creates planning documents and converts them to node trees (Anthropic Claude).
type Planner interface {
	GeneratePlan(ctx context.Context, prompt string) (string, error)
	ConvertToNodeTree(ctx context.Context, planDocument string) (string, error)
	AnalyzeReview(ctx context.Context, reviewContext string) (string, error)
	Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)
}

// Summarizer generates summaries and batch frontiers (Google Gemini Flash).
type Summarizer interface {
	Summarize(ctx context.Context, content string) (string, error)
	GenerateFrontierCommand(ctx context.Context, accumulatedSummary string, childTask string) (string, error)
	Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)
}
