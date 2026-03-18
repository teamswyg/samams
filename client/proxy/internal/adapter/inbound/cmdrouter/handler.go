package cmdrouter

import (
	"context"
	"encoding/json"
	"fmt"

	"proxy/internal/domain"
	"proxy/internal/port"
)

// New returns a CommandHandler that routes commands to the TaskService.
func New(svc port.TaskService) port.CommandHandler {
	return func(action string, payload json.RawMessage) (json.RawMessage, error) {
		ctx := context.Background()

		switch action {
		case "createTask":
			return handleCreateTask(ctx, svc, payload)
		case "listTasks":
			return marshalResult(svc.ListTasks())
		case "listAgents":
			return marshalResult(svc.ListAgents())
		case "getTask":
			return handleGetTask(svc, payload)
		case "stopTask":
			return handleStopTask(ctx, svc, payload)
		case "pauseTask":
			return handlePauseTask(ctx, svc, payload)
		case "resumeTask":
			return handleResumeTask(ctx, svc, payload)
		case "cancelTask":
			return handleCancelTask(ctx, svc, payload)
		case "scaleTask":
			return handleScaleTask(ctx, svc, payload)
		case "resetTask":
			return handleResetTask(ctx, svc, payload)
		case "stopAgent":
			return handleStopAgent(ctx, svc, payload)
		case "sendInput":
			return handleSendInput(svc, payload)
		case "getLogs":
			return marshalResult(svc.GetRecentLogs())
		case "createSkeleton":
			return handleCreateSkeleton(svc, payload)
		case "getAgentLogs":
			return handleGetAgentLogs(svc, payload)
		case "createReviewTask":
			return handleCreateTask(ctx, svc, payload) // same flow, NodeType="review" distinguishes
		case "mergeMilestone":
			return handleMergeMilestone(ctx, svc, payload)
		case "strategyPauseAll":
			return handleStrategyPauseAll(ctx, svc, payload)
		case "strategyApplyDecision":
			return handleStrategyApplyDecision(ctx, svc, payload)
		case "healthCheck":
			return json.Marshal(map[string]string{"status": "ok"})
		default:
			return nil, fmt.Errorf("unknown action: %s", action)
		}
	}
}

func handleCreateTask(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		Name             string   `json:"name"`
		Prompt           string   `json:"prompt"`
		NumAgents        int      `json:"numAgents"`
		CursorArgs       []string `json:"cursorArgs,omitempty"`
		Tags             []string `json:"tags,omitempty"`
		BoundedContextID string   `json:"boundedContextId,omitempty"`
		ParentTaskID     string   `json:"parentTaskId,omitempty"`
		AgentType        string   `json:"agentType,omitempty"`
		Mode             string   `json:"mode,omitempty"`
		NodeType         string   `json:"nodeType,omitempty"`
		NodeUID          string   `json:"nodeUid,omitempty"`
		ParentBranch     string   `json:"parentBranch,omitempty"`
		ProjectName      string   `json:"projectName,omitempty"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	result, err := svc.CreateTask(ctx, domain.CreateTaskParams{
		Name:             req.Name,
		Prompt:           req.Prompt,
		NumAgents:        req.NumAgents,
		CursorArgs:       req.CursorArgs,
		Tags:             req.Tags,
		BoundedContextID: req.BoundedContextID,
		ParentTaskID:     req.ParentTaskID,
		AgentType:        domain.AgentType(req.AgentType),
		Mode:             domain.AgentMode(req.Mode),
		NodeType:         req.NodeType,
		NodeUID:          req.NodeUID,
		ParentBranch:     req.ParentBranch,
		ProjectName:      req.ProjectName,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func handleGetTask(svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TaskID string `json:"taskID"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	result, err := svc.GetTask(req.TaskID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func handleStopTask(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TaskID   string `json:"taskID"`
		HardStop bool   `json:"hardStop"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	if err := svc.StopTask(ctx, req.TaskID, &domain.StopTaskOptions{Graceful: !req.HardStop}); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func handlePauseTask(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TaskID string `json:"taskID"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	// Wildcard: pause all running tasks.
	if req.TaskID == "*" {
		tasks := svc.ListTasks()
		paused := 0
		for _, t := range tasks {
			if t.Status == domain.TaskStatusRunning || t.Status == domain.TaskStatusScaling {
				if err := svc.PauseTask(ctx, t.ID); err == nil {
					paused++
				}
			}
		}
		return json.Marshal(map[string]any{"status": "ok", "paused": paused})
	}
	if err := svc.PauseTask(ctx, req.TaskID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func handleResumeTask(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TaskID string `json:"taskID"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	// Wildcard: resume all paused tasks.
	if req.TaskID == "*" {
		tasks := svc.ListTasks()
		resumed := 0
		for _, t := range tasks {
			if t.Status == domain.TaskStatusPaused {
				if _, err := svc.ResumeTask(ctx, t.ID); err == nil {
					resumed++
				}
			}
		}
		return json.Marshal(map[string]any{"status": "ok", "resumed": resumed})
	}
	if _, err := svc.ResumeTask(ctx, req.TaskID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func handleCancelTask(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TaskID      string `json:"taskID"`
		Reason      string `json:"reason"`
		CancelledBy string `json:"cancelledBy"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	if err := svc.CancelTask(ctx, req.TaskID, req.Reason, req.CancelledBy); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func handleScaleTask(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TaskID    string `json:"taskID"`
		NumAgents int    `json:"numAgents"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	if _, err := svc.ScaleTask(ctx, req.TaskID, domain.ScaleTaskParams{NumAgents: req.NumAgents}); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func handleResetTask(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TaskID string `json:"taskID"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	if _, err := svc.ResetTask(ctx, req.TaskID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func handleStopAgent(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		AgentID string `json:"agentID"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	if err := svc.StopAgent(ctx, req.AgentID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func handleSendInput(svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		AgentID string `json:"agentID"`
		Input   string `json:"input"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	if err := svc.AppendAgentInput(req.AgentID, req.Input); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func handleCreateSkeleton(svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var spec domain.SkeletonSpec
	if err := json.Unmarshal(payload, &spec); err != nil {
		return nil, fmt.Errorf("parse skeleton spec: %w", err)
	}
	mainPath, err := svc.CreateSkeleton(spec)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"mainPath": mainPath, "status": "created"})
}

func handleGetAgentLogs(svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		AgentID string `json:"agentID"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	logs, err := svc.GetAgentLogs(req.AgentID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"logs": logs})
}

func handleMergeMilestone(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		BranchName   string `json:"branchName"`
		TargetBranch string `json:"targetBranch"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	if err := svc.MergeMilestone(ctx, req.BranchName, req.TargetBranch); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "merged"})
}

func handleStrategyPauseAll(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var req struct {
		ParticipantNodeUIDs []string `json:"participantNodeUids"`
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &req)
	}
	if err := svc.StrategyPauseAll(ctx, req.ParticipantNodeUIDs); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "allPaused"})
}

func handleStrategyApplyDecision(ctx context.Context, svc port.TaskService, payload json.RawMessage) (json.RawMessage, error) {
	var decision domain.StrategyDecision
	if err := json.Unmarshal(payload, &decision); err != nil {
		return nil, err
	}
	if err := svc.StrategyApplyDecision(ctx, decision); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "applied"})
}

func marshalResult(v any, _ ...any) (json.RawMessage, error) {
	return json.Marshal(v)
}
