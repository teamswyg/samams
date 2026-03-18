package strategy

import (
	"context"
	"fmt"

	"server/internal/domain/shared"
	domainStrategy "server/internal/domain/strategy"
)

type Service struct {
	meetingRepo StrategyMeetingRepository
	failureRepo FailureTrackerRepository
	bus         StrategyEventBus
	clock       shared.Clock
}

func NewService(meetingRepo StrategyMeetingRepository, failureRepo FailureTrackerRepository, bus StrategyEventBus, clock shared.Clock) *Service {
	if clock == nil {
		clock = shared.SystemClock{}
	}
	return &Service{
		meetingRepo: meetingRepo,
		failureRepo: failureRepo,
		bus:         bus,
		clock:       clock,
	}
}

// MeetingRequested creates a new strategy meeting.
func (s *Service) MeetingRequested(ctx context.Context, cmd RequestMeetingCommand) error {
	meeting, err := domainStrategy.NewStrategyMeeting(s.clock, cmd.MeetingID, cmd.ProjectID, cmd.UserID, cmd.Topic, cmd.Description)
	if err != nil {
		return err
	}

	if err := s.meetingRepo.Save(ctx, meeting); err != nil {
		return fmt.Errorf("save meeting failed: %w", err)
	}

	return s.publishEvents(ctx, meeting.PullDomainEvents())
}

// MeetingStarted transitions a meeting to active status.
func (s *Service) MeetingStarted(ctx context.Context, cmd StartMeetingCommand) error {
	meeting, err := s.meetingRepo.FindByID(ctx, cmd.MeetingID)
	if err != nil {
		return fmt.Errorf("meeting not found: %w", err)
	}

	if err := meeting.Start(s.clock); err != nil {
		return err
	}

	if err := s.meetingRepo.Save(ctx, meeting); err != nil {
		return fmt.Errorf("save meeting failed: %w", err)
	}

	return s.publishEvents(ctx, meeting.PullDomainEvents())
}

// MeetingResolved resolves a meeting with a decision.
func (s *Service) MeetingResolved(ctx context.Context, cmd ResolveMeetingCommand) error {
	meeting, err := s.meetingRepo.FindByID(ctx, cmd.MeetingID)
	if err != nil {
		return fmt.Errorf("meeting not found: %w", err)
	}

	if err := meeting.Resolve(s.clock, cmd.Decision); err != nil {
		return err
	}

	if err := s.meetingRepo.Save(ctx, meeting); err != nil {
		return fmt.Errorf("save meeting failed: %w", err)
	}

	return s.publishEvents(ctx, meeting.PullDomainEvents())
}

// DecisionApplied handles the application of a meeting decision.
func (s *Service) DecisionApplied(ctx context.Context, cmd ResolveMeetingCommand) error {
	_ = ctx
	_ = cmd
	return nil
}

// RecordTaskDone records a task done event for repeated failure tracking.
func (s *Service) RecordTaskDone(ctx context.Context, cmd RecordTaskDoneCommand) error {
	tracker, err := s.failureRepo.FindBySubject(ctx, cmd.SubjectID)
	if err != nil {
		tracker = domainStrategy.NewRepeatedFailureTracker(cmd.SubjectID, cmd.Threshold)
	}

	tracker.RecordDone(s.clock)

	if err := s.failureRepo.Save(ctx, tracker); err != nil {
		return fmt.Errorf("save tracker failed: %w", err)
	}

	return s.publishEvents(ctx, tracker.PullDomainEvents())
}

// ReportTokenUsage checks token usage for anomaly detection.
func (s *Service) ReportTokenUsage(ctx context.Context, cmd ReportTokenUsageCommand) error {
	tu := domainStrategy.NewTokenUsage(cmd.UsedTokens, cmd.AvailableTokens, cmd.AnomalyThresholdRatio)

	if tu.IsAnomaly() {
		ev := shared.NewDomainEvent(
			s.clock, "TokenUsageAnomalyDetected", "strategy", cmd.SubjectID,
			"token_usage", cmd.SubjectID, "warning",
			map[string]any{
				"used_ratio": tu.UsedRatio,
				"threshold":  tu.AnomalyThreshold,
			},
		)
		return s.publishEvents(ctx, []shared.DomainEvent{ev})
	}
	return nil
}

func (s *Service) publishEvents(ctx context.Context, events []shared.DomainEvent) error {
	if s.bus == nil || len(events) == 0 {
		return nil
	}
	return s.bus.PublishAll(ctx, events)
}
