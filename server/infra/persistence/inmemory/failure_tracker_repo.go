package inmemory

import (
	"context"
	"sync"

	"server/internal/domain/shared"
	domainStrategy "server/internal/domain/strategy"
	appStrategy "server/internal/app/strategy"
)

var _ appStrategy.FailureTrackerRepository = (*FailureTrackerRepository)(nil)

// FailureTrackerRepository is an in-memory implementation of FailureTrackerRepository.
type FailureTrackerRepository struct {
	mu    sync.RWMutex
	store map[string]*domainStrategy.RepeatedFailureTracker
}

func NewFailureTrackerRepository() *FailureTrackerRepository {
	return &FailureTrackerRepository{
		store: make(map[string]*domainStrategy.RepeatedFailureTracker),
	}
}

func (r *FailureTrackerRepository) FindBySubject(_ context.Context, subjectID string) (*domainStrategy.RepeatedFailureTracker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.store[subjectID]
	if !ok {
		return nil, shared.NotFoundError{Resource: "failure_tracker", ID: subjectID}
	}
	return t, nil
}

func (r *FailureTrackerRepository) Save(_ context.Context, t *domainStrategy.RepeatedFailureTracker) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[t.SubjectID] = t
	return nil
}
