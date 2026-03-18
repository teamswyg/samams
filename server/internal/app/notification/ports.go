package notification

import (
	"context"

	"server/internal/domain/shared"
)

// Repository persists notification records.
type Repository interface {
	Save(ctx context.Context, n *Notification) error
	FindByUser(ctx context.Context, userID string) ([]*Notification, error)
}

// EventBus publishes notification domain events.
type EventBus interface {
	PublishAll(ctx context.Context, events []shared.DomainEvent) error
}

// Notification is the domain model for user-facing notifications.
type Notification struct {
	ID        string
	UserID    string
	Title     string
	Body      string
	Severity  string // "info" | "warning" | "error"
	Read      bool
	CreatedAt int64
}
