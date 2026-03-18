package control

import (
	"context"
	"fmt"

	domainControl "server/internal/domain/control"
	"server/internal/domain/shared"
)

type Service struct {
	repo  ControlStateRepository
	bus   ControlEventBus
	clock shared.Clock
}

func NewService(repo ControlStateRepository, bus ControlEventBus, clock shared.Clock) *Service {
	if clock == nil {
		clock = shared.SystemClock{}
	}
	return &Service{repo: repo, bus: bus, clock: clock}
}

// ContextReset captures current state and emits a reset event.
func (s *Service) ContextReset(ctx context.Context, cmd ResetCommand) (string, error) {
	cs := domainControl.NewControlState(cmd.ProjectID)
	snap := cs.PlanReset(s.clock, cmd.Reason, cmd.ContextPayload)

	if err := s.repo.Save(ctx, cs); err != nil {
		return "", fmt.Errorf("save state failed: %w", err)
	}

	if s.bus != nil {
		if err := s.bus.PublishAll(ctx, cs.PullDomainEvents()); err != nil {
			return "", fmt.Errorf("publish events failed: %w", err)
		}
	}

	return snap.StateID, nil
}

// ContextLoaded resumes from a previously captured state.
func (s *Service) ContextLoaded(ctx context.Context, cmd ResumeCommand) error {
	cs, err := s.repo.Load(ctx, cmd.StateID)
	if err != nil {
		return fmt.Errorf("state not found: %w", err)
	}

	snap := cs.FindSnapshot(cmd.StateID)
	if snap == nil {
		return fmt.Errorf("snapshot %s not found", cmd.StateID)
	}

	cs.PlanResume(s.clock, cmd.StateID)

	if s.bus != nil {
		if err := s.bus.PublishAll(ctx, cs.PullDomainEvents()); err != nil {
			return fmt.Errorf("publish events failed: %w", err)
		}
	}

	return nil
}

// ContextLostDetected captures current state for recovery.
func (s *Service) ContextLostDetected(ctx context.Context, cmd LoadContextCommand) error {
	cs, err := s.repo.FindByProject(ctx, cmd.ProjectID)
	if err != nil {
		cs = domainControl.NewControlState(cmd.ProjectID)
	}

	cs.Capture(s.clock, "context_lost_detected", "automatic recovery")

	if err := s.repo.Save(ctx, cs); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	if s.bus != nil {
		if err := s.bus.PublishAll(ctx, cs.PullDomainEvents()); err != nil {
			return fmt.Errorf("publish events failed: %w", err)
		}
	}

	return nil
}
