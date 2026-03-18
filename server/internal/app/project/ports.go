package project

import (
	"context"

	domainProject "server/internal/domain/project"
	"server/internal/domain/shared"
)

// ProjectRepository persists projects.
type ProjectRepository interface {
	GetByID(ctx context.Context, id domainProject.ID) (*domainProject.Project, error)
	ListByTenant(ctx context.Context, tenantID shared.TenantID) ([]*domainProject.Project, error)
	Save(ctx context.Context, p *domainProject.Project) error
}

// TaskMapRepository persists task maps.
type TaskMapRepository interface {
	Save(ctx context.Context, m *domainProject.TaskMapAggregate) error
	FindByID(ctx context.Context, id string) (*domainProject.TaskMapAggregate, error)
}

