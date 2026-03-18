package agent

// AnalyzeLogsCommand triggers MAAL log analysis.
type AnalyzeLogsCommand struct {
	Logs string
}

// GeneratePlanCommand triggers project plan generation.
type GeneratePlanCommand struct {
	Prompt string
}

// ConvertPlanToTreeCommand converts a plan document into a task tree.
type ConvertPlanToTreeCommand struct {
	PlanDocument string
}

// SummarizeCommand triggers content summarization.
type SummarizeCommand struct {
	Content string
}

// GenerateFrontierCommand generates a frontier command for a child task.
type GenerateFrontierCommand struct {
	AccumulatedSummary string
	ChildTask          string
}

// ChatCommand handles conversational planning assistant messages.
type ChatCommand struct {
	Message string
	Context string
}
