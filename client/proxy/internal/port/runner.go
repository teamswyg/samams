package port

import "context"

// LogFunc is called for each log line produced by an agent.
type LogFunc func(line string)

// StartOptions configures how the CLI agent should be started.
type StartOptions struct {
	Prompt     string
	CursorArgs []string
	WorkDir    string // override working directory (e.g., worktree path)
}

// Handle represents a running agent process.
type Handle struct {
	wait func() error
}

func (h *Handle) Wait() error {
	if h == nil || h.wait == nil {
		return nil
	}
	return h.wait()
}

func NewHandle(waitFn func() error) *Handle {
	return &Handle{wait: waitFn}
}

// NewHandleForTest creates a handle that returns immediately (for tests).
func NewHandleForTest() *Handle {
	return &Handle{wait: func() error { return nil }}
}

// Runner is a secondary (driven) port for agent process management.
type Runner interface {
	StartAgent(ctx context.Context, agentID string, opts StartOptions, logf LogFunc) (*Handle, error)
	StopAgent(ctx context.Context, agentID string) error
	// InterruptAgent sends a single SIGINT without killing the process.
	// The agent stays alive in input-waiting mode.
	InterruptAgent(ctx context.Context, agentID string) error
	SendInput(agentID, input string) error
}
