package domain

// StrategyDecision is the per-task restructuring decision from the server.
type StrategyDecision struct {
	TaskActions []TaskAction `json:"taskActions"`
}

// TaskAction describes what to do with a single worker agent.
type TaskAction struct {
	NodeUID   string `json:"nodeUid"`
	Action    string `json:"action"` // keep, reset_and_retry, cancel
	NewPrompt string `json:"newPrompt,omitempty"`
}
