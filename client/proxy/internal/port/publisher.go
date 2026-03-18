package port

import (
	"context"

	"proxy/internal/domain"
)

// Publisher is a secondary (driven) port for publishing domain events.
type Publisher interface {
	Publish(ctx context.Context, e domain.Envelope) error
}

// NoopPublisher does nothing (for tests or when event bus is not configured).
type NoopPublisher struct{}

func (NoopPublisher) Publish(context.Context, domain.Envelope) error { return nil }
