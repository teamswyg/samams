package task

import (
	"context"

	domainShared "server/internal/domain/shared"
	domainTask "server/internal/domain/task"
)

// TaskRepository is the primary persistence port for tasks.
type TaskRepository interface {
	GetByID(ctx context.Context, id domainShared.TaskID) (*domainTask.Task, error)
	Save(ctx context.Context, t *domainTask.Task) error
	FindChildren(ctx context.Context, parentID domainShared.TaskID) ([]*domainTask.Task, error)
	FindAll(ctx context.Context) ([]*domainTask.Task, error)
	NextID(ctx context.Context) (domainShared.TaskID, error)
}

// ContextPlanner defines how to derive a child task context and summary from a parent.
type ContextPlanner interface {
	BuildChildSummary(ctx context.Context, parent *domainTask.Task, child SummaryInput) (domainTask.TaskSummary, error)
}

// SummaryGenerator generates summaries for cascading execution.
type SummaryGenerator interface {
	Summarize(ctx context.Context, content string) (string, error)
	GenerateFrontierCommand(ctx context.Context, accumulatedSummary string, childTask string) (string, error)
}

// SummaryInput is the minimal information needed to construct a task summary.
type SummaryInput struct {
	Title       string
	Description string
}

// TaskContextInput aggregates all inputs the planner may need.
type TaskContextInput struct {
	ParentTaskID domainShared.TaskID
	TitleHint    string
	BodyHint     string
}

// TaskContextOutput represents the planned context/prompt for a child task.
type TaskContextOutput struct {
	PlannedTitle       string
	PlannedDescription string
}
