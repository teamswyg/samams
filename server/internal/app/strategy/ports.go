package strategy

import (
	"context"

	domainStrategy "server/internal/domain/strategy"
	"server/internal/domain/shared"
)

// StrategyMeetingRepository persists strategy meetings.
type StrategyMeetingRepository interface {
	Save(ctx context.Context, m *domainStrategy.StrategyMeeting) error
	FindByID(ctx context.Context, id domainStrategy.MeetingID) (*domainStrategy.StrategyMeeting, error)
}

// FailureTrackerRepository persists repeated failure trackers.
type FailureTrackerRepository interface {
	FindBySubject(ctx context.Context, subjectID string) (*domainStrategy.RepeatedFailureTracker, error)
	Save(ctx context.Context, t *domainStrategy.RepeatedFailureTracker) error
}

// StrategyEventBus publishes strategy domain events.
type StrategyEventBus interface {
	PublishAll(ctx context.Context, events []shared.DomainEvent) error
}
