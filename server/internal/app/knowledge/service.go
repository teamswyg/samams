package knowledge

import (
	"context"

	domainKnowledge "server/internal/domain/knowledge"
	"server/internal/domain/shared"
)

type Service struct {
	repo  KnowledgeRepository
	clock shared.Clock
}

func NewService(repo KnowledgeRepository, clock shared.Clock) *Service {
	if clock == nil {
		clock = shared.SystemClock{}
	}
	return &Service{repo: repo, clock: clock}
}

func (s *Service) Create(ctx context.Context, cmd CreateEntryCommand) (*domainKnowledge.Entry, error) {
	entry, err := domainKnowledge.NewEntry(
		s.clock,
		domainKnowledge.ID(shared.GenerateID()),
		cmd.TenantID,
		cmd.ProjectID,
		cmd.Title,
		cmd.Content,
		cmd.Category,
		cmd.Tags,
		cmd.SourceAgentID,
		cmd.SourceTaskID,
	)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *Service) Update(ctx context.Context, cmd UpdateEntryCommand) error {
	entry, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if err := entry.UpdateContent(s.clock, cmd.Title, cmd.Content); err != nil {
		return err
	}
	return s.repo.Save(ctx, entry)
}

func (s *Service) SetConfidence(ctx context.Context, cmd SetConfidenceCommand) error {
	entry, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if err := entry.SetConfidence(s.clock, cmd.Confidence); err != nil {
		return err
	}
	return s.repo.Save(ctx, entry)
}

func (s *Service) RecordUsage(ctx context.Context, id domainKnowledge.ID) error {
	entry, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	entry.RecordUsage(s.clock)
	return s.repo.Save(ctx, entry)
}

func (s *Service) Deactivate(ctx context.Context, id domainKnowledge.ID) error {
	entry, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := entry.Deactivate(s.clock); err != nil {
		return err
	}
	return s.repo.Save(ctx, entry)
}

func (s *Service) GetByID(ctx context.Context, id domainKnowledge.ID) (*domainKnowledge.Entry, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListByProject(ctx context.Context, projectID shared.ProjectID) ([]*domainKnowledge.Entry, error) {
	return s.repo.FindByProject(ctx, projectID)
}

func (s *Service) Search(ctx context.Context, cmd SearchCommand) ([]*domainKnowledge.Entry, error) {
	if cmd.Category != domainKnowledge.CategoryUnknown {
		return s.repo.FindByCategory(ctx, cmd.ProjectID, cmd.Category)
	}
	if len(cmd.Tags) > 0 {
		return s.repo.SearchByTags(ctx, cmd.ProjectID, cmd.Tags)
	}
	if cmd.Query != "" {
		return s.repo.SearchByContent(ctx, cmd.ProjectID, cmd.Query)
	}
	return s.repo.FindByProject(ctx, cmd.ProjectID)
}
