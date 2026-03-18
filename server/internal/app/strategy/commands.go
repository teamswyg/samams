package strategy

import domainStrategy "server/internal/domain/strategy"

// RequestMeetingCommand creates a new strategy meeting.
type RequestMeetingCommand struct {
	MeetingID   domainStrategy.MeetingID
	ProjectID   string
	UserID      string
	Topic       string
	Description string
}

// StartMeetingCommand transitions a meeting to active.
type StartMeetingCommand struct {
	MeetingID domainStrategy.MeetingID
}

// ResolveMeetingCommand resolves a meeting with a decision.
type ResolveMeetingCommand struct {
	MeetingID domainStrategy.MeetingID
	Decision  string
}

// RecordTaskDoneCommand records a task completion for failure tracking.
type RecordTaskDoneCommand struct {
	SubjectID string
	Threshold int
}

// ReportTokenUsageCommand reports token usage for anomaly detection.
type ReportTokenUsageCommand struct {
	SubjectID             string
	UsedTokens            int
	AvailableTokens       int
	AnomalyThresholdRatio float64
}
