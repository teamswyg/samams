package domain

import "time"

// MaalRecord is a single MAAL (Multiple AI Agent Logs) entry.
type MaalRecord struct {
	ID        string            `json:"id"`
	AgentID   string            `json:"agentId"`
	TaskID    string            `json:"taskId"`
	Timestamp time.Time         `json:"timestamp"`
	Action    string            `json:"action"`
	Content   string            `json:"content,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// LogEntry is a formatted log line for the frontend MAAL Log Stream.
type LogEntry struct {
	ID      string `json:"id"`
	Time    string `json:"time"`
	Type    string `json:"type"`    // INFO, WARN, ERROR
	Agent   string `json:"agent"`
	Message string `json:"message"`
}
