package domain

import (
	"fmt"
	"sync"
)

// transitions defines which agent state transitions are allowed.
var transitions = map[AgentStatus][]AgentStatus{
	AgentStatusIdle:     {AgentStatusStarting},
	AgentStatusStarting: {AgentStatusRunning, AgentStatusError},
	AgentStatusRunning:  {AgentStatusPaused, AgentStatusStopped, AgentStatusError, AgentStatusIdle},
	AgentStatusPaused:   {AgentStatusStarting, AgentStatusStopped},
	AgentStatusStopped:  {AgentStatusStarting},
	AgentStatusError:    {AgentStatusStarting},
}

// TransitionCallback is called after a successful state transition.
type TransitionCallback func(old, new AgentStatus, reason string)

// StateMachine manages agent state transitions with validation.
type StateMachine struct {
	mu           sync.Mutex
	current      AgentStatus
	onTransition TransitionCallback
}

func NewStateMachine(cb TransitionCallback) *StateMachine {
	return &StateMachine{
		current:      AgentStatusIdle,
		onTransition: cb,
	}
}

func (m *StateMachine) Current() AgentStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// Transition attempts to move to the target state.
// Returns an error if the transition is not allowed.
func (m *StateMachine) Transition(target AgentStatus, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	allowed := transitions[m.current]
	for _, s := range allowed {
		if s == target {
			old := m.current
			m.current = target
			if m.onTransition != nil {
				m.onTransition(old, target, reason)
			}
			return nil
		}
	}

	return fmt.Errorf("invalid transition: %s → %s", m.current, target)
}

// ForceSet sets the state without validation (for recovery scenarios).
func (m *StateMachine) ForceSet(status AgentStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = status
}

// ── Task State Machine ──────────────────────────────────────────

// taskTransitions defines valid task status transitions.
var taskTransitions = map[TaskStatus][]TaskStatus{
	TaskStatusPending:   {TaskStatusRunning, TaskStatusCancelled},
	TaskStatusRunning:   {TaskStatusPaused, TaskStatusDone, TaskStatusStopped, TaskStatusError, TaskStatusScaling, TaskStatusResetting, TaskStatusCancelled},
	TaskStatusPaused:    {TaskStatusRunning, TaskStatusStopped, TaskStatusCancelled},
	TaskStatusScaling:   {TaskStatusRunning, TaskStatusError},
	TaskStatusResetting: {TaskStatusRunning, TaskStatusError},
	TaskStatusDone:      {}, // terminal
	TaskStatusStopped:   {TaskStatusRunning, TaskStatusCancelled},
	TaskStatusCancelled: {}, // terminal
	TaskStatusError:     {TaskStatusRunning, TaskStatusResetting},
}

// TransitionTask validates and performs a task status transition.
// Returns an error if the transition is not allowed.
func TransitionTask(task *Task, target TaskStatus) error {
	allowed := taskTransitions[task.Status]
	for _, s := range allowed {
		if s == target {
			task.Status = target
			return nil
		}
	}
	return fmt.Errorf("invalid task transition: %s → %s", task.Status, target)
}
