package agent

import (
	"context"

	"server/internal/domain/llm"
	"server/infra/out/llm/prompt"
)

type Service struct {
	analyzer   LogAnalyzer
	planner    Planner
	summarizer Summarizer
}

func NewService(analyzer LogAnalyzer, planner Planner, summarizer Summarizer) *Service {
	return &Service{
		analyzer:   analyzer,
		planner:    planner,
		summarizer: summarizer,
	}
}

// AnalyzeLogs delegates MAAL log analysis to the LogAnalyzer (OpenAI).
func (s *Service) AnalyzeLogs(ctx context.Context, cmd AnalyzeLogsCommand) (string, error) {
	return s.analyzer.AnalyzeLogs(ctx, cmd.Logs)
}

// GeneratePlan delegates project plan generation to the Planner (Anthropic).
func (s *Service) GeneratePlan(ctx context.Context, cmd GeneratePlanCommand) (string, error) {
	return s.planner.GeneratePlan(ctx, cmd.Prompt)
}

// ConvertPlanToTree delegates plan-to-tree conversion to the Planner (Anthropic).
func (s *Service) ConvertPlanToTree(ctx context.Context, cmd ConvertPlanToTreeCommand) (string, error) {
	return s.planner.ConvertToNodeTree(ctx, cmd.PlanDocument)
}

// Summarize delegates content summarization to the Summarizer (Gemini).
func (s *Service) Summarize(ctx context.Context, cmd SummarizeCommand) (string, error) {
	return s.summarizer.Summarize(ctx, cmd.Content)
}

// GenerateFrontier delegates frontier command generation to the Summarizer (Gemini).
func (s *Service) GenerateFrontier(ctx context.Context, cmd GenerateFrontierCommand) (string, error) {
	return s.summarizer.GenerateFrontierCommand(ctx, cmd.AccumulatedSummary, cmd.ChildTask)
}

// Chat handles conversational planning assistant messages via the Planner (Anthropic).
func (s *Service) Chat(ctx context.Context, cmd ChatCommand) (string, error) {
	resp, err := s.planner.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt.Chat,
		UserPrompt:  cmd.Message + "\n\nCurrent planning context:\n" + cmd.Context,
		MaxTokens:   1024,
		Temperature: 0.7,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
