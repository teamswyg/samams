package httpapi

import "proxy/internal/domain"

type createTaskRequest struct {
	Name             string   `json:"name"`
	Prompt           string   `json:"prompt"`
	NumAgents        int      `json:"numAgents"`
	CursorArgs       []string `json:"cursorArgs"`
	Tags             []string `json:"tags"`
	BoundedContextID string   `json:"boundedContextId"`
	ParentTaskID     string   `json:"parentTaskId"`
	AgentType        string   `json:"agentType"`
	Mode             string   `json:"mode"`
}

type scaleTaskRequest struct {
	NumAgents int `json:"numAgents"`
}

type stopTaskRequest struct {
	Graceful    bool   `json:"graceful"`
	Reason      string `json:"reason"`
	CancelledBy string `json:"cancelledBy"`
}

type cancelTaskRequest struct {
	Reason      string `json:"reason"`
	CancelledBy string `json:"cancelledBy"`
}

type updateSummaryRequest struct {
	Summary string `json:"summary"`
}

func (r *createTaskRequest) toAgentType() domain.AgentType {
	switch r.AgentType {
	case "claude":
		return domain.AgentTypeClaude
	case "opencode":
		return domain.AgentTypeOpenCode
	default:
		return domain.AgentTypeCursor
	}
}

func (r *createTaskRequest) toMode() domain.AgentMode {
	switch r.Mode {
	case "plan":
		return domain.AgentModePlan
	case "debug":
		return domain.AgentModeDebug
	default:
		return domain.AgentModeExecute
	}
}

type errorResponse struct {
	Error string `json:"error"`
}
