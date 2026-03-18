package proxy

import "context"

// Repository abstracts S3 access for proxy state and commands.
type Repository interface {
	// SaveHeartbeat stores proxy heartbeat data.
	SaveHeartbeat(ctx context.Context, userID string, data HeartbeatData) error
	// UpdateLastSeen records the last communication timestamp.
	UpdateLastSeen(ctx context.Context, userID string) error
	// ListPendingCommands returns commands awaiting proxy pickup.
	ListPendingCommands(ctx context.Context, userID string) ([]PendingCommand, error)
	// CompleteCommand moves a command from pending to completed with the response.
	CompleteCommand(ctx context.Context, userID, commandID string, response CommandResult) error
}

// EventBus is required by the codegen template but proxy events
// are forwarded to the existing SQS pipeline instead.
type EventBus interface{}

// Service handles proxy-server communication via S3.
type Service struct {
	repo Repository
	bus  EventBus
}

// NewService creates a proxy service. Matches codegen template signature.
func NewService(repo Repository, bus EventBus) *Service {
	return &Service{repo: repo, bus: bus}
}

// Heartbeat processes a heartbeat from the proxy client.
func (s *Service) Heartbeat(ctx context.Context, cmd HeartbeatCommand) error {
	userID := cmd.UserID
	if err := s.repo.SaveHeartbeat(ctx, userID, HeartbeatData{
		AgentStatuses: cmd.AgentStatuses,
		Uptime:        cmd.Uptime,
		TaskCount:     cmd.TaskCount,
	}); err != nil {
		return err
	}
	return s.repo.UpdateLastSeen(ctx, userID)
}

// CommandsPoll returns pending commands for the proxy.
func (s *Service) CommandsPoll(ctx context.Context, cmd PollCommand) error {
	// In the actual Lambda handler, the result is returned as HTTP response.
	// This stub satisfies the codegen template; real logic is in the handler.
	return nil
}

// EventsPush forwards proxy events to the persistence pipeline.
func (s *Service) EventsPush(ctx context.Context, cmd EventsPushCommand) error {
	// Real logic sends each event to SQS. Implemented in the handler.
	return nil
}

// CommandResponse marks a command as completed with the proxy's response.
func (s *Service) CommandResponse(ctx context.Context, cmd CommandResponseCmd) error {
	return s.repo.CompleteCommand(ctx, cmd.UserID, cmd.CommandID, CommandResult{
		Payload: cmd.Payload,
		Error:   cmd.Error,
	})
}
