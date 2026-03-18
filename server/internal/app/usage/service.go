package usage

import (
	"context"

	"server/internal/domain/usage"
)

type ReportCommand struct {
	UserID          string
	ProjectID       string
	TokensUsedDelta int64
}

type SnapshotRepository interface {
	Get(ctx context.Context, userID, projectID string) (usage.Snapshot, error)
	Save(ctx context.Context, s usage.Snapshot) error
}

type EventBus interface {
	PublishUsageLimitApproaching(ctx context.Context, s usage.Snapshot, p usage.Prediction) error
	PublishUsageLimitExceeded(ctx context.Context, s usage.Snapshot, p usage.Prediction) error
}

type Service struct {
	repo SnapshotRepository
	bus  EventBus
}

func NewService(repo SnapshotRepository, bus EventBus) *Service {
	return &Service{
		repo: repo,
		bus:  bus,
	}
}

func (s *Service) Report(ctx context.Context, cmd ReportCommand) error {
	snap, err := s.repo.Get(ctx, cmd.UserID, cmd.ProjectID)
	if err != nil {
		return err
	}
	snap.UserID = cmd.UserID
	snap.ProjectID = cmd.ProjectID
	snap.TotalTokensUsed += cmd.TokensUsedDelta

	pred := usage.Predict(snap)

	if err := s.repo.Save(ctx, snap); err != nil {
		return err
	}

	if !pred.CanContinue {
		if s.bus != nil {
			_ = s.bus.PublishUsageLimitExceeded(ctx, snap, pred)
		}
	} else if pred.UsageRatio >= 0.8 {
		if s.bus != nil {
			_ = s.bus.PublishUsageLimitApproaching(ctx, snap, pred)
		}
	}

	return nil
}

