package app

import (
	"sync"

	"proxy/internal/domain"
)

// NotificationStore holds notifications in memory.
type NotificationStore struct {
	mu    sync.RWMutex
	items []domain.Notification
	max   int
}

func NewNotificationStore(maxItems int) *NotificationStore {
	if maxItems <= 0 {
		maxItems = 1000
	}
	return &NotificationStore{
		items: make([]domain.Notification, 0, maxItems),
		max:   maxItems,
	}
}

func (s *NotificationStore) Add(n domain.Notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) >= s.max {
		s.items = s.items[1:]
	}
	s.items = append(s.items, n)
}

func (s *NotificationStore) List() []domain.Notification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Notification, len(s.items))
	copy(out, s.items)
	return out
}

func (s *NotificationStore) ByTaskID(taskID string) []domain.Notification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.Notification
	for _, n := range s.items {
		if n.TaskID == taskID {
			out = append(out, n)
		}
	}
	return out
}
