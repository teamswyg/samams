package localstore

import (
	"context"
	"fmt"

	domainProject "server/internal/domain/project"
	"server/internal/domain/shared"
	appProject "server/internal/app/project"
)

var _ appProject.ProjectRepository = (*ProjectRepository)(nil)

// ProjectRepository stores projects as JSON files.
// Key: projects/{id}.json
type ProjectRepository struct {
	store *Store
}

func NewProjectRepository(store *Store) *ProjectRepository {
	return &ProjectRepository{store: store}
}

func (r *ProjectRepository) key(id domainProject.ID) string {
	return fmt.Sprintf("projects/%s.json", id)
}

func (r *ProjectRepository) GetByID(_ context.Context, id domainProject.ID) (*domainProject.Project, error) {
	var p domainProject.Project
	if err := r.store.Get(r.key(id), &p); err != nil {
		if err == ErrNotFound {
			return nil, shared.NotFoundError{Resource: "project", ID: string(id)}
		}
		return nil, err
	}
	return &p, nil
}

func (r *ProjectRepository) ListByTenant(_ context.Context, tenantID shared.TenantID) ([]*domainProject.Project, error) {
	keys, err := r.store.List("projects")
	if err != nil {
		return nil, err
	}
	var result []*domainProject.Project
	for _, k := range keys {
		var p domainProject.Project
		if err := r.store.Get(k, &p); err != nil {
			continue
		}
		if p.TenantID == tenantID {
			result = append(result, &p)
		}
	}
	return result, nil
}

func (r *ProjectRepository) Save(_ context.Context, p *domainProject.Project) error {
	return r.store.Put(r.key(p.ID), p)
}
