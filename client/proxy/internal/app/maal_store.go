package app

import (
	"sync"

	"proxy/internal/domain"
)

// MaalStore stores MAAL records in memory.
type MaalStore struct {
	mu      sync.RWMutex
	records []domain.MaalRecord
	max     int
}

func NewMaalStore(maxRecords int) *MaalStore {
	if maxRecords <= 0 {
		maxRecords = 10000
	}
	return &MaalStore{
		records: make([]domain.MaalRecord, 0, maxRecords),
		max:     maxRecords,
	}
}

func (s *MaalStore) Append(r domain.MaalRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) >= s.max {
		s.records = s.records[1:]
	}
	s.records = append(s.records, r)
}

func (s *MaalStore) ByTaskID(taskID string) []domain.MaalRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.MaalRecord
	for _, r := range s.records {
		if r.TaskID == taskID {
			out = append(out, r)
		}
	}
	return out
}

func (s *MaalStore) ByAgentID(agentID string) []domain.MaalRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.MaalRecord
	for _, r := range s.records {
		if r.AgentID == agentID {
			out = append(out, r)
		}
	}
	return out
}

func (s *MaalStore) All() []domain.MaalRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.MaalRecord, len(s.records))
	copy(out, s.records)
	return out
}
