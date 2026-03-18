package proxy

// HeartbeatCommand is the input for the Heartbeat usecase.
type HeartbeatCommand struct {
	UserID        string            `json:"userID"`
	AgentStatuses map[string]string `json:"agentStatuses"`
	Uptime        int64             `json:"uptime"`
	TaskCount     int               `json:"taskCount"`
}

// HeartbeatData is the stored heartbeat snapshot.
type HeartbeatData struct {
	AgentStatuses map[string]string `json:"agentStatuses"`
	Uptime        int64             `json:"uptime"`
	TaskCount     int               `json:"taskCount"`
}

// PollCommand is the input for the CommandsPoll usecase.
type PollCommand struct {
	UserID string `json:"userID"`
}

// PendingCommand is a command waiting for proxy pickup.
type PendingCommand struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Payload   any    `json:"payload"`
	CreatedAt string `json:"createdAt"`
}

// EventsPushCommand is the input for the EventsPush usecase.
type EventsPushCommand struct {
	UserID string       `json:"userID"`
	Events []ProxyEvent `json:"events"`
}

// ProxyEvent is a single event from the proxy.
type ProxyEvent struct {
	Type    string `json:"type"`
	TaskID  string `json:"taskID"`
	Payload any    `json:"payload"`
	TS      int64  `json:"ts"`
}

// CommandResponseCmd is the input for the CommandResponse usecase.
type CommandResponseCmd struct {
	UserID    string `json:"userID"`
	CommandID string `json:"commandID"`
	Payload   any    `json:"payload"`
	Error     string `json:"error,omitempty"`
}

// CommandResult is the stored response for a completed command.
type CommandResult struct {
	Payload any    `json:"payload"`
	Error   string `json:"error,omitempty"`
}

// TaskCompletedCommand is the input for the TaskCompleted usecase.
// Triggered when proxy sends task.completed event — invokes Summarizer + stores result.
type TaskCompletedCommand struct {
	UserID   string `json:"userID"`
	TaskID   string `json:"taskID"`
	AgentID  string `json:"agentID"`
	Context  string `json:"context"`  // agent execution context (logs/output)
	Frontier string `json:"frontier"` // frontier from agent
}

// TaskSummaryResult is the stored summary for a completed task.
type TaskSummaryResult struct {
	TaskID    string `json:"taskID"`
	AgentID   string `json:"agentID"`
	Summary   string `json:"summary"`
	Frontier  string `json:"frontier,omitempty"`
	CreatedAt string `json:"createdAt"`
}
