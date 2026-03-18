package domain

import "time"

// Notification is a user-facing notification (e.g. task cancelled reason).
type Notification struct {
	ID        string     `json:"id"`
	UserID    string     `json:"userId,omitempty"`
	TaskID    string     `json:"taskId,omitempty"`
	AgentID   string     `json:"agentId,omitempty"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Severity  string     `json:"severity"`
	CreatedAt time.Time  `json:"createdAt"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
}
