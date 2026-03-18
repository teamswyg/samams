package project

import (
	"context"

	"server/internal/domain/project"
	"server/internal/domain/shared"
)

type Service struct {
	repo  ProjectRepository
	clock shared.Clock
}

func NewService(repo ProjectRepository, clock shared.Clock) *Service {
	if clock == nil {
		clock = shared.SystemClock{}
	}
	return &Service{repo: repo, clock: clock}
}

type CreateProjectCommand struct {
	ID          project.ID
	TenantID    shared.TenantID
	Name        string
	Description string
	OwnerID     shared.UserID
}

func (s *Service) CreateProject(ctx context.Context, cmd CreateProjectCommand) (*project.Project, error) {
	p, err := project.NewProject(s.clock, cmd.ID, cmd.TenantID, cmd.Name, cmd.OwnerID, cmd.Description)
	if err != nil {
		return nil, err
	}
	return p, s.repo.Save(ctx, p)
}

type ArchiveProjectCommand struct {
	ID project.ID
}

func (s *Service) ArchiveProject(ctx context.Context, cmd ArchiveProjectCommand) (*project.Project, error) {
	p, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := p.Archive(s.clock); err != nil {
		return nil, err
	}
	return p, s.repo.Save(ctx, p)
}

