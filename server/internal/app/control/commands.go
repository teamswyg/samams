package control

// ResetCommand captures state and triggers a control reset.
type ResetCommand struct {
	ProjectID      string
	Reason         string
	ContextPayload string
}

// ResumeCommand resumes from a previously captured state.
type ResumeCommand struct {
	StateID string
}

// LoadContextCommand loads context for a project.
type LoadContextCommand struct {
	ProjectID string
}
