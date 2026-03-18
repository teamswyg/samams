package task

import "server/internal/domain/shared"

// ExecutionContext holds cascading execution data for a task.
type ExecutionContext struct {
	AccumulatedSummary string // Summary cascaded from all ancestors
	OwnSummary         string // Summary of this task's execution
	FrontierCommand    string // Command this task received
	ExecutionResult    string // Raw result of execution
	Agent              string // Assigned agent name
	ChildIDs           []shared.TaskID
}

// ReceiveFrontier sets the accumulated summary and frontier command from parent.
func (t *Task) ReceiveFrontier(clock shared.Clock, accumulatedSummary, frontierCommand string) {
	if t.Execution == nil {
		t.Execution = &ExecutionContext{}
	}
	t.Execution.AccumulatedSummary = accumulatedSummary
	t.Execution.FrontierCommand = frontierCommand
	t.Status = StatusInProgress
	t.UpdatedAt = clock.Now()
}

// CompleteExecution marks the task as done with its own summary.
func (t *Task) CompleteExecution(clock shared.Clock, executionResult, ownSummary string) {
	if t.Execution == nil {
		t.Execution = &ExecutionContext{}
	}
	t.Execution.ExecutionResult = executionResult
	t.Execution.OwnSummary = ownSummary
	t.Status = StatusDone
	t.UpdatedAt = clock.Now()
}

// FailExecution marks the task as stopped with a reason.
func (t *Task) FailExecution(clock shared.Clock, reason string) {
	if t.Execution == nil {
		t.Execution = &ExecutionContext{}
	}
	t.Execution.ExecutionResult = reason
	t.Status = StatusStopped
	t.UpdatedAt = clock.Now()
}

// BuildChildContext returns the accumulated summary to pass to children.
func (t *Task) BuildChildContext() string {
	if t.Execution == nil {
		return ""
	}
	if t.Execution.AccumulatedSummary == "" {
		return t.Execution.OwnSummary
	}
	return t.Execution.AccumulatedSummary + "\n\n---\n\n" + t.Execution.OwnSummary
}

// AddChild registers a child task ID.
func (t *Task) AddChild(childID shared.TaskID) {
	if t.Execution == nil {
		t.Execution = &ExecutionContext{}
	}
	t.Execution.ChildIDs = append(t.Execution.ChildIDs, childID)
}

// IsLeaf returns true if this task has no children.
func (t *Task) IsLeaf() bool {
	return t.Execution == nil || len(t.Execution.ChildIDs) == 0
}
