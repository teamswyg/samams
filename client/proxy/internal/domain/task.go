package domain

import "time"

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusPlanning  TaskStatus = "planning"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusPaused    TaskStatus = "paused"
	TaskStatusDone      TaskStatus = "done"
	TaskStatusStopped   TaskStatus = "stopped"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusError     TaskStatus = "error"
	TaskStatusScaling   TaskStatus = "scaling"
	TaskStatusResetting TaskStatus = "resetting"
)

type Task struct {
	ID               string     `json:"id"`
	Name             string     `json:"name,omitempty"`
	Prompt           string     `json:"prompt"`
	CursorArgs       []string   `json:"cursorArgs,omitempty"`
	Tags             []string   `json:"tags,omitempty"`
	AgentIDs         []string   `json:"agentIds"`
	Status           TaskStatus `json:"status"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	BoundedContextID string     `json:"boundedContextId,omitempty"`
	ParentTaskID     string     `json:"parentTaskId,omitempty"`
	RetryCount       int        `json:"retryCount"`
	CancelReason     string     `json:"cancelReason,omitempty"`
	CancelledBy      string     `json:"cancelledBy,omitempty"`
	Summary          string     `json:"summary,omitempty"`
	ContextPlannedAt *time.Time `json:"contextPlannedAt,omitempty"`
	AgentType        AgentType  `json:"agentType,omitempty"`
	Mode             AgentMode  `json:"mode,omitempty"`
	NodeType         string     `json:"nodeType,omitempty"`
	NodeUID          string     `json:"nodeUid,omitempty"`
	BranchName       string     `json:"branchName,omitempty"`
	ParentBranch     string     `json:"parentBranch,omitempty"`
	WorktreePath     string     `json:"worktreePath,omitempty"`
	ProjectName      string     `json:"projectName,omitempty"`
}

type CreateTaskParams struct {
	Name             string
	Prompt           string
	NumAgents        int
	CursorArgs       []string
	Tags             []string
	BoundedContextID string
	ParentTaskID     string
	AgentType        AgentType
	Mode             AgentMode
	NodeType         string
	NodeUID          string
	ParentBranch     string
	ProjectName      string
}

type ScaleTaskParams struct {
	NumAgents int
}

type StopTaskOptions struct {
	Graceful    bool
	Reason      string
	CancelledBy string
}

type TaskSummary struct {
	Task   *Task   `json:"task"`
	Agents []Agent `json:"agents"`
	Logs   []string `json:"logs,omitempty"`
}
