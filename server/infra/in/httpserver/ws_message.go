package httpserver

import "encoding/json"

// WSMessage is the envelope for all WebSocket communication between server and proxy.
type WSMessage struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`    // "command" | "response" | "event"
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Error     string          `json:"error,omitempty"`
	Timestamp int64           `json:"ts"`
}

// Message types.
const (
	WSTypeCommand  = "command"
	WSTypeResponse = "response"
	WSTypeEvent    = "event"
)

// Server → Proxy commands.
const (
	ActionCreateTask  = "createTask"
	ActionStopTask    = "stopTask"
	ActionPauseTask   = "pauseTask"
	ActionResumeTask  = "resumeTask"
	ActionCancelTask  = "cancelTask"
	ActionScaleTask   = "scaleTask"
	ActionResetTask   = "resetTask"
	ActionListTasks   = "listTasks"
	ActionListAgents  = "listAgents"
	ActionGetTask     = "getTask"
	ActionStopAgent   = "stopAgent"
	ActionSendInput   = "sendInput"
	ActionHealthCheck           = "healthCheck"
	ActionGetLogs               = "getLogs"
	ActionCreateSkeleton        = "createSkeleton"
	ActionGetAgentLogs          = "getAgentLogs"
	ActionCreateReviewTask      = "createReviewTask"
	ActionMergeMilestone        = "mergeMilestone"
	ActionStrategyPauseAll       = "strategyPauseAll"
	ActionStrategyApplyDecision  = "strategyApplyDecision"
)

// Proxy → Server events.
const (
	EventAgentStateChanged = "agent.stateChanged"
	EventAgentLogAppended  = "agent.logAppended"
	EventTaskCompleted     = "task.completed"
	EventTaskFailed        = "task.failed"
	EventHeartbeat         = "heartbeat"
	EventContextLost       = "contextLost"
	EventMemoryIssue       = "memoryIssue"
	EventMaalRecord               = "maal.record"
	EventMilestoneReviewCompleted = "milestone.review.completed"
	EventMilestoneReviewFailed    = "milestone.review.failed"
	EventMilestoneMerged          = "milestone.merged"
	EventStrategyAllPaused        = "strategy.allPaused"
	EventStrategyDecisionApplied = "strategy.decisionApplied"
)

// MeetingMeta is the persisted state for a strategy meeting session.
// Shared between run_handler (REST endpoints) and event_processor (orchestration).
type MeetingMeta struct {
	SessionID           string            `json:"sessionId"`
	ProjectName         string            `json:"projectName"`
	Status              string            `json:"status"` // idle, pausing, analyzing, dispatching
	Trigger             string            `json:"trigger"`
	Round               int               `json:"round"`
	MaxRounds           int               `json:"maxRounds"`
	ParticipantNodeUIDs []string          `json:"participantNodeUids"`
	MilestoneUID        string            `json:"milestoneUid,omitempty"`
	CreatedAt           int64             `json:"createdAt"`
	DiscussionContexts  map[string]string `json:"discussionContexts,omitempty"` // nodeUID → context from proxy watch agents
	Decision            string            `json:"decision,omitempty"`
	DecisionReasoning   string            `json:"decisionReasoning,omitempty"`
}
