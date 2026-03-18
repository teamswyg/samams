package maal

import (
	"context"

	"server/internal/domain/shared"
)

type Service struct {
	repo  MaalRecordRepository
	clock shared.Clock
}

func NewService(repo MaalRecordRepository, clock shared.Clock) *Service {
	if clock == nil {
		clock = shared.SystemClock{}
	}
	return &Service{repo: repo, clock: clock}
}

// RecordCreated persists a new MAAL log record.
func (s *Service) RecordCreated(ctx context.Context, cmd CreateRecordCommand) error {
	record := &MaalRecord{
		ID:        shared.GenerateID(),
		ProjectID: cmd.ProjectID,
		AgentID:   cmd.AgentID,
		Content:   cmd.Content,
		Severity:  cmd.Severity,
		CreatedAt: s.clock.Now(),
	}
	return s.repo.Save(ctx, record)
}
