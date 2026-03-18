package control

import (
	"time"

	"server/internal/domain/shared"
)

// Snapshot holds a captured context state for later resumption.
type Snapshot struct {
	StateID string
	Payload string
	SavedAt time.Time
}

// ControlStateAggregate manages context capture and resumption.
type ControlStateAggregate struct {
	ID        string
	ProjectID string
	Snapshots []Snapshot
	events    []shared.DomainEvent
}

// NewControlState creates a new control state aggregate.
func NewControlState(projectID string) *ControlStateAggregate {
	return &ControlStateAggregate{
		ID:        shared.GenerateID(),
		ProjectID: projectID,
	}
}

// Capture saves a context snapshot.
func (c *ControlStateAggregate) Capture(clock shared.Clock, payload, reason string) Snapshot {
	snap := Snapshot{
		StateID: shared.GenerateID(),
		Payload: payload,
		SavedAt: clock.Now(),
	}
	c.Snapshots = append(c.Snapshots, snap)
	c.addEvent(clock, "ControlStateCaptured", map[string]string{
		"state_id": snap.StateID,
		"reason":   reason,
	})
	return snap
}

// PlanReset captures state and emits reset event.
func (c *ControlStateAggregate) PlanReset(clock shared.Clock, reason, contextPayload string) Snapshot {
	if contextPayload == "" {
		contextPayload = "reset_reason:" + reason
	}
	snap := c.Capture(clock, contextPayload, reason)
	c.addEvent(clock, "ControlResetRequested", map[string]string{
		"state_id": snap.StateID,
		"reason":   reason,
	})
	return snap
}

// FindSnapshot finds a snapshot by ID.
func (c *ControlStateAggregate) FindSnapshot(stateID string) *Snapshot {
	for i := range c.Snapshots {
		if c.Snapshots[i].StateID == stateID {
			return &c.Snapshots[i]
		}
	}
	return nil
}

// PlanResume emits a state resume event.
func (c *ControlStateAggregate) PlanResume(clock shared.Clock, stateID string) {
	c.addEvent(clock, "StateLoadedAndApplied", map[string]string{
		"state_id": stateID,
	})
}

// PullDomainEvents returns and clears pending events.
func (c *ControlStateAggregate) PullDomainEvents() []shared.DomainEvent {
	out := make([]shared.DomainEvent, len(c.events))
	copy(out, c.events)
	c.events = c.events[:0]
	return out
}

func (c *ControlStateAggregate) addEvent(clock shared.Clock, name string, payload any) {
	c.events = append(c.events, shared.NewDomainEvent(
		clock, name, "control", c.ID, "control_state", c.ID, "info", payload,
	))
}
