package strategy

import (
	"time"

	"server/internal/domain/shared"
)

// MeetingStatus represents the lifecycle of a strategy meeting.
type MeetingStatus int32

const (
	MeetingStatusUnknown MeetingStatus = iota
	MeetingStatusRequested
	MeetingStatusActive
	MeetingStatusResolved
)

type MeetingID string

// StrategyMeeting captures a strategy discussion around a project.
type StrategyMeeting struct {
	ID        MeetingID
	ProjectID string
	CreatedBy string

	Topic       string
	Description string
	Decision    string

	Status    MeetingStatus
	CreatedAt time.Time
	UpdatedAt time.Time

	events []shared.DomainEvent
}

// NewStrategyMeeting creates a meeting in Requested status.
func NewStrategyMeeting(clock shared.Clock, id MeetingID, projectID, createdBy, topic, description string) (*StrategyMeeting, error) {
	if projectID == "" {
		return nil, shared.ValidationError{Msg: "project_id is required"}
	}
	if topic == "" {
		return nil, shared.ValidationError{Msg: "topic is required"}
	}
	now := clock.Now()
	m := &StrategyMeeting{
		ID:          id,
		ProjectID:   projectID,
		CreatedBy:   createdBy,
		Topic:       topic,
		Description: description,
		Status:      MeetingStatusRequested,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.addEvent(clock, "StrategyMeetingRequested", map[string]string{
		"meeting_id": string(id),
		"topic":      topic,
	})
	return m, nil
}

// Start transitions the meeting to Active status.
func (m *StrategyMeeting) Start(clock shared.Clock) error {
	if m.Status != MeetingStatusRequested {
		return shared.ConflictError{Msg: "meeting can only be started from requested status"}
	}
	m.Status = MeetingStatusActive
	m.UpdatedAt = clock.Now()
	m.addEvent(clock, "StrategyMeetingStarted", map[string]string{
		"meeting_id": string(m.ID),
	})
	return nil
}

// Resolve transitions the meeting to Resolved status with a decision.
func (m *StrategyMeeting) Resolve(clock shared.Clock, decision string) error {
	if m.Status != MeetingStatusActive {
		return shared.ConflictError{Msg: "meeting can only be resolved from active status"}
	}
	m.Status = MeetingStatusResolved
	m.Decision = decision
	m.UpdatedAt = clock.Now()
	m.addEvent(clock, "StrategyMeetingResolved", map[string]string{
		"meeting_id": string(m.ID),
		"decision":   decision,
	})
	return nil
}

// PullDomainEvents returns and clears pending events.
func (m *StrategyMeeting) PullDomainEvents() []shared.DomainEvent {
	out := make([]shared.DomainEvent, len(m.events))
	copy(out, m.events)
	m.events = m.events[:0]
	return out
}

func (m *StrategyMeeting) addEvent(clock shared.Clock, name string, payload any) {
	m.events = append(m.events, shared.NewDomainEvent(
		clock, name, "strategy", string(m.ID), "strategy_meeting", string(m.ID), "info", payload,
	))
}

