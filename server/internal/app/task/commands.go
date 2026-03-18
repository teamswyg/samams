package task

import (
	domainShared "server/internal/domain/shared"
	domainTask "server/internal/domain/task"
)

// CreateTaskCommand is the input model for creating a task.
type CreateTaskCommand struct {
	TenantID    domainShared.TenantID
	ProjectID   domainShared.ProjectID
	Title       string
	Description string
	ParentID    *domainShared.TaskID
	CreatorID   domainShared.UserID
}

// UpdateTaskStatusCommand updates the status of an existing task.
type UpdateTaskStatusCommand struct {
	TaskID domainShared.TaskID
	Status domainTask.Status
	Reason string
}

// SummarizeTaskCommand triggers (re-)summarization of a task.
type SummarizeTaskCommand struct {
	TaskID  domainShared.TaskID
	Trigger string
}

