package maal

import (
	"context"
	"time"
)

// MaalRecord represents a single MAAL log entry.
type MaalRecord struct {
	ID        string
	ProjectID string
	AgentID   string
	Content   string
	Severity  string
	CreatedAt time.Time
}

// MaalRecordRepository persists MAAL records.
type MaalRecordRepository interface {
	Save(ctx context.Context, record *MaalRecord) error
	FindByProject(ctx context.Context, projectID string) ([]MaalRecord, error)
}
