package stub

import (
	"context"
	"encoding/json"
	"fmt"

	"server/internal/app/agent"
	"server/internal/domain/llm"
	"server/internal/domain/project"
)

// Compile-time interface checks.
var (
	_ agent.LogAnalyzer = (*StubProvider)(nil)
	_ agent.Planner     = (*StubProvider)(nil)
	_ agent.Summarizer  = (*StubProvider)(nil)
)

// StubProvider returns canned responses for local development without API keys.
type StubProvider struct{}

func New() *StubProvider { return &StubProvider{} }

func (s *StubProvider) Role() llm.Role { return llm.RolePlanner }
func (s *StubProvider) Name() string   { return "stub/local" }

func (s *StubProvider) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{
		Content:      fmt.Sprintf("[stub] Processed prompt (%d chars)", len(req.UserPrompt)),
		FinishReason: "stop",
		TokensUsed:   0,
		Model:        "stub",
	}, nil
}

func (s *StubProvider) AnalyzeLogs(_ context.Context, logs string) (string, error) {
	result := map[string]any{
		"anomalies":       []string{},
		"patterns":        []string{"Normal operation detected"},
		"conflicts":       []string{},
		"recommendations": []string{"System is running normally"},
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

func (s *StubProvider) GeneratePlan(_ context.Context, prompt string) (string, error) {
	plan := map[string]any{
		"title":       "Generated Plan",
		"description": fmt.Sprintf("Plan based on: %s", truncate(prompt, 100)),
		"phases": []map[string]string{
			{"name": "Phase 1: Analysis", "description": "Analyze requirements"},
			{"name": "Phase 2: Implementation", "description": "Implement core features"},
			{"name": "Phase 3: Testing", "description": "Test and validate"},
		},
	}
	b, _ := json.MarshalIndent(plan, "", "  ")
	return string(b), nil
}

func (s *StubProvider) ConvertToNodeTree(_ context.Context, planDoc string) (string, error) {
	tree := map[string]any{
		"nodes": []map[string]any{
			{"id": "prop-1", "uid": "PROP-0001", "type": "proposal", "summary": "Project Root", "agent": "System", "status": "pending", "priority": "high", "parentId": nil, "boundedContext": "root"},
			{"id": "mlst-1", "uid": "MLST-0001-A", "type": "milestone", "summary": "Milestone 1: Setup & Infrastructure", "agent": "Unassigned", "status": "pending", "priority": "high", "parentId": "prop-1", "boundedContext": "infra"},
			{"id": "mlst-2", "uid": "MLST-0002-B", "type": "milestone", "summary": "Milestone 2: Core Logic", "agent": "Unassigned", "status": "pending", "priority": "high", "parentId": "prop-1", "boundedContext": "core"},
			{"id": "task-1", "uid": "TASK-0001-1", "type": "task", "summary": "Setup project scaffolding", "agent": "Cursor Agent", "status": "pending", "priority": "high", "parentId": "mlst-1", "boundedContext": "infra"},
			{"id": "task-2", "uid": "TASK-0002-1", "type": "task", "summary": "Configure CI/CD pipeline", "agent": "Cursor Agent", "status": "pending", "priority": "medium", "parentId": "mlst-1", "boundedContext": "infra"},
			{"id": "task-3", "uid": "TASK-0003-1", "type": "task", "summary": "Implement domain models", "agent": "Claude Code", "status": "pending", "priority": "high", "parentId": "mlst-2", "boundedContext": "core"},
			{"id": "task-4", "uid": "TASK-0004-1", "type": "task", "summary": "Write unit tests", "agent": "Claude Code", "status": "pending", "priority": "medium", "parentId": "mlst-2", "boundedContext": "core"},
		},
	}
	b, _ := json.MarshalIndent(tree, "", "  ")
	return project.EnforceTreeHierarchy(string(b))
}

func (s *StubProvider) AnalyzeReview(_ context.Context, _ string) (string, error) {
	return `{"decision":"APPROVED","reasoning":"stub: auto-approved","newTasks":[]}`, nil
}

func (s *StubProvider) Summarize(_ context.Context, content string) (string, error) {
	return fmt.Sprintf("Summary of content (%d chars): %s", len(content), truncate(content, 200)), nil
}

func (s *StubProvider) GenerateFrontierCommand(_ context.Context, accumulatedSummary, childTask string) (string, error) {
	return fmt.Sprintf("Execute task: %s (context: %s)", truncate(childTask, 100), truncate(accumulatedSummary, 100)), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
