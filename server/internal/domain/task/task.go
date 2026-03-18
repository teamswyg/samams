package task

import (
	"time"

	"server/internal/domain/shared"
)

// Priority represents a simple priority level for a task.
type Priority int32

const (
	PriorityUnknown Priority = iota
	PriorityLow
	PriorityNormal
	PriorityHigh
)

// TaskSummary represents an LLM/user generated summary attached to a task.
type TaskSummary struct {
	Title       string
	Description string
	// Source can track whether this summary came from user input, planner, etc.
	Source string
}

// Task is the main task aggregate.
type Task struct {
	ID          shared.TaskID
	TenantID    shared.TenantID
	ProjectID   shared.ProjectID
	Title       string
	Description string
	Status      Status

	ParentID  *shared.TaskID
	Summary   *TaskSummary
	Execution *ExecutionContext

	CreatedAt time.Time
	UpdatedAt time.Time
	DueAt     *time.Time

	Priority   Priority
	RetryCount int32
}

// NewTask constructs a new task with basic validation and initial state.
func NewTask(
	clock shared.Clock,
	id shared.TaskID,
	tenantID shared.TenantID,
	projectID shared.ProjectID,
	title string,
	description string,
	parentID *shared.TaskID,
) (*Task, error) {
	if tenantID == "" {
		return nil, shared.ValidationError{Msg: "tenant_id is required"}
	}
	if projectID == "" {
		return nil, shared.ValidationError{Msg: "project_id is required"}
	}
	if title == "" {
		return nil, shared.ValidationError{Msg: "title is required"}
	}

	now := clock.Now()

	return &Task{
		ID:          id,
		TenantID:    tenantID,
		ProjectID:   projectID,
		Title:       title,
		Description: description,
		Status:      StatusCreated,
		ParentID:    parentID,
		CreatedAt:   now,
		UpdatedAt:   now,
		Priority:    PriorityNormal,
	}, nil
}

// ChangeStatus validates and applies a status transition.
func (t *Task) ChangeStatus(clock shared.Clock, next Status) error {
	if t == nil {
		return shared.ValidationError{Msg: "task is nil"}
	}

	// Simple state machine – can be refined as needed.
	switch t.Status {
	case StatusCreated:
		if next != StatusInProgress && next != StatusDone && next != StatusStopped && next != StatusHardStopped {
			return shared.ConflictError{Msg: "invalid transition from Created"}
		}
	case StatusInProgress:
		if next != StatusDone && next != StatusStopped && next != StatusHardStopped && next != StatusCancelled && next != StatusRedistributed {
			return shared.ConflictError{Msg: "invalid transition from InProgress"}
		}
	case StatusDone, StatusStopped, StatusHardStopped, StatusCancelled, StatusRedistributed:
		// Terminal states; no further transitions allowed.
		if next != t.Status {
			return shared.ConflictError{Msg: "cannot transition from terminal status"}
		}
	default:
		return shared.ConflictError{Msg: "unknown current status"}
	}

	t.Status = next
	t.UpdatedAt = clock.Now()
	return nil
}

// ApplySummary sets or updates the task summary.
func (t *Task) ApplySummary(clock shared.Clock, summary TaskSummary) error {
	if t == nil {
		return shared.ValidationError{Msg: "task is nil"}
	}
	if summary.Title == "" && summary.Description == "" {
		return shared.ValidationError{Msg: "summary must have title or description"}
	}

	t.Summary = &summary
	t.UpdatedAt = clock.Now()
	return nil
}

// IncrementRetry increases the retry count for this task.
func (t *Task) IncrementRetry(clock shared.Clock) error {
	if t == nil {
		return shared.ValidationError{Msg: "task is nil"}
	}
	t.RetryCount++
	t.UpdatedAt = clock.Now()
	return nil
}


