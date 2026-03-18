package localstore

import (
	"context"
	"fmt"
	"time"

	appMaal "server/internal/app/maal"
)

var _ appMaal.MaalRecordRepository = (*MaalRepository)(nil)

// MaalRepository stores MAAL records as JSON files.
// Key: maal/{projectID}/{timestamp}-{id}.json
type MaalRepository struct {
	store *Store
}

func NewMaalRepository(store *Store) *MaalRepository {
	return &MaalRepository{store: store}
}

func (r *MaalRepository) Save(_ context.Context, record *appMaal.MaalRecord) error {
	key := fmt.Sprintf("maal/%s/%d-%s.json", record.ProjectID, time.Now().UnixNano(), record.ID)
	return r.store.Put(key, record)
}

func (r *MaalRepository) FindByProject(_ context.Context, projectID string) ([]appMaal.MaalRecord, error) {
	prefix := fmt.Sprintf("maal/%s", projectID)
	keys, err := r.store.List(prefix)
	if err != nil {
		return nil, err
	}
	var result []appMaal.MaalRecord
	for _, k := range keys {
		var rec appMaal.MaalRecord
		if err := r.store.Get(k, &rec); err != nil {
			continue
		}
		result = append(result, rec)
	}
	return result, nil
}
