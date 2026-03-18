package domain

import (
	"fmt"
	"log"
	"time"
)

type AgentStatus string

const (
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusStarting AgentStatus = "starting"
	AgentStatusRunning  AgentStatus = "running"
	AgentStatusPaused   AgentStatus = "paused"
	AgentStatusStopped  AgentStatus = "stopped"
	AgentStatusError    AgentStatus = "error"
)

type AgentType string

const (
	AgentTypeCursor   AgentType = "cursor"
	AgentTypeClaude   AgentType = "claude"
	AgentTypeOpenCode AgentType = "opencode"
)

type AgentMode string

const (
	AgentModePlan    AgentMode = "plan"
	AgentModeDebug   AgentMode = "debug"
	AgentModeExecute AgentMode = "execute"
)

// AgentNamePool is a pool of human names for agents.
var AgentNamePool = []string{
	"Alex", "Jordan", "Riley", "Morgan", "Casey", "Quinn",
	"Avery", "Blake", "Drew", "Sage", "Reese", "Skyler",
}

// PickAvailableName returns the first name not currently in use.
// usedNames is the set of names assigned to active agents.
func PickAvailableName(usedNames map[string]bool) string {
	for _, name := range AgentNamePool {
		if !usedNames[name] {
			return name
		}
	}
	// All 12 names used — fallback to numbered.
	return fmt.Sprintf("Agent-%d", len(usedNames)+1)
}


type Agent struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	TaskID    string      `json:"taskId"`
	TaskName  string      `json:"taskName,omitempty"`
	NodeUID   string      `json:"nodeUid,omitempty"`
	AgentType AgentType   `json:"agentType"`
	Mode      AgentMode   `json:"mode"`
	Status    AgentStatus `json:"status"`
	CreatedAt time.Time   `json:"createdAt"`
	StoppedAt *time.Time  `json:"stoppedAt,omitempty"`

	sm *StateMachine
}

// Transition validates the state change through the state machine, then updates Status.
func (a *Agent) Transition(target AgentStatus, reason string) error {
	if a.sm == nil {
		a.Status = target
		return nil
	}
	if err := a.sm.Transition(target, reason); err != nil {
		return err
	}
	a.Status = target
	return nil
}

// InitStateMachine creates and attaches a state machine to the agent.
func (a *Agent) InitStateMachine() {
	a.sm = NewStateMachine(func(old, new AgentStatus, reason string) {
		log.Printf("[state] agent %s: %s → %s (%s)", a.ID, old, new, reason)
	})
}

// Snapshot returns a copy of the agent suitable for JSON responses (without internal state).
func (a *Agent) Snapshot() Agent {
	var stoppedAt *time.Time
	if a.StoppedAt != nil {
		t := *a.StoppedAt
		stoppedAt = &t
	}
	return Agent{
		ID:        a.ID,
		Name:      a.Name,
		TaskID:    a.TaskID,
		TaskName:  a.TaskName,
		NodeUID:   a.NodeUID,
		AgentType: a.AgentType,
		Mode:      a.Mode,
		Status:    a.Status,
		CreatedAt: a.CreatedAt,
		StoppedAt: stoppedAt,
	}
}
