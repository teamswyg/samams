package inmemory

import (
	"context"
	"sync"

	"server/internal/domain/shared"
	domainStrategy "server/internal/domain/strategy"
	appStrategy "server/internal/app/strategy"
)

var _ appStrategy.StrategyMeetingRepository = (*StrategyMeetingRepository)(nil)

// StrategyMeetingRepository is an in-memory implementation of StrategyMeetingRepository.
type StrategyMeetingRepository struct {
	mu    sync.RWMutex
	store map[domainStrategy.MeetingID]*domainStrategy.StrategyMeeting
}

func NewStrategyMeetingRepository() *StrategyMeetingRepository {
	return &StrategyMeetingRepository{
		store: make(map[domainStrategy.MeetingID]*domainStrategy.StrategyMeeting),
	}
}

func (r *StrategyMeetingRepository) Save(_ context.Context, m *domainStrategy.StrategyMeeting) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[m.ID] = m
	return nil
}

func (r *StrategyMeetingRepository) FindByID(_ context.Context, id domainStrategy.MeetingID) (*domainStrategy.StrategyMeeting, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.store[id]
	if !ok {
		return nil, shared.NotFoundError{Resource: "strategy_meeting", ID: string(id)}
	}
	return m, nil
}
