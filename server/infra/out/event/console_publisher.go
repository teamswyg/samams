package event

import (
	"context"
	"log"

	"server/internal/domain/shared"
	appControl "server/internal/app/control"
	appStrategy "server/internal/app/strategy"
)

// Compile-time interface checks.
var (
	_ appControl.ControlEventBus  = (*ConsolePublisher)(nil)
	_ appStrategy.StrategyEventBus = (*ConsolePublisher)(nil)
)

// ConsolePublisher is a simple event publisher that logs events to stdout.
// It implements all EventBus ports and can be replaced with SQS/EventBridge later.
type ConsolePublisher struct{}

func NewConsolePublisher() *ConsolePublisher {
	return &ConsolePublisher{}
}

// PublishAll logs all domain events to the console.
func (p *ConsolePublisher) PublishAll(_ context.Context, events []shared.DomainEvent) error {
	for _, e := range events {
		log.Printf("[Event] %s | actor=%s:%s subject=%s:%s severity=%s",
			e.EventName, e.ActorType, e.ActorID, e.SubjectType, e.SubjectID, e.Severity)
	}
	return nil
}

