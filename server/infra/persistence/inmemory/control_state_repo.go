package inmemory

import (
	"context"
	"sync"

	domainControl "server/internal/domain/control"
	"server/internal/domain/shared"
	appControl "server/internal/app/control"
)

var _ appControl.ControlStateRepository = (*ControlStateRepository)(nil)

// ControlStateRepository is an in-memory implementation of ControlStateRepository.
type ControlStateRepository struct {
	mu    sync.RWMutex
	store map[string]*domainControl.ControlStateAggregate
}

func NewControlStateRepository() *ControlStateRepository {
	return &ControlStateRepository{
		store: make(map[string]*domainControl.ControlStateAggregate),
	}
}

func (r *ControlStateRepository) Save(_ context.Context, cs *domainControl.ControlStateAggregate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[cs.ID] = cs
	return nil
}

func (r *ControlStateRepository) Load(_ context.Context, id string) (*domainControl.ControlStateAggregate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Search by aggregate ID first.
	if cs, ok := r.store[id]; ok {
		return cs, nil
	}

	// Then search by snapshot ID.
	for _, cs := range r.store {
		if cs.FindSnapshot(id) != nil {
			return cs, nil
		}
	}

	return nil, shared.NotFoundError{Resource: "control_state", ID: id}
}

func (r *ControlStateRepository) FindByProject(_ context.Context, projectID string) (*domainControl.ControlStateAggregate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, cs := range r.store {
		if cs.ProjectID == projectID {
			return cs, nil
		}
	}
	return nil, shared.NotFoundError{Resource: "control_state", ID: projectID}
}
