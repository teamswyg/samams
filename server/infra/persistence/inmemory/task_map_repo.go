package inmemory

import (
	"context"
	"sync"

	domainProject "server/internal/domain/project"
	"server/internal/domain/shared"
	appProject "server/internal/app/project"
)

var _ appProject.TaskMapRepository = (*TaskMapRepository)(nil)

// TaskMapRepository is an in-memory implementation of TaskMapRepository.
type TaskMapRepository struct {
	mu    sync.RWMutex
	store map[string]*domainProject.TaskMapAggregate
}

func NewTaskMapRepository() *TaskMapRepository {
	return &TaskMapRepository{
		store: make(map[string]*domainProject.TaskMapAggregate),
	}
}

func (r *TaskMapRepository) Save(_ context.Context, m *domainProject.TaskMapAggregate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[m.ID] = m
	return nil
}

func (r *TaskMapRepository) FindByID(_ context.Context, id string) (*domainProject.TaskMapAggregate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.store[id]
	if !ok {
		return nil, shared.NotFoundError{Resource: "task_map", ID: id}
	}
	return m, nil
}
