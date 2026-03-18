package maal

// CreateRecordCommand triggers creation of a MAAL log record.
type CreateRecordCommand struct {
	ProjectID string
	AgentID   string
	Content   string
	Severity  string
}
