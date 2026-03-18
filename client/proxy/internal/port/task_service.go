package port

import (
	"context"

	"proxy/internal/domain"
)

// TaskService is the primary (driving) port — defines what callers can do.
type TaskService interface {
	CreateTask(ctx context.Context, params domain.CreateTaskParams) (*domain.TaskSummary, error)
	ListTasks() []*domain.Task
	GetTask(taskID string) (*domain.TaskSummary, error)
	ScaleTask(ctx context.Context, taskID string, params domain.ScaleTaskParams) (*domain.TaskSummary, error)
	StopTask(ctx context.Context, taskID string, opts *domain.StopTaskOptions) error
	PauseTask(ctx context.Context, taskID string) error
	ResumeTask(ctx context.Context, taskID string) (*domain.TaskSummary, error)
	CancelTask(ctx context.Context, taskID string, reason, cancelledBy string) error
	ResetTask(ctx context.Context, taskID string) (*domain.TaskSummary, error)
	IncrementRetryCount(ctx context.Context, taskID string) (int, bool)
	UpdateTaskSummary(ctx context.Context, taskID string, summary string) error
	SetContextPlanned(ctx context.Context, taskID string) error

	ListAgents() []domain.Agent
	GetAgent(agentID string) (*domain.Agent, []string, error)
	StopAgent(ctx context.Context, agentID string) error
	AppendAgentInput(agentID, input string) error

	GetMaalByTaskID(taskID string) []domain.MaalRecord
	GetMaalByAgentID(agentID string) []domain.MaalRecord
	GetRecentLogs() []domain.LogEntry
	ListNotifications() []domain.Notification
	StrategyPauseAll(ctx context.Context, participantNodeUIDs []string) error
	StrategyApplyDecision(ctx context.Context, decision domain.StrategyDecision) error

	MergeMilestone(ctx context.Context, branchName, targetBranch string) error
	GetAgentLogs(agentID string) ([]string, error)
	CreateSkeleton(spec domain.SkeletonSpec) (string, error)
}
