package inmemory

import (
	"context"
	"sync"

	"server/internal/domain/shared"
	"server/internal/domain/usage"
	appUsage "server/internal/app/usage"
)

var _ appUsage.SnapshotRepository = (*UsageRepository)(nil)

// UsageRepository is an in-memory implementation of SnapshotRepository.
type UsageRepository struct {
	mu    sync.RWMutex
	store map[string]usage.Snapshot
}

func NewUsageRepository() *UsageRepository {
	return &UsageRepository{
		store: make(map[string]usage.Snapshot),
	}
}

func key(userID, projectID string) string {
	return userID + ":" + projectID
}

func (r *UsageRepository) Get(_ context.Context, userID, projectID string) (usage.Snapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.store[key(userID, projectID)]
	if !ok {
		return usage.Snapshot{}, shared.NotFoundError{Resource: "usage_snapshot", ID: key(userID, projectID)}
	}
	return s, nil
}

func (r *UsageRepository) Save(_ context.Context, s usage.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[key(s.UserID, s.ProjectID)] = s
	return nil
}
