package shared

import "time"

// DomainEvent is the canonical event envelope used across all bounded contexts.
type DomainEvent struct {
	EventName   string
	OccurredAt  time.Time
	ActorType   string
	ActorID     string
	SubjectType string
	SubjectID   string
	Severity    string
	Payload     any
}

// NewDomainEvent constructs a domain event with the current clock time.
func NewDomainEvent(clock Clock, name, actorType, actorID, subjectType, subjectID, severity string, payload any) DomainEvent {
	return DomainEvent{
		EventName:   name,
		OccurredAt:  clock.Now(),
		ActorType:   actorType,
		ActorID:     actorID,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Severity:    severity,
		Payload:     payload,
	}
}
