package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"proxy/internal/domain"
	"proxy/internal/port"
)

const (
	eventSourceProxy       = "agent-proxy"
	retryFailThreshold     = 5
)

// agentRuntime wraps a domain Agent with application-level state.
type agentRuntime struct {
	agent *domain.Agent
	logs  *LogBuffer
}

// strategyWorkerInfo holds agent/task context for strategy meeting operations.
type strategyWorkerInfo struct {
	agentID  string
	taskID   string
	nodeUID  string
	worktree string
}

// TaskService orchestrates Tasks and Agents. Implements port.TaskService.
type TaskService struct {
	mu            sync.RWMutex
	tasks         map[string]*domain.Task
	agents        map[string]*agentRuntime
	watchAgents   map[string]*agentRuntime // separate pool for strategy watch agents
	runner        port.Runner
	branches      port.BranchManager
	logLines      int
	maxTasks      int
	maxAgents     int
	maxWatchAgents int
	publisher     port.Publisher
	maal          *MaalStore
	notifications *NotificationStore

	// proposalRunning: true while a proposal setup agent is working.
	// Blocks all non-proposal task creation until proposal completes.
	proposalRunning bool
	proposalRetries int
}

func NewTaskService(r port.Runner, opts ...Option) *TaskService {
	s := &TaskService{
		tasks:          make(map[string]*domain.Task),
		agents:         make(map[string]*agentRuntime),
		watchAgents:    make(map[string]*agentRuntime),
		runner:         r,
		logLines:       200,
		maxTasks:       100,
		maxAgents:      200,
		maxWatchAgents: 6,
		publisher:      port.NoopPublisher{},
		maal:           NewMaalStore(10000),
		notifications:  NewNotificationStore(1000),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *TaskService) CreateTask(ctx context.Context, params domain.CreateTaskParams) (*domain.TaskSummary, error) {
	if params.NumAgents <= 0 {
		params.NumAgents = 1
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Block non-proposal tasks while proposal setup is running.
	if s.proposalRunning && params.NodeType != "proposal" {
		return nil, fmt.Errorf("proposal setup in progress — task creation blocked until skeleton is ready")
	}

	// If this is a proposal task, set the guard.
	if params.NodeType == "proposal" {
		s.proposalRunning = true
	}

	if len(s.tasks) >= s.maxTasks {
		return nil, fmt.Errorf("max tasks reached: %d", s.maxTasks)
	}
	if len(s.agents)+params.NumAgents > s.maxAgents {
		return nil, fmt.Errorf("creating %d agents would exceed max agents %d", params.NumAgents, s.maxAgents)
	}

	if params.AgentType == "" {
		params.AgentType = domain.AgentTypeCursor
	}
	if params.Mode == "" {
		params.Mode = domain.AgentModeExecute
	}

	branchName := domain.BranchName(params.NodeType, params.NodeUID, params.Name)
	parentBranch := params.ParentBranch
	if parentBranch == "" {
		parentBranch = "main"
	}
	var worktreePath string

	if (params.NodeType == "review" || params.NodeType == "strategy-discussion") && s.branches != nil {
		// Review task: use existing worktree, do NOT create new branch.
		existingPath := s.branches.WorktreePath(params.ParentBranch)
		if existingPath != "" {
			worktreePath = existingPath
			log.Printf("[workspace] Review agent using existing worktree: %s", existingPath)
		} else {
			log.Printf("[workspace] Warning: no worktree found for %s, review agent will use default workdir", params.ParentBranch)
		}
	} else if params.NodeType == "proposal" && s.branches != nil {
		// Proposal creates main: git init + initial skeleton.
		// Milestones will fork from main, tasks will fork from milestones.
		s.mu.Unlock()
		mainPath, err := s.branches.InitWorkspace(params.ProjectName)
		if err != nil {
			s.mu.Lock()
			log.Printf("[workspace] Warning: workspace init failed: %v", err)
		} else {
			s.mu.Lock()
			worktreePath = mainPath
			log.Printf("[workspace] Initialized main: %s", mainPath)
		}
	} else if branchName != "" && s.branches != nil {
		// Milestone/task: create worktree from parent branch.
		s.mu.Unlock()
		wt, err := s.branches.CreateWorktree(params.ProjectName, branchName, parentBranch)
		if err != nil {
			s.mu.Lock()
			log.Printf("[worktree] Warning: worktree creation failed for %s: %v", branchName, err)
		} else {
			s.mu.Lock()
			worktreePath = wt
			log.Printf("[worktree] Agent will work in: %s", wt)
		}
	}

	taskID := newID()
	task := &domain.Task{
		ID:               taskID,
		Name:             params.Name,
		Prompt:           params.Prompt,
		CursorArgs:       append([]string{}, params.CursorArgs...),
		Tags:             append([]string{}, params.Tags...),
		AgentIDs:         make([]string, 0, params.NumAgents),
		Status:           domain.TaskStatusRunning,
		CreatedAt:        now,
		UpdatedAt:        now,
		BoundedContextID: params.BoundedContextID,
		ParentTaskID:     params.ParentTaskID,
		AgentType:        params.AgentType,
		Mode:             params.Mode,
		NodeType:         params.NodeType,
		NodeUID:          params.NodeUID,
		BranchName:       branchName,
		ParentBranch:     parentBranch,
		WorktreePath:     worktreePath,
		ProjectName:      params.ProjectName,
	}

	s.tasks[taskID] = task
	s.emitLocked(ctx, s.taskEnvelope(domain.EventTaskCreated, taskID, nil))
	agents := make([]domain.Agent, 0, params.NumAgents)

	for i := 0; i < params.NumAgents; i++ {
		ar, err := s.startAgentLocked(ctx, task, params.AgentType, params.Mode)
		if err != nil {
			return nil, fmt.Errorf("failed to start agent %d: %w", i, err)
		}
		agents = append(agents, ar.agent.Snapshot())
	}

	return &domain.TaskSummary{Task: task, Agents: agents}, nil
}

func (s *TaskService) ListTasks() []*domain.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*domain.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		cp := *t
		out = append(out, &cp)
	}
	return out
}

func (s *TaskService) GetTask(taskID string) (*domain.TaskSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, domain.ErrTaskNotFound
	}

	agents := make([]domain.Agent, 0, len(task.AgentIDs))
	var mergedLogs []string
	for _, id := range task.AgentIDs {
		ar, ok := s.agents[id]
		if !ok {
			continue
		}
		agents = append(agents, ar.agent.Snapshot())
		if ar.logs != nil {
			mergedLogs = append(mergedLogs, ar.logs.Lines()...)
		}
	}

	return &domain.TaskSummary{Task: task, Agents: agents, Logs: mergedLogs}, nil
}

func (s *TaskService) ListAgents() []domain.Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.Agent, 0, len(s.agents)+len(s.watchAgents))
	for _, ar := range s.agents {
		out = append(out, ar.agent.Snapshot())
	}
	for _, ar := range s.watchAgents {
		out = append(out, ar.agent.Snapshot())
	}
	return out
}

func (s *TaskService) GetAgent(agentID string) (*domain.Agent, []string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ar, ok := s.agents[agentID]
	if !ok {
		return nil, nil, domain.ErrAgentNotFound
	}
	var logs []string
	if ar.logs != nil {
		logs = ar.logs.Lines()
	}
	snap := ar.agent.Snapshot()
	return &snap, logs, nil
}

func (s *TaskService) ScaleTask(ctx context.Context, taskID string, params domain.ScaleTaskParams) (*domain.TaskSummary, error) {
	if params.NumAgents <= 0 {
		return nil, fmt.Errorf("numAgents must be > 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, domain.ErrTaskNotFound
	}

	current := len(task.AgentIDs)
	target := params.NumAgents

	if target == current {
		return s.buildTaskSummaryLocked(task), nil
	}
	if len(s.agents)+target-current > s.maxAgents {
		return nil, fmt.Errorf("scaling to %d agents would exceed max agents %d", target, s.maxAgents)
	}

	_ = domain.TransitionTask(task, domain.TaskStatusScaling)
	task.UpdatedAt = time.Now().UTC()

	if target > current {
		diff := target - current
		agentType, mode := task.AgentType, task.Mode
		if agentType == "" {
			agentType = domain.AgentTypeCursor
		}
		if mode == "" {
			mode = domain.AgentModeExecute
		}
		for i := 0; i < diff; i++ {
			if _, err := s.startAgentLocked(ctx, task, agentType, mode); err != nil {
				return nil, fmt.Errorf("failed to start additional agent: %w", err)
			}
		}
	} else {
		diff := current - target
		for i := 0; i < diff; i++ {
			agentID := task.AgentIDs[len(task.AgentIDs)-1]
			task.AgentIDs = task.AgentIDs[:len(task.AgentIDs)-1]
			if _, ok := s.agents[agentID]; ok {
				go s.stopAgentAsync(context.Background(), agentID)
			}
		}
	}

	_ = domain.TransitionTask(task, domain.TaskStatusRunning)
	task.UpdatedAt = time.Now().UTC()

	return s.buildTaskSummaryLocked(task), nil
}

func (s *TaskService) StopTask(ctx context.Context, taskID string, opts *domain.StopTaskOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return domain.ErrTaskNotFound
	}

	for _, id := range task.AgentIDs {
		if _, ok := s.agents[id]; ok {
			go s.stopAgentAsync(context.Background(), id)
		}
	}

	if opts != nil && opts.Graceful {
		_ = domain.TransitionTask(task, domain.TaskStatusPaused)
		s.emitLocked(ctx, s.agentStateEnvelope(domain.EventAgentPaused, "", taskID, "running", "paused", opts.Reason))
	} else {
		_ = domain.TransitionTask(task, domain.TaskStatusStopped)
		s.emitLocked(ctx, s.taskEnvelope(domain.EventTaskHardStopped, taskID, map[string]any{"reason": "hard stop"}))
		if opts != nil {
			task.CancelReason = opts.Reason
			task.CancelledBy = opts.CancelledBy
			if opts.CancelledBy != "" {
				_ = domain.TransitionTask(task, domain.TaskStatusCancelled)
				s.emitLocked(ctx, s.taskEnvelope(domain.EventTaskCancelled, taskID, map[string]any{
					"reason": opts.Reason, "cancelledBy": opts.CancelledBy, "graceful": false,
				}))
				s.notifyLocked(domain.Notification{
					ID: newID(), TaskID: taskID, Title: "Task cancelled",
					Body: opts.Reason, Severity: "warning", CreatedAt: time.Now().UTC(),
				})
			}
		}
		s.emitLocked(ctx, s.taskEnvelope(domain.EventTaskStatusUpdated, taskID, map[string]any{"status": string(task.Status)}))
	}
	task.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *TaskService) PauseTask(ctx context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return domain.ErrTaskNotFound
	}
	if task.Status != domain.TaskStatusRunning && task.Status != domain.TaskStatusScaling {
		return fmt.Errorf("task is not running (status: %s)", task.Status)
	}

	// Interrupt agents (2x SIGINT — process stays alive in input-waiting mode).
	for _, id := range task.AgentIDs {
		if _, ok := s.agents[id]; ok {
			if err := s.runner.InterruptAgent(ctx, id); err != nil {
				log.Printf("[service] InterruptAgent %s failed: %v", id, err)
			}
		}
	}

	_ = domain.TransitionTask(task, domain.TaskStatusPaused)
	task.UpdatedAt = time.Now().UTC()
	s.emitLocked(ctx, s.agentStateEnvelope(domain.EventAgentPaused, "", taskID, "running", "paused", "user pause"))
	return nil
}

func (s *TaskService) ResumeTask(ctx context.Context, taskID string) (*domain.TaskSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, domain.ErrTaskNotFound
	}
	if task.Status != domain.TaskStatusPaused {
		return nil, fmt.Errorf("task is not paused (status: %s)", task.Status)
	}

	// Try to resume existing agents via SendInput (interrupt-based pause keeps process alive).
	resumed := 0
	for _, id := range task.AgentIDs {
		if _, ok := s.agents[id]; ok {
			if err := s.runner.SendInput(id, "resume"); err != nil {
				log.Printf("[service] SendInput resume to %s failed: %v (agent may be dead)", id, err)
			} else {
				resumed++
			}
		}
	}

	// Fallback: if no agents could be resumed, create new ones.
	if resumed == 0 {
		log.Printf("[service] No agents resumed for task %s — creating new agents", taskID)
		n := len(task.AgentIDs)
		if n == 0 {
			n = 1
		}
		agentType, mode := task.AgentType, task.Mode
		if agentType == "" {
			agentType = domain.AgentTypeCursor
		}
		if mode == "" {
			mode = domain.AgentModeExecute
		}
		task.AgentIDs = task.AgentIDs[:0]
		for i := 0; i < n; i++ {
			if _, err := s.startAgentLocked(ctx, task, agentType, mode); err != nil {
				return nil, fmt.Errorf("resume agent %d: %w", i, err)
			}
		}
	}

	task.Status = domain.TaskStatusRunning
	task.UpdatedAt = time.Now().UTC()
	s.emitLocked(ctx, s.taskEnvelope(domain.EventTaskStatusUpdated, taskID, map[string]any{"status": string(domain.TaskStatusRunning)}))
	return s.buildTaskSummaryLocked(task), nil
}

func (s *TaskService) CancelTask(ctx context.Context, taskID string, reason, cancelledBy string) error {
	if cancelledBy == "" {
		cancelledBy = "user"
	}
	return s.StopTask(ctx, taskID, &domain.StopTaskOptions{
		Graceful: false, Reason: reason, CancelledBy: cancelledBy,
	})
}

func (s *TaskService) IncrementRetryCount(ctx context.Context, taskID string) (newCount int, triggeredMeeting bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return 0, false
	}
	task.RetryCount++
	task.UpdatedAt = time.Now().UTC()
	e := s.taskEnvelope(domain.EventTaskRetryIncremented, taskID, map[string]any{"retryCount": task.RetryCount})
	e.Metadata["retry_count"] = fmt.Sprintf("%d", task.RetryCount)
	s.emitLocked(ctx, e)

	if task.RetryCount >= retryFailThreshold {
		s.emitLocked(ctx, s.taskEnvelope(domain.EventControlContextReset, taskID, map[string]any{"scope": "sentinel", "reason": "retry_limit"}))
		s.emitLocked(ctx, s.taskEnvelope(domain.EventStrategyMeetingRequested, taskID, map[string]any{
			"trigger": "retry_limit_reached", "related_task_ids": []string{taskID},
		}))
		return task.RetryCount, true
	}
	return task.RetryCount, false
}

func (s *TaskService) UpdateTaskSummary(ctx context.Context, taskID string, summary string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return domain.ErrTaskNotFound
	}
	task.Summary = summary
	task.UpdatedAt = time.Now().UTC()
	s.emitLocked(ctx, s.taskEnvelope(domain.EventTaskSummaryUpdated, taskID, map[string]any{"summary": summary}))
	return nil
}

func (s *TaskService) SetContextPlanned(ctx context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return domain.ErrTaskNotFound
	}
	now := time.Now().UTC()
	task.ContextPlannedAt = &now
	task.UpdatedAt = now
	s.emitLocked(ctx, s.taskEnvelope(domain.EventTaskContextPlanned, taskID, nil))
	return nil
}

func (s *TaskService) GetMaalByTaskID(taskID string) []domain.MaalRecord {
	if s.maal == nil {
		return nil
	}
	return s.maal.ByTaskID(taskID)
}

func (s *TaskService) GetMaalByAgentID(agentID string) []domain.MaalRecord {
	if s.maal == nil {
		return nil
	}
	return s.maal.ByAgentID(agentID)
}

// GetRecentLogs returns recent MAAL records formatted as LogEntry for the frontend.
func (s *TaskService) GetRecentLogs() []domain.LogEntry {
	if s.maal == nil {
		return nil
	}
	records := s.maal.All()

	// Take last 100 records.
	start := 0
	if len(records) > 100 {
		start = len(records) - 100
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]domain.LogEntry, 0, len(records)-start)
	for _, r := range records[start:] {
		logType := "INFO"
		switch r.Action {
		case "error":
			logType = "ERROR"
		case "stop_requested":
			logType = "WARN"
		}

		agentName := r.AgentID
		if ar, ok := s.agents[r.AgentID]; ok {
			agentName = string(ar.agent.AgentType) + " " + r.AgentID[:8]
		}

		entries = append(entries, domain.LogEntry{
			ID:      r.ID,
			Time:    r.Timestamp.Format("15:04:05"),
			Type:    logType,
			Agent:   agentName,
			Message: r.Content,
		})
	}
	return entries
}

func (s *TaskService) ListNotifications() []domain.Notification {
	if s.notifications == nil {
		return nil
	}
	return s.notifications.List()
}

func (s *TaskService) StrategyPauseAll(ctx context.Context, participantNodeUIDs []string) error {
	// Build a set of participant UIDs for O(1) lookup.
	participantSet := make(map[string]bool, len(participantNodeUIDs))
	for _, uid := range participantNodeUIDs {
		participantSet[uid] = true
	}

	s.mu.RLock()
	var workers []strategyWorkerInfo
	for _, t := range s.tasks {
		if t.Status != domain.TaskStatusRunning && t.Status != domain.TaskStatusScaling {
			continue
		}
		// If participant list provided, only include matching tasks.
		if len(participantSet) > 0 && !participantSet[t.NodeUID] {
			continue
		}
		for _, aid := range t.AgentIDs {
			if _, ok := s.agents[aid]; ok {
				workers = append(workers, strategyWorkerInfo{
					agentID: aid, taskID: t.ID, nodeUID: t.NodeUID, worktree: t.WorktreePath,
				})
			}
		}
	}
	s.mu.RUnlock()

	if len(workers) == 0 {
		log.Println("[strategy] No running workers to pause — emitting allPaused immediately")
		e := domain.NewEnvelope(domain.EventStrategyAllPaused, eventSourceProxy)
		e.Payload = map[string]any{"discussionContexts": map[string]string{}}
		s.emit(ctx, e)
		return nil
	}

	// Fire-and-forget: return immediately, do the work in a goroutine.
	// The server gets an immediate ack; the allPaused event arrives when ready.
	go s.strategyPauseAllAsync(workers)
	return nil
}

// strategyPauseAllAsync runs the full pause sequence in a background goroutine.
func (s *TaskService) strategyPauseAllAsync(workers []strategyWorkerInfo) {
	ctx := context.Background()

	// 1. Interrupt all workers (2x SIGINT — process stays alive in input-waiting mode).
	for _, w := range workers {
		if err := s.runner.InterruptAgent(ctx, w.agentID); err != nil {
			log.Printf("[strategy] InterruptAgent %s failed: %v", w.agentID, err)
		}
	}

	// 2. Wait for workers to transition to input-waiting.
	time.Sleep(2 * time.Second)

	// 3. Mark tasks as paused.
	s.mu.Lock()
	for _, w := range workers {
		if t, ok := s.tasks[w.taskID]; ok {
			_ = domain.TransitionTask(t, domain.TaskStatusPaused)
			t.UpdatedAt = time.Now().UTC()
		}
	}
	s.mu.Unlock()

	// 4. Launch watch agents (max 6, batched).
	discussionContexts := make(map[string]string)
	batchSize := s.maxWatchAgents
	for i := 0; i < len(workers); i += batchSize {
		end := i + batchSize
		if end > len(workers) {
			end = len(workers)
		}
		batch := workers[i:end]

		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, w := range batch {
			if w.worktree == "" {
				continue
			}
			wg.Add(1)
			go func(info strategyWorkerInfo) {
				defer wg.Done()
				result := s.runWatchAgent(ctx, info.nodeUID, info.worktree)
				mu.Lock()
				discussionContexts[info.nodeUID] = result
				mu.Unlock()
			}(w)
		}
		wg.Wait()
	}

	// 5. Emit allPaused with discussionContexts.
	e := domain.NewEnvelope(domain.EventStrategyAllPaused, eventSourceProxy)
	e.Payload = map[string]any{"discussionContexts": discussionContexts}
	s.emit(ctx, e)
	log.Printf("[strategy] All agents interrupted (%d workers), watch agents collected %d contexts", len(workers), len(discussionContexts))
}

// runWatchAgent spawns a temporary watch agent in the worker's worktree,
// waits for it to write .samams-context.md, then collects the result.
func (s *TaskService) runWatchAgent(ctx context.Context, nodeUID, worktreePath string) string {
	watchID := "watch-" + newID()[:12]
	logs := NewLogBuffer(100)

	opts := port.StartOptions{
		Prompt:  "You are a strategy meeting analyst. Run `git diff HEAD~5..HEAD` and `git log --oneline -10` to see recent changes. Analyze the worktree for conflicts. IMPORTANT: If .samams-context.md already exists, APPEND your analysis to the bottom of the file (do NOT overwrite existing content). Add a `## watch-agent` header before your analysis. Write with sections: ### My Progress, ### Files Modified, ### Git Diff Summary, ### Potential Conflicts, ### Proposed Solutions. Then exit immediately.",
		WorkDir: worktreePath,
	}

	handle, err := s.runner.StartAgent(ctx, watchID, opts, logs.Append)
	if err != nil {
		log.Printf("[strategy] Failed to start watch agent for %s: %v", nodeUID, err)
		return ""
	}

	// Track watch agent.
	s.mu.Lock()
	watchAgent := &domain.Agent{
		ID:        watchID,
		Name:      "Watch-" + nodeUID,
		TaskID:    nodeUID,
		NodeUID:   nodeUID,
		AgentType: domain.AgentTypeCursor,
		Mode:      "watch",
		Status:    domain.AgentStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	watchAR := &agentRuntime{agent: watchAgent, logs: logs}
	s.watchAgents[watchID] = watchAR
	s.mu.Unlock()

	// Wait with timeout.
	done := make(chan error, 1)
	go func() { done <- handle.Wait() }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		_ = s.runner.StopAgent(ctx, watchID)
	}

	// Cleanup watch agent.
	s.mu.Lock()
	delete(s.watchAgents, watchID)
	s.mu.Unlock()

	// Collect .samams-context.md.
	contextPath := worktreePath + "/.samams-context.md"
	data, err := os.ReadFile(contextPath)
	if err != nil {
		log.Printf("[strategy] Watch agent %s: no context file found", nodeUID)
		return ""
	}
	os.Remove(contextPath)
	log.Printf("[strategy] Watch agent %s: collected %d bytes", nodeUID, len(data))
	return string(data)
}

func (s *TaskService) StrategyApplyDecision(ctx context.Context, decision domain.StrategyDecision) error {
	s.mu.RLock()
	// Build nodeUID → (taskID, agentID, branchName, ...) map.
	type taskRef struct {
		taskID      string
		agentID     string
		branchName  string
		worktree    string
		nodeUID     string
		nodeType    string
		projectName string
	}
	nodeMap := make(map[string]taskRef)
	for _, t := range s.tasks {
		if t.NodeUID == "" {
			continue
		}
		var agentID string
		if len(t.AgentIDs) > 0 {
			agentID = t.AgentIDs[0]
		}
		nodeMap[t.NodeUID] = taskRef{
			taskID: t.ID, agentID: agentID,
			branchName: t.BranchName, worktree: t.WorktreePath,
			nodeUID: t.NodeUID, nodeType: t.NodeType, projectName: t.ProjectName,
		}
	}
	s.mu.RUnlock()

	// Fire-and-forget: return ack immediately, apply actions in background.
	go func() {
		bgCtx := context.Background()
		for _, ta := range decision.TaskActions {
			ref, ok := nodeMap[ta.NodeUID]
			if !ok {
				log.Printf("[strategy] ApplyDecision: node %s not found", ta.NodeUID)
				continue
			}

			switch ta.Action {
			case "keep":
				// Resume: send "resume" to the interrupted worker.
				if ref.agentID != "" {
					if err := s.runner.SendInput(ref.agentID, "resume"); err != nil {
						log.Printf("[strategy] SendInput resume to %s failed: %v", ta.NodeUID, err)
					}
				}
				s.mu.Lock()
				if t, ok := s.tasks[ref.taskID]; ok {
					_ = domain.TransitionTask(t, domain.TaskStatusRunning)
					t.UpdatedAt = time.Now().UTC()
				}
				s.mu.Unlock()

			case "reset_and_retry":
				// 1. Reset worktree to HEAD.
				if s.branches != nil && ref.branchName != "" {
					if err := s.branches.ResetWorktree(ref.branchName); err != nil {
						log.Printf("[strategy] ResetWorktree %s failed: %v", ref.branchName, err)
					}
				}
				// 2. Write new prompt.
				if ta.NewPrompt != "" && ref.worktree != "" {
					promptPath := ref.worktree + "/.samams-prompt.md"
					if err := os.WriteFile(promptPath, []byte(ta.NewPrompt), 0644); err != nil {
						log.Printf("[strategy] Write prompt for %s failed: %v", ta.NodeUID, err)
					}
				}
				// 3. Clear context and give new instructions.
				if ref.agentID != "" {
					_ = s.runner.SendInput(ref.agentID, "/clear")
					time.Sleep(500 * time.Millisecond)
					_ = s.runner.SendInput(ref.agentID, "Read .samams-prompt.md and follow ALL instructions")
				}
				s.mu.Lock()
				if t, ok := s.tasks[ref.taskID]; ok {
					_ = domain.TransitionTask(t, domain.TaskStatusRunning)
					t.UpdatedAt = time.Now().UTC()
				}
				s.mu.Unlock()

			case "cancel":
				// 1. Kill the agent (4x SIGINT).
				if ref.agentID != "" {
					if err := s.runner.StopAgent(bgCtx, ref.agentID); err != nil {
						log.Printf("[strategy] StopAgent %s failed: %v", ta.NodeUID, err)
					}
				}
				// 2. Purge worktree + branch.
				if s.branches != nil && ref.branchName != "" {
					if err := s.branches.PurgeWorktree(ref.branchName); err != nil {
						log.Printf("[strategy] PurgeWorktree %s failed: %v", ref.branchName, err)
					}
				}
				// 3. Transition to cancelled directly — do NOT call CancelTask
				//    because StopAgent already killed the process; CancelTask would
				//    trigger another stopAgentAsync on a dead process (#13 double stop).
				s.mu.Lock()
				if t, ok := s.tasks[ref.taskID]; ok {
					_ = domain.TransitionTask(t, domain.TaskStatusCancelled)
					t.CancelReason = "strategy meeting decision: cancel"
					t.CancelledBy = "sentinel"
					t.UpdatedAt = time.Now().UTC()
				}
				s.mu.Unlock()
				s.emit(bgCtx, s.taskEnvelope(domain.EventTaskCancelled, ref.taskID, map[string]any{
					"reason":      "strategy meeting decision: cancel",
					"cancelledBy": "sentinel",
					"nodeUid":     ref.nodeUID,
					"nodeType":    ref.nodeType,
					"projectName": ref.projectName,
				}))
			}
		}

		s.emit(bgCtx, domain.NewEnvelope(domain.EventStrategyDecisionApplied, eventSourceProxy))
		log.Printf("[strategy] Decision applied: %d actions processed", len(decision.TaskActions))
	}()

	return nil
}

func (s *TaskService) ResetTask(ctx context.Context, taskID string) (*domain.TaskSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, domain.ErrTaskNotFound
	}

	_ = domain.TransitionTask(task, domain.TaskStatusResetting)
	task.UpdatedAt = time.Now().UTC()

	for _, id := range task.AgentIDs {
		if _, ok := s.agents[id]; ok {
			go s.stopAgentAsync(ctx, id)
		}
	}

	agentCount := len(task.AgentIDs)
	task.AgentIDs = task.AgentIDs[:0]
	if agentCount == 0 {
		agentCount = 1
	}
	agentType, mode := task.AgentType, task.Mode
	if agentType == "" {
		agentType = domain.AgentTypeCursor
	}
	if mode == "" {
		mode = domain.AgentModeExecute
	}
	agents := make([]domain.Agent, 0, agentCount)
	for i := 0; i < agentCount; i++ {
		ar, err := s.startAgentLocked(ctx, task, agentType, mode)
		if err != nil {
			return nil, fmt.Errorf("failed to start agent during reset: %w", err)
		}
		agents = append(agents, ar.agent.Snapshot())
	}

	task.Status = domain.TaskStatusRunning
	task.UpdatedAt = time.Now().UTC()

	s.emitLocked(ctx, s.taskEnvelope(domain.EventTaskRedistributed, taskID, map[string]any{
		"agentCount": agentCount, "reason": "task reset and redistributed",
	}))

	return &domain.TaskSummary{Task: task, Agents: agents}, nil
}

func (s *TaskService) StopAgent(ctx context.Context, agentID string) error {
	return s.stopAgentAsync(ctx, agentID)
}

func (s *TaskService) stopAgentAsync(ctx context.Context, agentID string) error {
	s.mu.Lock()
	ar, ok := s.agents[agentID]
	if !ok {
		s.mu.Unlock()
		return domain.ErrAgentNotFound
	}
	taskID := ar.agent.TaskID
	s.maal.Append(domain.MaalRecord{
		ID: newID(), AgentID: agentID, TaskID: taskID,
		Timestamp: time.Now().UTC(), Action: "stop_requested", Content: "stop requested",
	})
	s.mu.Unlock()

	if err := s.runner.StopAgent(ctx, agentID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if ar, ok = s.agents[agentID]; ok {
		now := time.Now().UTC()
		_ = ar.agent.Transition(domain.AgentStatusStopped, "stop requested")
		ar.agent.StoppedAt = &now
	}
	return nil
}

func (s *TaskService) CreateSkeleton(spec domain.SkeletonSpec) (string, error) {
	if s.branches == nil {
		return "", fmt.Errorf("branch manager not configured")
	}
	mainPath, err := s.branches.CreateSkeleton(spec)
	if err != nil {
		return "", err
	}
	s.emit(context.Background(), domain.Envelope{
		ID: newID(), Type: domain.EventProjectCreated, Source: eventSourceProxy,
		Timestamp: time.Now().UnixMilli(),
		Payload:   map[string]any{"projectName": spec.ProjectName, "mainPath": mainPath},
		Metadata:  map[string]string{"category": "project"},
	})
	return mainPath, nil
}

func (s *TaskService) GetAgentLogs(agentID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ar, ok := s.agents[agentID]
	if !ok {
		return nil, domain.ErrAgentNotFound
	}
	if ar.logs == nil {
		return nil, nil
	}
	return ar.logs.Lines(), nil
}

func (s *TaskService) MergeMilestone(ctx context.Context, branchName, targetBranch string) error {
	if s.branches == nil {
		return fmt.Errorf("branch manager not configured")
	}
	log.Printf("[service] Merging milestone %s → %s", branchName, targetBranch)
	if err := s.branches.MergeBack(branchName, targetBranch); err != nil {
		return fmt.Errorf("merge milestone: %w", err)
	}

	// Milestone approved — purge all child task worktrees that used this milestone as parent.
	s.mu.RLock()
	var childBranches []string
	for _, t := range s.tasks {
		if t.ParentBranch == branchName && t.BranchName != "" {
			childBranches = append(childBranches, t.BranchName)
		}
	}
	s.mu.RUnlock()
	for _, cb := range childBranches {
		if err := s.branches.PurgeWorktree(cb); err != nil {
			log.Printf("[service] Cleanup child worktree %s: %v", cb, err)
		}
	}
	if len(childBranches) > 0 {
		log.Printf("[service] Cleaned up %d child task worktrees for milestone %s", len(childBranches), branchName)
	}

	s.emit(ctx, domain.NewEnvelope(domain.EventMilestoneMerged, eventSourceProxy))
	log.Printf("[service] Milestone merged: %s → %s", branchName, targetBranch)
	return nil
}

func (s *TaskService) AppendAgentInput(agentID, input string) error {
	return s.runner.SendInput(agentID, input)
}

func (s *TaskService) buildTaskSummaryLocked(task *domain.Task) *domain.TaskSummary {
	agents := make([]domain.Agent, 0, len(task.AgentIDs))
	for _, id := range task.AgentIDs {
		if ar, ok := s.agents[id]; ok {
			agents = append(agents, ar.agent.Snapshot())
		}
	}
	return &domain.TaskSummary{Task: task, Agents: agents}
}

func (s *TaskService) startAgentLocked(ctx context.Context, task *domain.Task, agentType domain.AgentType, mode domain.AgentMode) (*agentRuntime, error) {
	agentID := newID()
	// Pick a name not used by any active agent.
	usedNames := make(map[string]bool, len(s.agents))
	for _, ar := range s.agents {
		usedNames[ar.agent.Name] = true
	}

	agent := &domain.Agent{
		ID:        agentID,
		Name:      domain.PickAvailableName(usedNames),
		TaskID:    task.ID,
		TaskName:  task.Name,
		NodeUID:   task.NodeUID,
		AgentType: agentType,
		Mode:      mode,
		Status:    "idle",
		CreatedAt: time.Now().UTC(),
	}
	agent.InitStateMachine()
	_ = agent.Transition(domain.AgentStatusStarting, "agent created")

	logs := NewLogBuffer(s.logLines)
	ar := &agentRuntime{agent: agent, logs: logs}

	opts := port.StartOptions{
		Prompt:     task.Prompt,
		CursorArgs: append([]string{}, task.CursorArgs...),
		WorkDir:    task.WorktreePath, // agent works in isolated worktree
	}

	// Proposal setup has a 5-minute timeout — skeleton only, no coding.
	agentCtx := context.Background()
	if task.NodeType == "proposal" {
		var cancel context.CancelFunc
		agentCtx, cancel = context.WithTimeout(agentCtx, 5*time.Minute)
		_ = cancel // cancel is called when process exits via handle.Wait()
	}

	handle, err := s.runner.StartAgent(agentCtx, agentID, opts, logs.Append)
	if err != nil {
		return nil, err
	}

	s.agents[agentID] = ar
	task.AgentIDs = append(task.AgentIDs, agentID)
	task.UpdatedAt = time.Now().UTC()
	_ = agent.Transition(domain.AgentStatusRunning, "process started")

	s.emitLocked(ctx, s.agentEnvelope(domain.EventAgentCreated, agentID, task.ID, map[string]any{
		"agentType": string(agentType), "mode": string(mode),
	}))
	s.emitLocked(ctx, s.agentEnvelope(domain.EventAgentAssigned, agentID, task.ID, nil))
	s.maal.Append(domain.MaalRecord{
		ID: newID(), AgentID: agentID, TaskID: task.ID,
		Timestamp: time.Now().UTC(), Action: "started", Content: "agent started",
	})

	go s.watchAgent(agentID, handle)

	// Periodically analyze CLI output and emit human-readable MAAL entries.
	go s.analyzeAgentLogs(agentID, logs)

	return ar, nil
}

// analyzeAgentLogs periodically reads new CLI stdout lines and emits MAAL summaries.
// Runs until the agent is removed from s.agents.
func (s *TaskService) analyzeAgentLogs(agentID string, logBuf *LogBuffer) {
	lastAnalyzed := 0
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		ar, ok := s.agents[agentID]
		s.mu.RUnlock()
		if !ok {
			return // agent gone, stop analyzing
		}

		lines := logBuf.Lines()
		if len(lines) <= lastAnalyzed {
			continue
		}

		// Take new lines since last analysis (max 50 lines to avoid huge prompts).
		newLines := lines[lastAnalyzed:]
		if len(newLines) > 50 {
			newLines = newLines[len(newLines)-50:]
		}
		lastAnalyzed = len(lines)

		// Build a concise activity summary from raw CLI output.
		var chunk strings.Builder
		for _, l := range newLines {
			chunk.WriteString(l)
			chunk.WriteByte('\n')
		}

		// Emit as MAAL record with the raw activity (server-side OpenAI analysis is optional).
		summary := extractActivityHint(newLines)
		if summary != "" {
			s.mu.Lock()
			agentName := ar.agent.Name
			taskID := ar.agent.TaskID
			s.maal.Append(domain.MaalRecord{
				ID: newID(), AgentID: agentID, TaskID: taskID,
				Timestamp: time.Now().UTC(), Action: "activity",
				Content: agentName + ": " + summary,
			})
			s.mu.Unlock()
		}
	}
}

// extractActivityHint creates a brief human-readable hint from CLI output lines.
func extractActivityHint(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	var files []string
	var lastAction string
	for _, l := range lines {
		lower := strings.ToLower(l)
		// Detect file operations.
		if strings.Contains(lower, "created") || strings.Contains(lower, "wrote") || strings.Contains(lower, "modified") {
			parts := strings.Fields(l)
			for _, p := range parts {
				if strings.Contains(p, "/") || strings.Contains(p, ".go") || strings.Contains(p, ".js") || strings.Contains(p, ".ts") {
					files = append(files, p)
				}
			}
		}
		// Detect tool use.
		if strings.Contains(lower, "edit") || strings.Contains(lower, "write") || strings.Contains(lower, "read") ||
			strings.Contains(lower, "bash") || strings.Contains(lower, "search") {
			lastAction = l
		}
	}

	if len(files) > 0 {
		if len(files) > 3 {
			return fmt.Sprintf("Working on %d files: %s ...", len(files), strings.Join(files[:3], ", "))
		}
		return "Working on: " + strings.Join(files, ", ")
	}
	if lastAction != "" {
		if len(lastAction) > 80 {
			lastAction = lastAction[:80] + "..."
		}
		return lastAction
	}
	return fmt.Sprintf("Processing (%d lines of output)", len(lines))
}

func (s *TaskService) watchAgent(agentID string, handle *port.Handle) {
	err := handle.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()

	ar, ok := s.agents[agentID]
	if !ok {
		return
	}
	now := time.Now().UTC()
	ar.agent.StoppedAt = &now
	taskID := ar.agent.TaskID

	if err != nil {
		if ar.logs != nil {
			ar.logs.Append("agent process error: " + err.Error())
		}
		_ = ar.agent.Transition(domain.AgentStatusError, err.Error())

		// Include nodeUid + projectName + nodeType so server can update tree.json + SSOT.
		errorPayload := map[string]any{"message": err.Error()}
		if task, ok := s.tasks[taskID]; ok {
			errorPayload["nodeUid"] = task.NodeUID
			errorPayload["projectName"] = task.ProjectName
			errorPayload["nodeType"] = task.NodeType
		}
		s.emitLocked(context.Background(), s.agentEnvelope(domain.EventAgentError, agentID, taskID, errorPayload))
		s.maal.Append(domain.MaalRecord{ID: newID(), AgentID: agentID, TaskID: taskID, Timestamp: now, Action: "error", Content: err.Error()})

		// Keep errored task worktrees — agent may have committed partial work
		// that's useful for debugging or manual recovery. The branch and worktree
		// stay on disk; SSOT release + milestone check happen server-side.
		if task, ok := s.tasks[taskID]; ok && task.BranchName != "" {
			log.Printf("[worktree] Preserving errored task worktree: %s (branch: %s)", task.WorktreePath, task.BranchName)
		}

		// Proposal retry: if proposal agent failed, retry up to 3 times.
		if task, ok := s.tasks[taskID]; ok && task.NodeType == "proposal" {
			delete(s.agents, agentID)
			if s.proposalRetries < 3 {
				s.proposalRetries++
				log.Printf("[service] Proposal attempt %d/3 failed: %v — retrying...", s.proposalRetries, err)
				agentType, mode := task.AgentType, task.Mode
				if agentType == "" {
					agentType = domain.AgentTypeCursor
				}
				if mode == "" {
					mode = domain.AgentModeExecute
				}
				task.AgentIDs = task.AgentIDs[:0]
				if _, retryErr := s.startAgentLocked(context.Background(), task, agentType, mode); retryErr != nil {
					log.Printf("[service] Proposal retry failed: %v — unlocking task creation", retryErr)
					s.proposalRunning = false
					s.proposalRetries = 0
				}
				return
			}
			log.Printf("[service] Proposal failed after 3 attempts — task creation unblocked")
			s.proposalRunning = false
			s.proposalRetries = 0
			s.emitLocked(context.Background(), s.taskEnvelope(domain.EventTaskStatusUpdated, taskID, map[string]any{
				"status": string(domain.TaskStatusError), "reason": "proposal failed after 3 retries",
			}))
		} else {
			// Non-proposal agent error: remove from map to prevent agent slot leak.
			delete(s.agents, agentID)
		}
	} else {
		taskWorktree := ""
		taskPrompt := ""
		if task, ok := s.tasks[taskID]; ok {
			taskWorktree = task.WorktreePath
			taskPrompt = task.Prompt
		}
		agentContext := s.extractAgentContext(ar, taskWorktree)
		agentContext.Frontier = taskPrompt

		// Include full node info so server can build history with correct tree path.
		taskNodeType := ""
		taskNodeUID := ""
		taskParentBranch := ""
		taskProjectName := ""
		if task, ok := s.tasks[taskID]; ok {
			taskNodeType = task.NodeType
			taskNodeUID = task.NodeUID
			taskParentBranch = task.ParentBranch
			taskProjectName = task.ProjectName
		}
		// Review tasks emit a different event and skip merge-back.
		if task, ok := s.tasks[taskID]; ok && task.NodeType == "review" {
			s.emitLocked(context.Background(), s.taskEnvelope(domain.EventMilestoneReviewCompleted, taskID, map[string]any{
				"agentID":     agentID,
				"context":     agentContext.Context,
				"nodeUid":     taskNodeUID,
				"projectName": taskProjectName,
			}))
			_ = ar.agent.Transition(domain.AgentStatusStopped, "review completed")
			s.emitLocked(context.Background(), s.agentEnvelope(domain.EventAgentStopped, agentID, taskID, nil))
			task.Status = domain.TaskStatusDone
			task.UpdatedAt = time.Now().UTC()
			delete(s.agents, agentID)
			return // skip normal completion flow (no merge, no cascade)
		}

		s.clearAgentContext(ar)

		// Agent → stopped (task finished, not reused).
		_ = ar.agent.Transition(domain.AgentStatusStopped, "task completed")
		s.emitLocked(context.Background(), s.agentEnvelope(domain.EventAgentStopped, agentID, taskID, nil))
		s.maal.Append(domain.MaalRecord{ID: newID(), AgentID: agentID, TaskID: taskID, Timestamp: now, Action: "completed", Content: "task completed, context delivered to server"})

		// Task → done. If proposal, release the guard.
		if task, ok := s.tasks[taskID]; ok {
			if task.NodeType == "proposal" {
				s.proposalRunning = false
				s.proposalRetries = 0
				log.Printf("[service] Proposal setup complete — task creation unblocked")
			}
			_ = domain.TransitionTask(task, domain.TaskStatusDone)
			task.UpdatedAt = now
		}

		// 1. Merge FIRST — ensure code is in parent branch before notifying server.
		mergeFailed := false
		if task, ok := s.tasks[taskID]; ok && task.BranchName != "" && s.branches != nil {
			branchToMerge := task.BranchName
			parentBranch := task.ParentBranch
			s.mu.Unlock()
			if err := s.branches.MergeBack(branchToMerge, parentBranch); err != nil {
				log.Printf("[worktree] Merge failed %s → %s: %v", branchToMerge, parentBranch, err)
				mergeFailed = true
			} else {
				log.Printf("[worktree] Merged %s → %s (worktree removed)", branchToMerge, parentBranch)
			}
			s.mu.Lock()
			if !mergeFailed {
				task.WorktreePath = ""
			}
		}

		// 2. Emit events — merge success → task.completed, merge failure → task.failed.
		if mergeFailed {
			log.Printf("[service] Task %s completed but merge failed — emitting task.failed (node=%s)", taskID, taskNodeUID)
			s.emitLocked(context.Background(), s.taskEnvelope(domain.EventTaskCancelled, taskID, map[string]any{
				"agentID":      agentID,
				"nodeType":     taskNodeType,
				"nodeUid":      taskNodeUID,
				"projectName":  taskProjectName,
				"reason":       "merge conflict",
				"retryCount":   float64(0),
			}))
			if task, ok := s.tasks[taskID]; ok {
				task.Status = domain.TaskStatusError
				task.UpdatedAt = time.Now().UTC()
			}
		} else {
			s.emitLocked(context.Background(), s.taskEnvelope(domain.EventTaskCompleted, taskID, map[string]any{
				"agentID":      agentID,
				"context":      agentContext.Context,
				"frontier":     agentContext.Frontier,
				"nodeType":     taskNodeType,
				"nodeUid":      taskNodeUID,
				"parentBranch": taskParentBranch,
				"projectName":  taskProjectName,
			}))
			s.emitLocked(context.Background(), s.taskEnvelope(domain.EventTaskStatusUpdated, taskID, map[string]any{"status": string(domain.TaskStatusDone)}))
		}

		// Clean up agent from map.
		delete(s.agents, agentID)
	}
}

type agentContext struct {
	Context  string `json:"context"`
	Frontier string `json:"frontier"`
}

func (s *TaskService) extractAgentContext(ar *agentRuntime, worktreePath string) agentContext {
	ctx := agentContext{}

	// 1. Try reading .samams-context.md from worktree (structured output).
	if worktreePath != "" {
		contextPath := worktreePath + "/.samams-context.md"
		if data, err := os.ReadFile(contextPath); err == nil && len(data) > 0 {
			ctx.Context = string(data)
			os.Remove(contextPath) // clean up so it doesn't get committed in merges
			log.Printf("[context] Read .samams-context.md from %s (%d bytes)", worktreePath, len(data))
			return ctx
		}
	}

	// 2. Fallback: LogBuffer stdout (current behavior).
	if ar.logs != nil {
		lines := ar.logs.Lines()
		var buf strings.Builder
		for _, l := range lines {
			buf.WriteString(l)
			buf.WriteByte('\n')
		}
		ctx.Context = buf.String()
	}
	return ctx
}

func (s *TaskService) clearAgentContext(ar *agentRuntime) {
	if ar.logs != nil {
		ar.logs.Clear()
	}
}

func (s *TaskService) emitLocked(ctx context.Context, e domain.Envelope) {
	if s.publisher == nil {
		return
	}
	if err := s.publisher.Publish(ctx, e); err != nil {
		log.Printf("[service] Failed to publish event %s: %v", e.Type, err)
	}
}

func (s *TaskService) emit(ctx context.Context, e domain.Envelope) {
	if s.publisher == nil {
		return
	}
	if err := s.publisher.Publish(ctx, e); err != nil {
		log.Printf("[service] Failed to publish event %s: %v", e.Type, err)
	}
}

func (s *TaskService) taskEnvelope(t domain.EventType, taskID string, payload map[string]any) domain.Envelope {
	e := domain.NewEnvelope(t, eventSourceProxy)
	e.TaskID = taskID
	e.Metadata["category"] = "task"
	if payload != nil {
		e.Payload = payload
	}
	return e
}

func (s *TaskService) agentEnvelope(t domain.EventType, agentID, taskID string, payload map[string]any) domain.Envelope {
	e := domain.NewEnvelope(t, eventSourceProxy)
	e.TaskID = taskID
	e.Metadata["category"] = "agent"
	e.Metadata["agent_id"] = agentID
	if payload != nil {
		e.Payload = payload
	}
	return e
}

func (s *TaskService) agentStateEnvelope(t domain.EventType, agentID, taskID, prevState, newState, reason string) domain.Envelope {
	e := domain.NewEnvelope(t, eventSourceProxy)
	e.TaskID = taskID
	e.Metadata["category"] = "agent"
	e.Metadata["agent_id"] = agentID
	e.Metadata["prev_state"] = prevState
	e.Metadata["new_state"] = newState
	if reason != "" {
		e.Metadata["reason"] = reason
	}
	return e
}

func (s *TaskService) notifyLocked(n domain.Notification) {
	if s.notifications != nil {
		s.notifications.Add(n)
	}
	s.emitLocked(context.Background(), domain.Envelope{
		ID: newID(), Type: domain.EventNotificationCreated, Source: eventSourceProxy,
		Timestamp: time.Now().UnixMilli(),
		Payload:   map[string]any{"title": n.Title, "body": n.Body, "severity": n.Severity},
		Metadata:  map[string]string{"category": "notification", "taskId": n.TaskID},
	})
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("id-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
