package event

type Type string

type Envelope struct {
	ID        string                 `json:"id"`
	Type      Type                   `json:"type"`
	Source    string                 `json:"source"`
	Timestamp int64                  `json:"timestamp"`
	UserID    string                 `json:"user_id,omitempty"`
	ProjectID string                 `json:"project_id,omitempty"`
	TaskID    string                 `json:"task_id,omitempty"`
	Payload   map[string]any         `json:"payload,omitempty"`
	Metadata  map[string]string      `json:"metadata,omitempty"`
}

