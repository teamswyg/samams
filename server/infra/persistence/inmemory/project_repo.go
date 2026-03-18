package inmemory

import (
	"context"
	"sync"

	domainProject "server/internal/domain/project"
	"server/internal/domain/shared"
	appProject "server/internal/app/project"
)

var _ appProject.ProjectRepository = (*ProjectRepository)(nil)

// ProjectRepository is an in-memory implementation of ProjectRepository.
type ProjectRepository struct {
	mu    sync.RWMutex
	store map[domainProject.ID]*domainProject.Project
}

func NewProjectRepository() *ProjectRepository {
	return &ProjectRepository{
		store: make(map[domainProject.ID]*domainProject.Project),
	}
}

func (r *ProjectRepository) GetByID(_ context.Context, id domainProject.ID) (*domainProject.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.store[id]
	if !ok {
		return nil, shared.NotFoundError{Resource: "project", ID: string(id)}
	}
	return p, nil
}

func (r *ProjectRepository) ListByTenant(_ context.Context, tenantID shared.TenantID) ([]*domainProject.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*domainProject.Project
	for _, p := range r.store {
		if p.TenantID == tenantID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (r *ProjectRepository) Save(_ context.Context, p *domainProject.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[p.ID] = p
	return nil
}
