package control

import (
	"context"

	domainControl "server/internal/domain/control"
	"server/internal/domain/shared"
)

// ControlStateRepository persists control state aggregates.
type ControlStateRepository interface {
	Save(ctx context.Context, cs *domainControl.ControlStateAggregate) error
	Load(ctx context.Context, id string) (*domainControl.ControlStateAggregate, error)
	FindByProject(ctx context.Context, projectID string) (*domainControl.ControlStateAggregate, error)
}

// ControlEventBus publishes control domain events.
type ControlEventBus interface {
	PublishAll(ctx context.Context, events []shared.DomainEvent) error
}
