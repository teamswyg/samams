package inmemory

import (
	"context"
	"sync"

	appMaal "server/internal/app/maal"
)

var _ appMaal.MaalRecordRepository = (*MaalRepository)(nil)

// MaalRepository is an in-memory implementation of MaalRecordRepository.
type MaalRepository struct {
	mu      sync.RWMutex
	records []appMaal.MaalRecord
}

func NewMaalRepository() *MaalRepository {
	return &MaalRepository{}
}

func (r *MaalRepository) Save(_ context.Context, record *appMaal.MaalRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, *record)
	return nil
}

func (r *MaalRepository) FindByProject(_ context.Context, projectID string) ([]appMaal.MaalRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []appMaal.MaalRecord
	for _, rec := range r.records {
		if rec.ProjectID == projectID {
			result = append(result, rec)
		}
	}
	return result, nil
}
