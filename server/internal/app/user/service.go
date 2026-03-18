package user

import (
	"context"

	"server/internal/domain/shared"
	domainUser "server/internal/domain/user"
)

type Service struct {
	repo  UserRepository
	clock shared.Clock
}

func NewService(repo UserRepository, clock shared.Clock) *Service {
	if clock == nil {
		clock = shared.SystemClock{}
	}
	return &Service{repo: repo, clock: clock}
}

type CreateOrLoginUserViaProviderCommand struct {
	TenantID    shared.TenantID
	Email       string
	DisplayName string
	Plan        domainUser.Plan
	GoogleSub   string
	FirebaseUID string
}

func (s *Service) CreateOrLoginUserViaProvider(ctx context.Context, cmd CreateOrLoginUserViaProviderCommand) (*domainUser.User, error) {
	existing, err := s.repo.GetByEmail(ctx, cmd.Email)
	if err == nil && existing != nil {
		// Simple happy-path login: ensure active.
		_ = existing.Activate(s.clock)
		return existing, s.repo.Save(ctx, existing)
	}

	u, err := domainUser.NewUser(s.clock, domainUser.ID(cmd.Email), cmd.TenantID, cmd.Email, cmd.DisplayName, cmd.Plan)
	if err != nil {
		return nil, err
	}
	u.GoogleSub = cmd.GoogleSub
	u.FirebaseUID = cmd.FirebaseUID
	return u, s.repo.Save(ctx, u)
}

