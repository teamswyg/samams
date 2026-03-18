package domain

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type EventType string

const (
	EventUserSignedUp EventType = "user.signed_up"
	EventUserLoggedIn EventType = "user.logged_in"

	EventProjectCreated EventType = "project.created"

	EventTaskCreated          EventType = "task.created"
	EventTaskStatusUpdated    EventType = "task.status_updated"
	EventTaskSummaryUpdated   EventType = "task.summary_updated"
	EventTaskRetryIncremented EventType = "task.retry_incremented"
	EventTaskContextPlanned   EventType = "task.context_planned"
	EventTaskCompleted        EventType = "task.completed"
	EventTaskCancelled        EventType = "task.cancelled"
	EventTaskHardStopped      EventType = "task.hard_stopped"
	EventTaskRedistributed    EventType = "task.redistributed"

	EventStrategyMeetingRequested   EventType = "strategy.meetingRequested"
	EventStrategyMeetingStarted     EventType = "strategy.meetingStarted"
	EventStrategyMeetingResolved    EventType = "strategy.meetingResolved"
	EventStrategyDecisionApplied    EventType = "strategy.decisionApplied"
	EventStrategyAllPaused EventType = "strategy.allPaused"

	EventControlContextReset        EventType = "control.context_reset"
	EventControlContextLoaded       EventType = "control.context_loaded"
	EventControlContextLostDetected EventType = "contextLost" // Server SSOT naming

	// Usage events — currently NOT emitted because Cursor CLI does not expose token data.
	// Will be connected when token tracking becomes available.
	EventUsageReported         EventType = "usage.reported"
	EventUsageLimitApproaching EventType = "usage.limit_approaching"
	EventUsageLimitExceeded    EventType = "usage.limit_exceeded"

	EventAgentCreated      EventType = "agent.created"
	EventAgentAssigned     EventType = "agent.assigned"
	EventAgentStateChanged EventType = "agent.stateChanged" // Server SSOT naming
	EventAgentPaused       EventType = "agent.paused"
	EventAgentResumed      EventType = "agent.resumed"
	EventAgentStopped      EventType = "agent.stopped"
	EventAgentError        EventType = "agent.error"
	EventAgentLogAppended  EventType = "agent.logAppended" // Server SSOT naming

	EventNotificationCreated EventType = "notification.created"
	EventMaalRecordCreated   EventType = "maal.record" // Server SSOT naming

	EventMilestoneReviewCompleted EventType = "milestone.review.completed"
	EventMilestoneReviewFailed    EventType = "milestone.review.failed"
	EventMilestoneMerged          EventType = "milestone.merged"
)

// Envelope is the common event envelope (compatible with shared/domain/event).
type Envelope struct {
	ID        string            `json:"id"`
	Type      EventType         `json:"type"`
	Source    string            `json:"source"`
	Timestamp int64             `json:"timestamp"`
	UserID    string            `json:"user_id,omitempty"`
	ProjectID string            `json:"project_id,omitempty"`
	TaskID    string            `json:"task_id,omitempty"`
	Payload   map[string]any    `json:"payload,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func NewEnvelope(t EventType, source string) Envelope {
	return Envelope{
		ID:        newEventID(),
		Type:      t,
		Source:    source,
		Timestamp: time.Now().UnixMilli(),
		Payload:   make(map[string]any),
		Metadata:  make(map[string]string),
	}
}

func newEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
