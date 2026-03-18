package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"server/infra/persistence/localstore"
)

// pendingRun holds tasks waiting for proposal setup to complete.
type pendingRun struct {
	nodes       []treeNode
	nodeByID    map[string]treeNode
	maxAgents   int
	projectName string
	userID      string
}

type dispatched struct {
	NodeID string `json:"nodeId"`
	TaskID string `json:"taskId"`
	Status string `json:"status"`
}

// FrontierGenerator generates milestoneProposals and frontiers via LLM.
// Implemented by orchestration.Planner adapter in cmd/local/main.go.
type FrontierGenerator interface {
	// GenerateMilestoneProposal generates a detailed milestone spec via Claude.
	GenerateMilestoneProposal(ctx context.Context, proposal, milestoneSummary string, taskSummaries []string) (string, error)
	// GenerateTaskFrontiers generates frontiers for ALL tasks under a milestone in one call via Gemini.
	// Returns map[taskUID] → frontier string.
	GenerateTaskFrontiers(ctx context.Context, proposal, milestoneProposal string, tasks []TaskFrontierInput) (map[string]string, error)
}

// TaskFrontierInput is the input for batch frontier generation.
type TaskFrontierInput struct {
	UID     string
	Summary string
}

// Note: BuildSkeletonFromPlan, contextMdInstructions, and other shared logic
// are also available in server/internal/app/orchestration/ for Lambda handlers.

// RunHandler handles /run/* endpoints for dispatching tasks via the proxy WebSocket hub.
type RunHandler struct {
	hub       *ProxyHub
	store     *localstore.Store
	frontier  FrontierGenerator

	mu          sync.Mutex
	pendingRuns map[string]pendingRun
}

func NewRunHandler(hub *ProxyHub, store *localstore.Store, fg FrontierGenerator) *RunHandler {
	return &RunHandler{
		hub:         hub,
		store:       store,
		frontier:    fg,
		pendingRuns: make(map[string]pendingRun),
	}
}

func (h *RunHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /run/start", h.start)
	mux.HandleFunc("POST /run/add-agent", h.addAgent)
	mux.HandleFunc("POST /run/stop-agent", h.stopAgent)
	mux.HandleFunc("GET /run/agents", h.listAgents)
	mux.HandleFunc("GET /run/tasks", h.listTasks)
	mux.HandleFunc("GET /run/logs", h.getLogs)
	mux.HandleFunc("GET /run/resumable", h.listResumable)
	mux.HandleFunc("GET /run/progress", h.getProgress)
	mux.HandleFunc("GET /run/agents/{agentId}/logs", h.getAgentLogs)
	mux.HandleFunc("POST /run/pause-all", h.pauseAll)
	mux.HandleFunc("POST /run/resume-all", h.resumeAll)
	mux.HandleFunc("POST /run/strategy-meeting/start", h.startStrategyMeeting)
	mux.HandleFunc("GET /run/strategy-meeting/status", h.strategyMeetingStatus)
}

// treeNode mirrors the node shape sent from the frontend.
type treeNode struct {
	ID             string  `json:"id"`
	UID            string  `json:"uid"`
	Type           string  `json:"type"`
	Summary        string  `json:"summary"`
	Agent          string  `json:"agent"`
	Status         string  `json:"status"`
	Priority       string  `json:"priority"`
	ParentID       *string `json:"parentId"`
	BoundedContext string  `json:"boundedContext"`
}

type createTaskPayload struct {
	Name             string   `json:"name"`
	Prompt           string   `json:"prompt"`
	NumAgents        int      `json:"numAgents"`
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

func (h *RunHandler) start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nodes       []treeNode `json:"nodes"`
		MaxAgents   int        `json:"max_agents"`
		ProjectName string     `json:"project_name"`
		Plan        *planSpec  `json:"plan"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Nodes) == 0 {
		WriteError(w, http.StatusBadRequest, "nodes are required")
		return
	}

	maxAgents := req.MaxAgents
	if maxAgents <= 0 || maxAgents > 6 {
		maxAgents = 6
	}

	// Check proxy connection.
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy client is not connected")
		return
	}
	proxyUserID := h.hub.FirstUserID()

	// Build node lookup for branch resolution.
	nodeByID := make(map[string]treeNode, len(req.Nodes))
	for _, n := range req.Nodes {
		nodeByID[n.ID] = n
	}

	var results []dispatched
	var errors []string

	// Fork tree into tracks/{planId}/tree.json with initial "running" status.
	if h.store != nil && req.ProjectName != "" {
		trackNodes := make([]map[string]any, 0, len(req.Nodes))
		for _, n := range req.Nodes {
			trackNodes = append(trackNodes, map[string]any{
				"id": n.ID, "uid": n.UID, "type": n.Type,
				"summary": n.Summary, "agent": n.Agent,
				"status": "pending", "priority": n.Priority,
				"parentId": n.ParentID, "boundedContext": n.BoundedContext,
				"startedAt": time.Now().UnixMilli(),
			})
		}
		planId := fmt.Sprintf("%d", time.Now().UnixMilli())
		if len(req.Nodes) > 0 {
			// Use first node's UID-based hash or plan name as ID.
			planId = sanitizeTrackID(req.ProjectName)
		}
		track := map[string]any{"nodes": trackNodes, "projectName": req.ProjectName, "startedAt": time.Now().UnixMilli()}
		if err := h.store.Put("tracks/"+planId+"/tree.json", track); err != nil {
			log.Printf("[Run] Failed to fork tree to tracks: %v", err)
		} else {
			log.Printf("[Run] Forked tree to tracks/%s (%d nodes)", planId, len(trackNodes))
		}
	}

	// Phase 0: Dispatch proposal as setup task (initializes workspace skeleton).
	// Task nodes are NOT dispatched here — they wait for proposal completion.
	// When the setup agent finishes, the proxy sends task.completed event,
	// which triggers localEventRouter to dispatch the pending tasks.

	// Save pending tasks for later dispatch (after proposal completes).
	pendingKey := fmt.Sprintf("pending_run_%d", time.Now().UnixMilli())
	var pendingTasks []treeNode
	for _, n := range req.Nodes {
		if n.Type == "task" {
			pendingTasks = append(pendingTasks, n)
		}
	}
	if len(pendingTasks) > 0 {
		h.mu.Lock()
		h.pendingRuns[pendingKey] = pendingRun{
			nodes:       pendingTasks,
			nodeByID:    nodeByID,
			maxAgents:   maxAgents,
			projectName: req.ProjectName,
			userID:      proxyUserID,
		}
		h.mu.Unlock()
	}

	// Dispatch proposal: skeleton (proxy-side) or agent-based (fallback).
	for _, n := range req.Nodes {
		if n.Type != "proposal" {
			continue
		}

		// Try proxy-side skeleton creation.
		// 1. Use structuredSkeleton if available
		// 2. Otherwise, build from techSpec.folderStructure
		skeleton := buildSkeletonFromPlan(req.Plan, req.ProjectName)
		if skeleton != nil && len(skeleton["files"].([]map[string]string)) > 0 {
			h.setProgress("creating_skeleton", "Creating project structure...")
			skeletonPayload := skeleton
			_, err := h.hub.SendCommand(r.Context(), proxyUserID, ActionCreateSkeleton, skeletonPayload)
			if err != nil {
				log.Printf("[Run] Skeleton creation failed for %s: %v — falling back to agent", n.UID, err)
			} else {
				results = append(results, dispatched{NodeID: n.ID, TaskID: "skeleton", Status: "done"})
				log.Printf("[Run] Skeleton created by proxy for %s (pending tasks: %d)", n.UID, len(pendingTasks))
				// Generate histories (Claude: milestoneProposal + frontiers) → then dispatch.
				go h.prepareHistoriesAndDispatch(req.Plan, req.Nodes, req.ProjectName)
				break
			}
		}

		// Fallback: agent-based setup.
		setupPayload := createTaskPayload{
			Name:         n.Summary,
			Prompt:       buildSetupPrompt(n.Summary, req.Nodes, req.Plan),
			NumAgents:    1,
			Tags:         []string{n.UID, "setup", pendingKey},
			AgentType:    mapAgent(n.Agent),
			Mode:         "execute",
			NodeType:     "proposal",
			NodeUID:      n.UID,
			ParentBranch: "main",
			ProjectName:  req.ProjectName,
		}

		respBytes, err := h.hub.SendCommand(r.Context(), proxyUserID, ActionCreateTask, setupPayload)
		if err != nil {
			log.Printf("[Run] Setup task failed for %s: %v", n.UID, err)
			errors = append(errors, n.UID+": "+err.Error())
		} else {
			var resp struct {
				Task struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"task"`
			}
			if err := json.Unmarshal(respBytes, &resp); err == nil {
				results = append(results, dispatched{NodeID: n.ID, TaskID: resp.Task.ID, Status: resp.Task.Status})
			}
			log.Printf("[Run] Setup task dispatched: %s (pending tasks: %d, key: %s)", n.UID, len(pendingTasks), pendingKey)
		}
		break
	}

	// If no proposal node, dispatch tasks directly.
	hasProposal := false
	for _, n := range req.Nodes {
		if n.Type == "proposal" {
			hasProposal = true
			break
		}
	}

	if !hasProposal {
		h.dispatchTasks(r.Context(), proxyUserID, req.Nodes, nodeByID, maxAgents, req.ProjectName, &results, &errors)
	}

	WriteOK(w, map[string]any{
		"dispatched": results,
		"errors":     errors,
		"total":      len(results),
	})
}

// dispatchTasks sends task nodes to the proxy as createTask commands.
func (h *RunHandler) dispatchTasks(ctx context.Context, userID string, nodes []treeNode, nodeByID map[string]treeNode, maxAgents int, projectName string, results *[]dispatched, errors *[]string) {
	dispatchCount := 0
	for _, n := range nodes {
		if n.Type != "task" {
			continue
		}
		if dispatchCount >= maxAgents {
			log.Printf("[Run] Agent limit reached (%d), skipping task %s", maxAgents, n.UID)
			break
		}

		parentBranch := "main"
		if n.ParentID != nil {
			if parent, ok := nodeByID[*n.ParentID]; ok {
				parentBranch = nodeBranchName(parent)
			}
		}

		// Load frontier from task history if available.
		taskPrompt := n.Summary
		if h.store != nil {
			trackID := sanitizeTrackID(projectName)
			var history map[string]any
			if err := h.store.Get("tracks/"+trackID+"/histories/tasks/"+n.UID+".json", &history); err == nil {
				if f, ok := history["frontier"].(string); ok && f != "" {
					taskPrompt = f
				}
			}
		}
		taskPrompt += contextMdInstructions

		payload := createTaskPayload{
			Name:             n.Summary,
			Prompt:           taskPrompt,
			NumAgents:        1,
			Tags:             []string{n.UID, n.Priority},
			BoundedContextID: n.BoundedContext,
			ParentTaskID:     stringVal(n.ParentID),
			AgentType:        mapAgent(n.Agent),
			Mode:             "execute",
			NodeType:         n.Type,
			NodeUID:          n.UID,
			ParentBranch:     parentBranch,
			ProjectName:      projectName,
		}

		respBytes, err := h.hub.SendCommand(ctx, userID, ActionCreateTask, payload)
		if err != nil {
			log.Printf("[Run] Failed to dispatch task %s: %v", n.UID, err)
			*errors = append(*errors, n.UID+": "+err.Error())
			continue
		}

		var resp struct {
			Task struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"task"`
		}
		if err := json.Unmarshal(respBytes, &resp); err != nil {
			*errors = append(*errors, n.UID+": decode error")
			continue
		}

		*results = append(*results, dispatched{NodeID: n.ID, TaskID: resp.Task.ID, Status: resp.Task.Status})
		dispatchCount++
		log.Printf("[Run] Dispatched task %s → proxy task %s (%d/%d)", n.UID, resp.Task.ID, dispatchCount, maxAgents)

		// Mark task as running in tree.json so DispatchNextPendingTask won't re-dispatch it.
		if h.store != nil {
			h.updateTrackNodeStatus(projectName, n.UID, "running")
		}

		// Stagger agent starts to avoid Cursor CLI config file race condition.
		if dispatchCount < maxAgents {
			time.Sleep(3 * time.Second)
		}

		// Update parent milestone status to "running" when first child task dispatched.
		if h.store != nil && n.ParentID != nil {
			if parent, ok := nodeByID[*n.ParentID]; ok && parent.Type == "milestone" {
				h.updateTrackNodeStatus(projectName, parent.UID, "running")
			}
		}

		// SSOT: record this node as occupied.
		if h.store != nil {
			h.addOccupiedNode(projectName, n.UID, resp.Task.ID)
		}
	}
}

// DispatchPending is called by localEventRouter when a proposal setup task completes.
// It dispatches the pending task nodes that were waiting for the workspace to be ready.
func (h *RunHandler) DispatchPending(pendingKey string) {
	h.mu.Lock()
	pending, ok := h.pendingRuns[pendingKey]
	if ok {
		delete(h.pendingRuns, pendingKey)
	}
	h.mu.Unlock()

	if !ok {
		return
	}

	log.Printf("[Run] Proposal complete, dispatching %d pending tasks (key: %s)", len(pending.nodes), pendingKey)

	var results []dispatched
	var errors []string
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h.dispatchTasks(ctx, pending.userID, pending.nodes, pending.nodeByID, pending.maxAgents, pending.projectName, &results, &errors)

	log.Printf("[Run] Pending dispatch complete: %d dispatched, %d errors", len(results), len(errors))
}

// addAgent adds a single agent to the highest-priority unassigned task.
// Queries the proxy for current tasks, finds pending ones, picks the most important.
func (h *RunHandler) addAgent(w http.ResponseWriter, r *http.Request) {
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy client is not connected")
		return
	}
	proxyUserID := h.hub.FirstUserID()

	// 1. Get current tasks from proxy.
	tasksBytes, err := h.hub.SendCommand(r.Context(), proxyUserID, ActionListTasks, struct{}{})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list tasks: "+err.Error())
		return
	}

	var tasks []struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Status       string   `json:"status"`
		NodeUID      string   `json:"nodeUid"`
		NodeType     string   `json:"nodeType"`
		AgentIDs     []string `json:"agentIds"`
		Tags         []string `json:"tags"`
		ParentBranch string   `json:"parentBranch"`
		BranchName   string   `json:"branchName"`
	}
	if err := json.Unmarshal(tasksBytes, &tasks); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to parse tasks")
		return
	}

	// 2. Also check tracks for unstarted task nodes.
	type candidate struct {
		Name         string
		NodeUID      string
		NodeType     string
		Priority     string
		TaskID       string
		ParentBranch string
	}
	var bestTask *candidate

	// Find tasks with no agents (pending) — priority: high > medium > low.
	priorityOrder := map[string]int{"high": 3, "medium": 2, "low": 1, "": 0}

	for _, t := range tasks {
		if len(t.AgentIDs) == 0 && (t.Status == "pending" || t.Status == "running") {
			pri := ""
			for _, tag := range t.Tags {
				if tag == "high" || tag == "medium" || tag == "low" {
					pri = tag
				}
			}
			if bestTask == nil || priorityOrder[pri] > priorityOrder[bestTask.Priority] {
				bestTask = &candidate{t.Name, t.NodeUID, t.NodeType, pri, t.ID, t.ParentBranch}
			}
		}
	}

	// 3. If no unassigned tasks, scale up an existing running task.
	if bestTask == nil {
		for _, t := range tasks {
			if t.Status == "running" && t.NodeType == "task" {
				// Scale this task: add 1 agent.
				_, err := h.hub.SendCommand(r.Context(), proxyUserID, ActionScaleTask, map[string]any{
					"taskID": t.ID, "numAgents": len(t.AgentIDs) + 1,
				})
				if err != nil {
					WriteError(w, http.StatusInternalServerError, "scale failed: "+err.Error())
					return
				}
				WriteOK(w, map[string]any{
					"action": "scaled",
					"taskId": t.ID,
					"name":   t.Name,
					"agents": len(t.AgentIDs) + 1,
				})
				return
			}
		}
		WriteError(w, http.StatusNotFound, "no tasks available for new agent")
		return
	}

	// 4. Find project name from tracks.
	projectName := ""
	if h.store != nil {
		trackKeys, _ := h.store.List("tracks")
		for _, k := range trackKeys {
			if !strings.HasSuffix(k, "/tree.json") {
				continue
			}
			var track map[string]any
			if err := h.store.Get(k, &track); err == nil {
				if pn, ok := track["projectName"].(string); ok {
					projectName = pn
					break
				}
			}
		}
	}

	// 5. Create a new task with 1 agent for the best candidate.
	parentBranch := bestTask.ParentBranch
	if parentBranch == "" {
		parentBranch = "main"
	}

	payload := createTaskPayload{
		Name:         bestTask.Name,
		Prompt:       bestTask.Name,
		NumAgents:    1,
		Tags:         []string{bestTask.NodeUID, bestTask.Priority},
		AgentType:    "cursor",
		Mode:         "execute",
		NodeType:     bestTask.NodeType,
		NodeUID:      bestTask.NodeUID,
		ParentBranch: parentBranch,
		ProjectName:  projectName,
	}

	respBytes, err := h.hub.SendCommand(r.Context(), proxyUserID, ActionCreateTask, payload)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "create task failed: "+err.Error())
		return
	}

	var resp struct {
		Task struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"task"`
	}
	_ = json.Unmarshal(respBytes, &resp)

	WriteOK(w, map[string]any{
		"action": "created",
		"taskId": resp.Task.ID,
		"name":   bestTask.Name,
		"nodeUid": bestTask.NodeUID,
	})
	log.Printf("[Run] Added agent to task %s (%s)", bestTask.NodeUID, bestTask.Name)
}

// stopAgent kills an agent immediately via the proxy.
func (h *RunHandler) stopAgent(w http.ResponseWriter, r *http.Request) {
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy not connected")
		return
	}

	var req struct {
		AgentID string `json:"agentId"`
	}
	if err := DecodeJSON(r, &req); err != nil || req.AgentID == "" {
		WriteError(w, http.StatusBadRequest, "agentId is required")
		return
	}

	proxyUserID := h.hub.FirstUserID()
	_, err := h.hub.SendCommand(r.Context(), proxyUserID, ActionStopAgent, map[string]string{
		"agentID": req.AgentID,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "stop agent failed: "+err.Error())
		return
	}

	WriteOK(w, map[string]string{"stopped": req.AgentID})
}

// DispatchAllPending dispatches all pending task runs (called after proposal setup completes).
func (h *RunHandler) DispatchAllPending() {
	h.mu.Lock()
	pending := make(map[string]pendingRun, len(h.pendingRuns))
	for k, v := range h.pendingRuns {
		pending[k] = v
	}
	h.pendingRuns = make(map[string]pendingRun)
	h.mu.Unlock()

	log.Printf("[Run] DispatchAllPending: %d pending runs to dispatch", len(pending))
	if len(pending) == 0 {
		log.Printf("[Run] WARNING: No pending runs found — tasks may have been dispatched already or never saved")
	}

	for key, p := range pending {
		log.Printf("[Run] Dispatching pending run %s (%d tasks, maxAgents=%d, project=%s)", key, len(p.nodes), p.maxAgents, p.projectName)
		var results []dispatched
		var errors []string
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		h.dispatchTasks(ctx, p.userID, p.nodes, p.nodeByID, p.maxAgents, p.projectName, &results, &errors)
		cancel()
		log.Printf("[Run] Pending %s done: %d dispatched, %d errors, errors=%v", key, len(results), len(errors), errors)
	}
}

// DispatchNextPendingTask finds the next unstarted task in tracks and dispatches it.
// Called when an agent completes a task — the freed agent slot picks up the next job.
func (h *RunHandler) DispatchNextPendingTask(projectName string) {
	log.Printf("[Run] DispatchNextPendingTask called: projectName=%q", projectName)
	if h.store == nil || !h.hub.AnyConnected() {
		log.Printf("[Run] DispatchNextPendingTask: skip (store=%v connected=%v)", h.store != nil, h.hub.AnyConnected())
		return
	}
	userID := h.hub.FirstUserID()
	trackID := sanitizeTrackID(projectName)
	key := "tracks/" + trackID + "/tree.json"

	var track map[string]any
	if err := h.store.Get(key, &track); err != nil {
		return
	}
	nodes, ok := track["nodes"].([]any)
	if !ok {
		return
	}

	// Build nodeByID for parent branch resolution.
	nodeByID := make(map[string]treeNode)
	for _, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			continue
		}
		uid, _ := node["uid"].(string)
		id, _ := node["id"].(string)
		nType, _ := node["type"].(string)
		summary, _ := node["summary"].(string)
		priority, _ := node["priority"].(string)
		bc, _ := node["boundedContext"].(string)
		var parentID *string
		if pid, ok := node["parentId"].(string); ok && pid != "" {
			parentID = &pid
		}
		nodeByID[id] = treeNode{
			ID: id, UID: uid, Type: nType, Summary: summary,
			Priority: priority, ParentID: parentID, BoundedContext: bc,
		}
	}

	// Try to dispatch any pending reviews (one at a time, review has priority).
	// Each milestone gets its own pending review file: pending_reviews/{milestoneUID}.json
	pendingKeys, _ := h.store.List("tracks/" + trackID + "/pending_reviews")
	reviewDispatched := false
	for _, pk := range pendingKeys {
		var pendingReview map[string]any
		if err := h.store.Get(pk, &pendingReview); err != nil {
			continue
		}
		nodeUID, _ := pendingReview["nodeUID"].(string)
		reviewPrompt, _ := pendingReview["prompt"].(string)
		reviewBranch, _ := pendingReview["reviewBranch"].(string)
		if nodeUID == "" || reviewPrompt == "" {
			continue
		}
		if err := h.DispatchReviewTask(reviewBranch, nodeUID, projectName, reviewPrompt); err != nil {
			log.Printf("[Run] Pending review retry failed for %s: %v — will try next time", nodeUID, err)
			continue // try next pending review, don't block everything
		}
		// Review dispatched — remove this pending file.
		h.store.Delete(pk)
		log.Printf("[Run] Pending review dispatched for %s", nodeUID)
		reviewDispatched = true
		break // one review per dispatch cycle
	}
	if reviewDispatched {
		return // review got the agent slot
	}
	// If no pending reviews could be dispatched, proceed to dispatch next task.

	// Load SSOT occupied nodes to avoid dispatching to already-occupied slots.
	var occupied []map[string]any
	_ = h.store.Get("tracks/"+trackID+"/ssot/occupied.json", &occupied)
	occupiedUIDs := make(map[string]bool, len(occupied))
	for _, entry := range occupied {
		if uid, _ := entry["nodeUid"].(string); uid != "" {
			occupiedUIDs[uid] = true
		}
	}

	// Build set of milestones currently in "reviewing" state — skip their tasks.
	reviewingMilestones := make(map[string]bool)
	for _, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			continue
		}
		nType, _ := node["type"].(string)
		nStatus, _ := node["status"].(string)
		if nType == "milestone" && nStatus == "reviewing" {
			nID, _ := node["id"].(string)
			reviewingMilestones[nID] = true
		}
	}

	// Find first "pending" task node that is NOT occupied and NOT under a reviewing milestone.
	for _, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			continue
		}
		nType, _ := node["type"].(string)
		status, _ := node["status"].(string)
		if nType != "task" || status != "pending" {
			continue
		}

		// Skip tasks under a milestone that is currently in review.
		if pid, ok := node["parentId"].(string); ok && reviewingMilestones[pid] {
			continue
		}

		uid, _ := node["uid"].(string)

		// Skip if already occupied (SSOT check).
		if occupiedUIDs[uid] {
			continue
		}

		summary, _ := node["summary"].(string)
		priority, _ := node["priority"].(string)
		bc, _ := node["boundedContext"].(string)

		parentBranch := "main"
		if pid, ok := node["parentId"].(string); ok && pid != "" {
			if parent, ok := nodeByID[pid]; ok {
				parentBranch = nodeBranchName(parent)
			}
		}

		// Load frontier from history.
		taskPrompt := summary
		var history map[string]any
		if err := h.store.Get("tracks/"+trackID+"/histories/tasks/"+uid+".json", &history); err == nil {
			if f, ok := history["frontier"].(string); ok && f != "" {
				taskPrompt = f
			}
		}
		taskPrompt += contextMdInstructions

		payload := createTaskPayload{
			Name:             summary,
			Prompt:           taskPrompt,
			NumAgents:        1,
			Tags:             []string{uid, priority},
			BoundedContextID: bc,
			AgentType:        "cursor",
			Mode:             "execute",
			NodeType:         "task",
			NodeUID:          uid,
			ParentBranch:     parentBranch,
			ProjectName:      projectName,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := h.hub.SendCommand(ctx, userID, ActionCreateTask, payload)
		cancel()
		if err != nil {
			log.Printf("[Run] Failed to dispatch next task %s: %v", uid, err)
			return
		}

		// Update status + SSOT.
		h.updateTrackNodeStatus(projectName, uid, "running")
		h.addOccupiedNode(projectName, uid, "next-pending")
		log.Printf("[Run] Dispatched next pending task: %s (%s)", uid, summary)

		// Update parent milestone to running if needed.
		if pid, ok := node["parentId"].(string); ok {
			if parent, ok := nodeByID[pid]; ok && parent.Type == "milestone" {
				h.updateTrackNodeStatus(projectName, parent.UID, "running")
			}
		}
		return // dispatch one at a time
	}
	log.Printf("[Run] No more pending tasks for %s", projectName)
}

func (h *RunHandler) listAgents(w http.ResponseWriter, r *http.Request) {
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy not connected")
		return
	}
	proxyUserID := h.hub.FirstUserID()

	respBytes, err := h.hub.SendCommand(r.Context(), proxyUserID, ActionListAgents, struct{}{})
	if err != nil {
		log.Printf("[Run] Failed to list agents: %v", err)
		WriteError(w, http.StatusServiceUnavailable, "proxy command failed")
		return
	}

	var agents json.RawMessage = respBytes
	WriteOK(w, map[string]any{"agents": agents})
}

func (h *RunHandler) listTasks(w http.ResponseWriter, r *http.Request) {
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy not connected")
		return
	}
	proxyUserID := h.hub.FirstUserID()

	respBytes, err := h.hub.SendCommand(r.Context(), proxyUserID, ActionListTasks, struct{}{})
	if err != nil {
		log.Printf("[Run] Failed to list tasks: %v", err)
		WriteError(w, http.StatusServiceUnavailable, "proxy command failed")
		return
	}

	var tasks json.RawMessage = respBytes
	WriteOK(w, map[string]any{"tasks": tasks})
}

// getLogs returns MAAL logs from the proxy (all recent logs across all tasks).
func (h *RunHandler) getLogs(w http.ResponseWriter, r *http.Request) {
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy not connected")
		return
	}
	proxyUserID := h.hub.FirstUserID()

	respBytes, err := h.hub.SendCommand(r.Context(), proxyUserID, ActionGetLogs, struct{}{})
	if err != nil {
		WriteError(w, http.StatusServiceUnavailable, "proxy command failed")
		return
	}

	var logs json.RawMessage = respBytes
	WriteOK(w, map[string]any{"logs": logs})
}

// listResumable returns plans that have an existing workspace (can be resumed).
// Checks tracks/ and ~/.samams/workspaces/ for existing project folders.
func (h *RunHandler) listResumable(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		WriteOK(w, map[string]any{"resumable": []any{}})
		return
	}

	var resumable []map[string]any

	trackKeys, _ := h.store.List("tracks")
	for _, k := range trackKeys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		var track map[string]any
		if err := h.store.Get(k, &track); err != nil {
			continue
		}
		projectName, _ := track["projectName"].(string)
		if projectName == "" {
			continue
		}

		// Check node progress.
		nodes, _ := track["nodes"].([]any)
		total, completed, running := 0, 0, 0
		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			nType, _ := node["type"].(string)
			if nType != "task" {
				continue
			}
			total++
			switch s, _ := node["status"].(string); s {
			case "complete":
				completed++
			case "running", "active":
				running++
			}
		}

		// Only show as resumable if not fully complete and has been started.
		if total > 0 && completed < total {
			trackDir := strings.TrimSuffix(k, "/tree.json")
			startedAt, _ := track["startedAt"].(float64)
			resumable = append(resumable, map[string]any{
				"projectName": projectName,
				"trackId":     strings.TrimPrefix(trackDir, "tracks/"),
				"totalTasks":  total,
				"completed":   completed,
				"running":     running,
				"pending":     total - completed - running,
				"startedAt":   int64(startedAt),
			})
		}
	}

	if resumable == nil {
		resumable = []map[string]any{}
	}
	WriteOK(w, map[string]any{"resumable": resumable})
}

// getProgress returns the current preparation progress (skeleton, history generation, etc).
func (h *RunHandler) getProgress(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		WriteOK(w, map[string]any{"stage": "idle", "message": ""})
		return
	}
	var progress map[string]any
	if err := h.store.Get("progress/status.json", &progress); err != nil {
		WriteOK(w, map[string]any{"stage": "idle", "message": ""})
		return
	}
	WriteOK(w, progress)
}

// setProgress saves the current preparation stage to store.
func (h *RunHandler) setProgress(stage, message string) {
	if h.store == nil {
		return
	}
	_ = h.store.Put("progress/status.json", map[string]any{
		"stage":   stage,
		"message": message,
		"ts":      time.Now().UnixMilli(),
	})
}

// getAgentLogs returns CLI stdout logs for a specific agent.
func (h *RunHandler) getAgentLogs(w http.ResponseWriter, r *http.Request) {
	agentId := r.PathValue("agentId")
	if agentId == "" {
		WriteError(w, http.StatusBadRequest, "agentId is required")
		return
	}
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy not connected")
		return
	}
	userID := h.hub.FirstUserID()

	respBytes, err := h.hub.SendCommand(r.Context(), userID, ActionGetAgentLogs, map[string]string{
		"agentID": agentId,
	})
	if err != nil {
		WriteError(w, http.StatusNotFound, "agent not found or logs unavailable")
		return
	}

	var result json.RawMessage = respBytes
	WriteOK(w, result)
}

func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// planSpec holds the planning document's technical specifications.
type planSpec struct {
	Title               string         `json:"title"`
	Goal                string         `json:"goal"`
	Description         string         `json:"description"`
	TechSpec            map[string]any `json:"techSpec"`
	AbstractSpec        map[string]any `json:"abstractSpec"`
	StructuredSkeleton  *skeletonSpec  `json:"structuredSkeleton,omitempty"`
}

// skeletonSpec is the structured JSON format for deterministic skeleton creation.
type skeletonSpec struct {
	Module struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"module"`
	Files []struct {
		Path    string `json:"path"`
		Purpose string `json:"purpose"`
	} `json:"files"`
}

// buildSetupPrompt creates a prompt for the setup agent that reads the proposal
// and initializes the project structure based on the plan's techSpec.
func buildSetupPrompt(proposalSummary string, nodes []treeNode, plan *planSpec) string {
	var sb strings.Builder
	sb.WriteString("You are setting up a new project workspace. ")
	sb.WriteString("Read the following project specification and create the initial project structure.\n\n")

	sb.WriteString("## Project Overview\n")
	if plan != nil && plan.Goal != "" {
		sb.WriteString("**Goal:** " + plan.Goal + "\n\n")
	}
	if plan != nil && plan.Description != "" {
		sb.WriteString("**Description:**\n" + plan.Description + "\n\n")
	} else {
		sb.WriteString(proposalSummary + "\n\n")
	}

	// Tech Spec — folder structure, architecture, tech stack, etc.
	if plan != nil && len(plan.TechSpec) > 0 {
		sb.WriteString("## Technical Specification\n")
		for key, val := range plan.TechSpec {
			if s, ok := val.(string); ok && s != "" {
				sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", key, s))
			}
		}
	}

	// Abstract Spec — domain model, aggregates, events, workflows.
	if plan != nil && len(plan.AbstractSpec) > 0 {
		sb.WriteString("## Domain Specification\n")
		for key, val := range plan.AbstractSpec {
			if s, ok := val.(string); ok && s != "" {
				sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", key, s))
			}
		}
	}

	sb.WriteString("## Task Tree Overview\n")
	for _, n := range nodes {
		indent := ""
		switch n.Type {
		case "milestone":
			indent = "  "
		case "task":
			indent = "    "
		}
		sb.WriteString(fmt.Sprintf("%s- [%s] %s (%s)\n", indent, n.Type, n.Summary, n.UID))
	}

	sb.WriteString("\n## CRITICAL RULES\n")
	sb.WriteString("You MUST follow these rules EXACTLY. Violation will cause the entire pipeline to fail.\n\n")
	sb.WriteString("### FORBIDDEN — Do NOT do any of these:\n")
	sb.WriteString("- Do NOT write any code (no functions, no logic, no implementations)\n")
	sb.WriteString("- Do NOT write any import statements\n")
	sb.WriteString("- Do NOT create test files\n")
	sb.WriteString("- Do NOT install dependencies (no npm install, no go get)\n")
	sb.WriteString("- Do NOT run any build or compile commands\n\n")
	sb.WriteString("### REQUIRED — Do ONLY these:\n")
	sb.WriteString("1. Create ONLY the folder structure (mkdir) as specified in the Technical Specification.\n")
	sb.WriteString("2. Create ONLY empty files as placeholders (touch). Each file should contain ONLY a single-line comment describing its purpose.\n")
	sb.WriteString("   - Go: `// Package {name} handles {purpose}`\n")
	sb.WriteString("   - JS/TS: `// {purpose}`\n")
	sb.WriteString("   - Python: `# {purpose}`\n")
	sb.WriteString("3. Create ONE README.md with project overview, goal, and folder structure description.\n")
	sb.WriteString("4. Create ONE go.mod or package.json with ONLY the module name (no dependencies).\n")
	sb.WriteString("5. git add . && git commit -m 'initial project skeleton'\n\n")
	sb.WriteString("### WHY:\n")
	sb.WriteString("Other agents will fork this skeleton and implement features in isolated branches.\n")
	sb.WriteString("If you write code here, it will cause merge conflicts across all agents.\n\n")
	sb.WriteString("### WHEN DONE:\n")
	sb.WriteString("After completing the folder structure and committing, you are FINISHED.\n")
	sb.WriteString("Do NOT continue working. Do NOT implement anything else.\n")
	sb.WriteString("Simply stop. Your job is done. Other agents will take over from here.\n")
	return sb.String()
}

func sanitizeTrackID(name string) string {
	var b strings.Builder
	prev := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prev = false
		} else if !prev && b.Len() > 0 {
			b.WriteByte('-')
			prev = true
		}
	}
	result := strings.TrimRight(b.String(), "-")
	if len(result) > 50 {
		result = result[:50]
	}
	return strings.ToLower(result)
}

// nodeBranchName returns the git branch name for a tree node.
// proposal → "" (uses main), milestone → dev/{uid}, task → dev|fix|hotfix/{uid}
func nodeBranchName(n treeNode) string {
	switch n.Type {
	case "proposal":
		return "main"
	case "milestone":
		return "dev/" + n.UID
	case "task":
		lower := strings.ToLower(n.Summary)
		switch {
		case strings.Contains(lower, "hotfix") || strings.Contains(lower, "critical fix"):
			return "hotfix/" + n.UID
		case strings.Contains(lower, "bug") || strings.Contains(lower, "fix") || strings.Contains(lower, "patch"):
			return "fix/" + n.UID
		default:
			return "dev/" + n.UID
		}
	default:
		return "dev/" + n.UID
	}
}

// DispatchReviewTask sends a code review agent to the milestone branch.
// reviewBranch is the milestone UID (for milestone reviews) or "main" (for proposal reviews).
func (h *RunHandler) DispatchReviewTask(reviewBranch, nodeUID, projectName, reviewPrompt string) error {
	if !h.hub.AnyConnected() {
		return fmt.Errorf("proxy not connected")
	}
	userID := h.hub.FirstUserID()

	// Proposal review: ParentBranch = "main", milestone review: ParentBranch = "dev/{UID}"
	parentBranch := "dev/" + reviewBranch
	if reviewBranch == "main" {
		parentBranch = "main"
	}

	payload := createTaskPayload{
		Name:         "Code Review: " + reviewBranch,
		Prompt:       reviewPrompt,
		NumAgents:    1,
		Tags:         []string{reviewBranch, "review"},
		AgentType:    "cursor",
		Mode:         "review",
		NodeType:     "review",
		NodeUID:      nodeUID,
		ParentBranch: parentBranch,
		ProjectName:  projectName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := h.hub.SendCommand(ctx, userID, ActionCreateReviewTask, payload)
	if err != nil {
		return fmt.Errorf("dispatch review task: %w", err)
	}
	log.Printf("[Run] Review agent dispatched for %s (parentBranch=%s)", reviewBranch, parentBranch)
	return nil
}

// MergeMilestoneToMain sends a merge command to the proxy: milestone branch → main.
func (h *RunHandler) MergeMilestoneToMain(milestoneUID, projectName string) error {
	if !h.hub.AnyConnected() {
		return fmt.Errorf("proxy not connected")
	}
	userID := h.hub.FirstUserID()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := h.hub.SendCommand(ctx, userID, ActionMergeMilestone, map[string]string{
		"branchName":   "dev/" + milestoneUID,
		"targetBranch": "main",
	})
	if err != nil {
		return fmt.Errorf("merge milestone: %w", err)
	}
	log.Printf("[Run] Milestone %s merge → main command sent", milestoneUID)
	return nil
}

// DispatchReviewTasks generates frontiers for review-created tasks, saves histories, then dispatches.
// Same rule as initial dispatch: frontier 없는 task에는 절대 agent를 할당하지 않음.
func (h *RunHandler) DispatchReviewTasks(projectName, milestoneUID string, newTasks any) {
	if !h.hub.AnyConnected() {
		log.Printf("[Run] Cannot dispatch review tasks — proxy not connected")
		return
	}
	userID := h.hub.FirstUserID()

	tasks, ok := newTasks.([]struct {
		Summary        string `json:"summary"`
		Detail         string `json:"detail"`
		ParentUID      string `json:"parentUid"`
		Relationship   string `json:"relationship"`
		Priority       string `json:"priority"`
		Reason         string `json:"reason"`
		BoundedContext string `json:"boundedContext"`
	})
	if !ok {
		log.Printf("[Run] DispatchReviewTasks: invalid task type, skipping dispatch")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	trackID := sanitizeTrackID(projectName)

	// Load milestoneProposal from history for frontier generation.
	milestoneProposal := ""
	proposalText := ""
	if h.store != nil {
		var mh map[string]any
		if err := h.store.Get("tracks/"+trackID+"/histories/milestones/"+milestoneUID+".json", &mh); err == nil {
			milestoneProposal, _ = mh["milestoneProposal"].(string)
			proposalText, _ = mh["proposal"].(string)
		}
	}

	// Pre-generate UIDs and batch frontiers via Gemini.
	type reviewTask struct {
		uid string
		t   struct {
			Summary        string
			Detail         string
			ParentUID      string
			Relationship   string
			Priority       string
			Reason         string
			BoundedContext string
		}
	}
	var reviewTasks []reviewTask
	var frontierInputs []TaskFrontierInput
	for _, t := range tasks {
		uid := fmt.Sprintf("RVTASK-%s-%d", milestoneUID, time.Now().UnixNano()%10000)
		reviewTasks = append(reviewTasks, reviewTask{uid: uid, t: struct {
			Summary        string
			Detail         string
			ParentUID      string
			Relationship   string
			Priority       string
			Reason         string
			BoundedContext string
		}{t.Summary, t.Detail, t.ParentUID, t.Relationship, t.Priority, t.Reason, t.BoundedContext}})
		frontierInputs = append(frontierInputs, TaskFrontierInput{UID: uid, Summary: t.Summary})
	}

	// Batch frontier generation via Gemini.
	allFrontiers := make(map[string]string)
	if h.frontier != nil && len(frontierInputs) > 0 {
		result, err := h.frontier.GenerateTaskFrontiers(ctx, proposalText, milestoneProposal, frontierInputs)
		if err != nil {
			log.Printf("[Run] Batch frontier for review tasks failed: %v (using summaries)", err)
		} else {
			allFrontiers = result
		}
	}

	for _, rt := range reviewTasks {
		uid := rt.uid
		t := rt.t

		frontier := t.Summary
		if t.Detail != "" {
			frontier = t.Detail
		}
		if f, ok := allFrontiers[uid]; ok && f != "" {
			frontier = f
		}

		// Save task history (self-contained).
		if h.store != nil {
			// For review-generated tasks that are children of existing tasks, include parent summary.
			var summaries []string
			if t.Relationship == "child" && t.ParentUID != "" {
				var parentHistory map[string]any
				if err := h.store.Get("tracks/"+trackID+"/histories/tasks/"+t.ParentUID+".json", &parentHistory); err == nil {
					// Parent task has a frontier which serves as its "summary" of what it did.
					if ps, ok := parentHistory["frontier"].(string); ok {
						summaries = append(summaries, ps)
					}
				}
			}

			taskHistory := map[string]any{
				"proposal":          proposalText,
				"milestoneProposal": milestoneProposal,
				"frontier":          frontier,
				"summaries":         summaries,
				"uid":               uid,
				"milestoneUid":      milestoneUID,
				"origin":            "review",
			}
			_ = h.store.Put("tracks/"+trackID+"/histories/tasks/"+uid+".json", taskHistory)
			log.Printf("[Run] Saved review task history for %s (frontier: %d chars)", uid, len(frontier))
		}

		// Dispatch with frontier as prompt.
		payload := createTaskPayload{
			Name:             t.Summary,
			Prompt:           frontier + contextMdInstructions,
			NumAgents:        1,
			Tags:             []string{uid, t.Priority, "review"},
			BoundedContextID: t.BoundedContext,
			AgentType:        "cursor",
			Mode:             "execute",
			NodeType:         "task",
			NodeUID:          uid,
			ParentBranch:     "dev/" + milestoneUID,
			ProjectName:      projectName,
		}

		if _, err := h.hub.SendCommand(ctx, userID, ActionCreateTask, payload); err != nil {
			log.Printf("[Run] Failed to dispatch review task %s: %v", uid, err)
		} else {
			log.Printf("[Run] Dispatched review task %s for milestone %s", uid, milestoneUID)
		}

		// Stagger to avoid Cursor CLI config race.
		time.Sleep(3 * time.Second)
	}
}

// updateTrackNodeStatus updates a node's status in the tracked tree by UID.
func (h *RunHandler) updateTrackNodeStatus(projectName, nodeUID, status string) {
	trackID := sanitizeTrackID(projectName)
	key := "tracks/" + trackID + "/tree.json"

	var track map[string]any
	if err := h.store.Get(key, &track); err != nil {
		return
	}
	nodes, ok := track["nodes"].([]any)
	if !ok {
		return
	}
	for i, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			continue
		}
		if uid, _ := node["uid"].(string); uid == nodeUID {
			node["status"] = status
			node["updatedAt"] = time.Now().UnixMilli()
			nodes[i] = node
			_ = h.store.Put(key, track)
			log.Printf("[Run] Updated %s status → %s", nodeUID, status)
			return
		}
	}
}

// addOccupiedNode records a node as occupied in the SSOT file.
// tracks/{planId}/ssot/occupied.json — single source of truth for which nodes are assigned.
func (h *RunHandler) addOccupiedNode(projectName, nodeUID, taskID string) {
	trackID := sanitizeTrackID(projectName)
	key := "tracks/" + trackID + "/ssot/occupied.json"

	var occupied []map[string]any
	_ = h.store.Get(key, &occupied) // may not exist yet

	// Check if already tracked.
	for _, entry := range occupied {
		if uid, _ := entry["nodeUid"].(string); uid == nodeUID {
			return
		}
	}

	occupied = append(occupied, map[string]any{
		"nodeUid":      nodeUID,
		"taskId":       taskID,
		"status":       "running",
		"dispatchedAt": time.Now().UnixMilli(),
	})
	_ = h.store.Put(key, occupied)
	log.Printf("[SSOT] Node %s occupied (task %s)", nodeUID, taskID)
}

// RemoveOccupiedNode removes a node from the SSOT occupied list.
func (h *RunHandler) RemoveOccupiedNode(projectName, nodeUID string) {
	trackID := sanitizeTrackID(projectName)
	key := "tracks/" + trackID + "/ssot/occupied.json"

	var occupied []map[string]any
	if err := h.store.Get(key, &occupied); err != nil {
		return
	}

	filtered := make([]map[string]any, 0, len(occupied))
	for _, entry := range occupied {
		if uid, _ := entry["nodeUid"].(string); uid != nodeUID {
			filtered = append(filtered, entry)
		}
	}
	_ = h.store.Put(key, filtered)
	log.Printf("[SSOT] Node %s released", nodeUID)
}

// buildSkeletonFromPlan constructs a skeleton spec from the plan's techSpec.
// Uses structuredSkeleton if available, otherwise parses folderStructure text.
func buildSkeletonFromPlan(plan *planSpec, projectName string) map[string]any {
	if plan == nil {
		return nil
	}

	// 1. Use structuredSkeleton if present.
	if plan.StructuredSkeleton != nil && len(plan.StructuredSkeleton.Files) > 0 {
		files := make([]map[string]string, 0, len(plan.StructuredSkeleton.Files))
		for _, f := range plan.StructuredSkeleton.Files {
			files = append(files, map[string]string{"path": f.Path, "purpose": f.Purpose})
		}
		return map[string]any{
			"module":      plan.StructuredSkeleton.Module,
			"files":       files,
			"projectName": projectName,
			"projectGoal": plan.Goal,
		}
	}

	// 2. Parse folderStructure from techSpec (indented text → file list).
	fsRaw, ok := plan.TechSpec["folderStructure"]
	if !ok {
		return nil
	}
	fsText, ok := fsRaw.(string)
	if !ok || strings.TrimSpace(fsText) == "" {
		return nil
	}

	// Detect language from techStack.
	moduleType := "go" // default
	moduleName := projectName
	if ts, ok := plan.TechSpec["techStack"].(string); ok {
		lower := strings.ToLower(ts)
		if strings.Contains(lower, "react") || strings.Contains(lower, "node") || strings.Contains(lower, "typescript") {
			moduleType = "node"
		} else if strings.Contains(lower, "python") || strings.Contains(lower, "django") || strings.Contains(lower, "flask") {
			moduleType = "python"
		}
	}

	// Parse indented lines into file paths, preserving hierarchy.
	// Tracks indentation depth to build full paths.
	//   cmd/
	//     server/
	//       main.go   → cmd/server/main.go
	var files []map[string]string
	type stackEntry struct {
		name  string
		depth int
	}
	var stack []stackEntry

	for _, line := range strings.Split(fsText, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Remove tree characters but preserve leading spaces for depth.
		cleaned := strings.NewReplacer("├─", "  ", "└─", "  ", "│", " ", "├", " ", "└", " ", "─", " ").Replace(line)

		// Calculate depth from leading whitespace.
		stripped := strings.TrimLeft(cleaned, " \t")
		if stripped == "" || strings.HasPrefix(stripped, "#") || strings.HasPrefix(stripped, "//") {
			continue
		}
		depth := len(cleaned) - len(stripped)

		// Extract name and optional purpose.
		name := stripped
		purpose := "placeholder"
		if idx := strings.IndexAny(name, " \t"); idx > 0 {
			rest := strings.TrimSpace(name[idx:])
			rest = strings.TrimLeft(rest, "—-#/ ")
			if rest != "" {
				purpose = rest
			}
			name = name[:idx]
		}

		// Pop stack to match current depth.
		for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
			stack = stack[:len(stack)-1]
		}

		// Build full path from stack.
		var fullPath strings.Builder
		for _, s := range stack {
			fullPath.WriteString(s.name)
			if !strings.HasSuffix(s.name, "/") {
				fullPath.WriteByte('/')
			}
		}
		fullPath.WriteString(name)

		isDir := strings.HasSuffix(name, "/")
		stack = append(stack, stackEntry{name: name, depth: depth})

		// Only add files (directories are created implicitly).
		if !isDir {
			files = append(files, map[string]string{"path": fullPath.String(), "purpose": purpose})
		}
	}

	if len(files) == 0 {
		return nil
	}

	return map[string]any{
		"module":      map[string]string{"type": moduleType, "name": moduleName},
		"files":       files,
		"projectName": projectName,
		"projectGoal": plan.Goal,
	}
}

// prepareHistoriesAndDispatch generates milestoneProposals + frontiers via Claude,
// saves all histories, then dispatches tasks. Called async after skeleton creation.
// agent-proxy-tactics.md Phase 1, Step 7 (a~e).
func (h *RunHandler) prepareHistoriesAndDispatch(plan *planSpec, nodes []treeNode, projectName string) {
	// Each LLM call gets its own 2-minute timeout (see below).
	// No global timeout — avoid cascading cancellation across many LLM calls.

	trackID := sanitizeTrackID(projectName)

	h.setProgress("skeleton_done", "Project skeleton created. Generating milestone specifications...")

	// a) Save proposal.json (plan document, strip features).
	if plan != nil && h.store != nil {
		proposalDoc := map[string]any{
			"title":       plan.Title,
			"goal":        plan.Goal,
			"description": plan.Description,
			"techSpec":    plan.TechSpec,
			"abstractSpec": plan.AbstractSpec,
		}
		_ = h.store.Put("tracks/"+trackID+"/histories/proposal.json", proposalDoc)
		log.Printf("[histories] Saved proposal.json for %s", projectName)
	}

	proposalText := ""
	if plan != nil {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n## Goal\n%s\n\n## Description\n%s\n", plan.Title, plan.Goal, plan.Description))
		if len(plan.TechSpec) > 0 {
			sb.WriteString("\n## Technical Specification\n")
			for k, v := range plan.TechSpec {
				if s, ok := v.(string); ok && s != "" {
					sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", k, s))
				}
			}
		}
		if len(plan.AbstractSpec) > 0 {
			sb.WriteString("\n## Domain Specification\n")
			for k, v := range plan.AbstractSpec {
				if s, ok := v.(string); ok && s != "" {
					sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", k, s))
				}
			}
		}
		proposalText = sb.String()
	}

	// Group nodes by type.
	var milestones, tasks []treeNode
	nodeByID := make(map[string]treeNode)
	for _, n := range nodes {
		nodeByID[n.ID] = n
		switch n.Type {
		case "milestone":
			milestones = append(milestones, n)
		case "task":
			tasks = append(tasks, n)
		}
	}

	// b) Generate milestoneProposal for each milestone via Claude.
	h.setProgress("generating_milestones", fmt.Sprintf("Generating milestone specifications (%d milestones)...", len(milestones)))
	milestoneProposals := make(map[string]string) // milestoneUID → milestoneProposal
	for _, m := range milestones {
		// Collect child task summaries for this milestone.
		var childSummaries []string
		for _, t := range tasks {
			if t.ParentID != nil && *t.ParentID == m.ID {
				childSummaries = append(childSummaries, t.Summary)
			}
		}

		if h.frontier != nil {
			// Each milestone gets its own 2-minute timeout.
			mlCtx, mlCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			mp, err := h.frontier.GenerateMilestoneProposal(mlCtx, proposalText, m.Summary, childSummaries)
			mlCancel()
			if err != nil {
				log.Printf("[histories] MilestoneProposal generation FAILED for %s: %v (using summary as fallback)", m.UID, err)
				mp = m.Summary
			} else {
				log.Printf("[histories] MilestoneProposal generated for %s (%d chars)", m.UID, len(mp))
			}
			milestoneProposals[m.UID] = mp
		} else {
			milestoneProposals[m.UID] = m.Summary
		}

		// Save milestones/{MLST-UID}.json (server bookkeeping, review agent access).
		milestoneHistory := map[string]any{
			"proposal":          proposalText,
			"milestoneProposal": milestoneProposals[m.UID],
			"uid":               m.UID,
			"tasks":             childSummaries,
		}
		_ = h.store.Put("tracks/"+trackID+"/histories/milestones/"+m.UID+".json", milestoneHistory)
		log.Printf("[histories] Saved milestone history for %s (%d tasks)", m.UID, len(childSummaries))
	}

	// c) Generate frontiers per milestone in batch (1 Gemini call per milestone).
	h.setProgress("generating_frontiers", fmt.Sprintf("Generating task frontiers (%d tasks across %d milestones)...", len(tasks), len(milestones)))

	// Group tasks by parent milestone.
	tasksByMilestone := make(map[string][]treeNode) // milestoneID → tasks
	for _, t := range tasks {
		pid := ""
		if t.ParentID != nil {
			pid = *t.ParentID
		}
		tasksByMilestone[pid] = append(tasksByMilestone[pid], t)
	}

	// For each milestone, generate all frontiers in one call.
	allFrontiers := make(map[string]string) // taskUID → frontier
	log.Printf("[histories] === FRONTIER GENERATION START === milestones=%d, tasks=%d, frontier=%v", len(milestones), len(tasks), h.frontier != nil)
	for _, m := range milestones {
		mTasks := tasksByMilestone[m.ID]
		log.Printf("[histories] Milestone %s (id=%s): %d tasks in group", m.UID, m.ID, len(mTasks))
		if len(mTasks) == 0 {
			log.Printf("[histories] WARNING: No tasks found for milestone %s (id=%s) — check tasksByMilestone grouping", m.UID, m.ID)
			continue
		}

		milestoneProposal := milestoneProposals[m.UID]
		h.setProgress("generating_frontiers", fmt.Sprintf("Generating frontiers for milestone %s (%d tasks)...", m.UID, len(mTasks)))

		var inputs []TaskFrontierInput
		for _, t := range mTasks {
			inputs = append(inputs, TaskFrontierInput{UID: t.UID, Summary: t.Summary})
		}

		if h.frontier != nil {
			frontierCtx, frontierCancel := context.WithTimeout(context.Background(), 3*time.Minute)
			log.Printf("[histories] Batch frontier generation for %s: %d tasks (proposal=%d, milestone=%d chars)",
				m.UID, len(inputs), len(proposalText), len(milestoneProposal))

			result, err := h.frontier.GenerateTaskFrontiers(frontierCtx, proposalText, milestoneProposal, inputs)
			frontierCancel()

			if err != nil {
				log.Printf("[histories] *** Batch frontier FAILED for %s: %v (using summaries as fallback)", m.UID, err)
			} else {
				for uid, f := range result {
					if f != "" {
						allFrontiers[uid] = f
					}
				}
				log.Printf("[histories] Batch frontiers generated for %s: %d/%d succeeded", m.UID, len(result), len(inputs))
			}
		}
	}

	// Save task histories.
	log.Printf("[histories] === SAVING TASK HISTORIES === totalFrontiers=%d, totalTasks=%d", len(allFrontiers), len(tasks))
	for _, t := range tasks {
		parentMilestoneUID := ""
		milestoneProposal := ""
		if t.ParentID != nil {
			if parent, ok := nodeByID[*t.ParentID]; ok && parent.Type == "milestone" {
				parentMilestoneUID = parent.UID
				milestoneProposal = milestoneProposals[parent.UID]
			}
		}

		frontier := t.Summary // fallback
		if f, ok := allFrontiers[t.UID]; ok {
			frontier = f
			log.Printf("[histories] Task %s: using GENERATED frontier (%d chars)", t.UID, len(f))
		} else {
			log.Printf("[histories] Task %s: FALLBACK to summary (frontier not found in allFrontiers)", t.UID)
		}

		taskHistory := map[string]any{
			"proposal":          proposalText,
			"milestoneProposal": milestoneProposal,
			"frontier":          frontier,
			"uid":               t.UID,
			"milestoneUid":      parentMilestoneUID,
		}
		_ = h.store.Put("tracks/"+trackID+"/histories/tasks/"+t.UID+".json", taskHistory)
		log.Printf("[histories] Saved task history for %s (frontier: %d chars, isSummary: %v)", t.UID, len(frontier), frontier == t.Summary)
	}

	log.Printf("[histories] All histories generated for %s — dispatching tasks", projectName)

	h.setProgress("dispatching", "All specifications ready. Dispatching agents...")

	// e) All histories saved → dispatch tasks (frontiers will be loaded in dispatchTasks).
	h.DispatchAllPending()

	h.setProgress("running", "Agents dispatched and running.")
}

const contextMdInstructions = `

## WHEN DONE — CRITICAL
1. git add . && git commit -m "descriptive message about what you implemented"
2. Create .samams-context.md in the project root with EXACTLY this format:
   ## Summary
   2-3 sentences describing what you accomplished.
   ## Files Modified
   - path/to/file.go: what was changed and why
   - path/to/other.go: what was changed and why
   ## Issues
   - Any problems encountered (or "None")
   ## Dependencies
   - Any tasks this depends on (or "None")
3. Do NOT push. Just commit and stop.
`

func mapAgent(agent string) string {
	a := strings.ToLower(agent)
	switch {
	case strings.Contains(a, "cursor"):
		return "cursor"
	case strings.Contains(a, "claude"):
		return "claude"
	case strings.Contains(a, "opencode"):
		return "opencode"
	default:
		return "cursor"
	}
}

// ── Strategy Meeting REST Endpoints ─────────────────────────

// SendProxyCommand sends a command to the proxy and returns the response.
func (h *RunHandler) SendProxyCommand(ctx context.Context, action string, payload any) (json.RawMessage, error) {
	if !h.hub.AnyConnected() {
		return nil, fmt.Errorf("proxy not connected")
	}
	return h.hub.SendCommand(ctx, h.hub.FirstUserID(), action, payload)
}

func (h *RunHandler) startStrategyMeeting(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectName string `json:"projectName"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProjectName == "" {
		WriteError(w, http.StatusBadRequest, "projectName is required")
		return
	}

	// Lock to prevent concurrent meeting creation (read-check-write race).
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check no meeting in progress.
	var existing map[string]any
	if err := h.store.Get("strategy-meetings/current/meta.json", &existing); err == nil {
		if status, _ := existing["status"].(string); status != "" && status != "idle" {
			WriteError(w, http.StatusConflict, "meeting already in progress: "+status)
			return
		}
	}

	// Check proxy connected.
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy not connected")
		return
	}

	// Find running task UIDs.
	participants := h.findRunningTaskUIDs(req.ProjectName)
	if len(participants) == 0 {
		WriteError(w, http.StatusBadRequest, "no running tasks found for project")
		return
	}

	// Create meeting meta — shared type ensures field parity with event_processor.
	sessionID := fmt.Sprintf("sm-%d", time.Now().UnixMilli())
	meta := MeetingMeta{
		SessionID:           sessionID,
		ProjectName:         req.ProjectName,
		Status:              "pausing",
		Trigger:             "manual",
		Round:               1,
		MaxRounds:           3,
		ParticipantNodeUIDs: participants,
		CreatedAt:           time.Now().UnixMilli(),
	}
	_ = h.store.Put("strategy-meetings/current/meta.json", meta)

	// Send pause command with participant list to proxy.
	if _, err := h.hub.SendCommand(r.Context(), h.hub.FirstUserID(), ActionStrategyPauseAll, map[string]any{
		"participantNodeUids": participants,
	}); err != nil {
		meta.Status = "idle"
		_ = h.store.Put("strategy-meetings/current/meta.json", meta)
		WriteError(w, http.StatusInternalServerError, "failed to pause agents: "+err.Error())
		return
	}

	log.Printf("[strategy] Meeting %s started manually (participants: %d)", sessionID, len(participants))
	WriteOK(w, meta)
}

func (h *RunHandler) strategyMeetingStatus(w http.ResponseWriter, _ *http.Request) {
	var meta map[string]any
	if err := h.store.Get("strategy-meetings/current/meta.json", &meta); err != nil {
		WriteOK(w, map[string]string{"status": "idle"})
		return
	}
	WriteOK(w, meta)
}

// pauseAll pauses all running tasks via proxy.
func (h *RunHandler) pauseAll(w http.ResponseWriter, r *http.Request) {
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy not connected")
		return
	}
	resp, err := h.hub.SendCommand(r.Context(), h.hub.FirstUserID(), ActionPauseTask, map[string]string{"taskID": "*"})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "pause all failed: "+err.Error())
		return
	}
	WriteOK(w, json.RawMessage(resp))
}

// resumeAll resumes all paused tasks via proxy.
func (h *RunHandler) resumeAll(w http.ResponseWriter, r *http.Request) {
	if !h.hub.AnyConnected() {
		WriteError(w, http.StatusServiceUnavailable, "proxy not connected")
		return
	}
	resp, err := h.hub.SendCommand(r.Context(), h.hub.FirstUserID(), ActionResumeTask, map[string]string{"taskID": "*"})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "resume all failed: "+err.Error())
		return
	}
	WriteOK(w, json.RawMessage(resp))
}

// findRunningTaskUIDs returns nodeUIDs of running/scaling tasks for a project.
// Based on tree.json status — may briefly include tasks that just completed on proxy
// but whose completion event hasn't been processed yet (harmless race).
// TriggerStrategyMeeting starts a strategy meeting programmatically (auto-trigger).
// failedNodeUID is the task that triggered the meeting — only sibling tasks under the
// same milestone participate. Returns true if a meeting was started.
func (h *RunHandler) TriggerStrategyMeeting(projectName, trigger, failedNodeUID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check no meeting in progress.
	var existing map[string]any
	if err := h.store.Get("strategy-meetings/current/meta.json", &existing); err == nil {
		if status, _ := existing["status"].(string); status != "" && status != "idle" {
			log.Printf("[strategy] Auto-trigger skipped: meeting already in progress (%s)", status)
			return false
		}
	}

	if !h.hub.AnyConnected() {
		log.Println("[strategy] Auto-trigger skipped: proxy not connected")
		return false
	}

	// Find sibling tasks under the same milestone as the failed task.
	participants := h.findSiblingTaskUIDs(projectName, failedNodeUID)
	if len(participants) == 0 {
		log.Printf("[strategy] Auto-trigger skipped: no sibling tasks for %s", failedNodeUID)
		return false
	}

	sessionID := fmt.Sprintf("sm-%d", time.Now().UnixMilli())
	meta := MeetingMeta{
		SessionID:           sessionID,
		ProjectName:         projectName,
		Status:              "pausing",
		Trigger:             trigger,
		Round:               1,
		MaxRounds:           3,
		ParticipantNodeUIDs: participants,
		CreatedAt:           time.Now().UnixMilli(),
	}
	_ = h.store.Put("strategy-meetings/current/meta.json", meta)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := h.hub.SendCommand(ctx, h.hub.FirstUserID(), ActionStrategyPauseAll, map[string]any{
		"participantNodeUids": participants,
	}); err != nil {
		meta.Status = "idle"
		_ = h.store.Put("strategy-meetings/current/meta.json", meta)
		log.Printf("[strategy] Auto-trigger failed: %v", err)
		return false
	}

	log.Printf("[strategy] Meeting %s auto-triggered (%s, failed=%s, participants: %v)", sessionID, trigger, failedNodeUID, participants)
	return true
}

// findSiblingTaskUIDs returns all active/running task UIDs under the same milestone
// as the given nodeUID. Includes the failed task itself (for watch agent analysis).
func (h *RunHandler) findSiblingTaskUIDs(projectName, nodeUID string) []string {
	keys, _ := h.store.List("tracks")
	for _, k := range keys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		var track map[string]any
		if err := h.store.Get(k, &track); err != nil {
			continue
		}
		pn, _ := track["projectName"].(string)
		if pn != projectName && projectName != "" {
			continue
		}
		nodes, _ := track["nodes"].([]any)

		// Find the parent (milestone) of the failed task.
		var failedParentID string
		for _, n := range nodes {
			node, _ := n.(map[string]any)
			uid, _ := node["uid"].(string)
			if uid == nodeUID {
				failedParentID, _ = node["parentId"].(string)
				break
			}
		}
		if failedParentID == "" {
			continue
		}

		// Collect all task UIDs under the same milestone (any status except "complete").
		var siblings []string
		for _, n := range nodes {
			node, _ := n.(map[string]any)
			nType, _ := node["type"].(string)
			parentID, _ := node["parentId"].(string)
			status, _ := node["status"].(string)
			uid, _ := node["uid"].(string)
			if nType == "task" && parentID == failedParentID && uid != "" && status != "complete" {
				siblings = append(siblings, uid)
			}
		}
		return siblings
	}
	return nil
}

func (h *RunHandler) findRunningTaskUIDs(projectName string) []string {
	keys, _ := h.store.List("tracks")
	var uids []string
	for _, k := range keys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		var track map[string]any
		if err := h.store.Get(k, &track); err != nil {
			continue
		}
		pn, _ := track["projectName"].(string)
		if pn != projectName && projectName != "" {
			continue
		}
		nodes, _ := track["nodes"].([]any)
		for _, n := range nodes {
			node, _ := n.(map[string]any)
			nodeType, _ := node["type"].(string)
			status, _ := node["status"].(string)
			uid, _ := node["uid"].(string)
			if nodeType == "task" && (status == "running" || status == "active" || status == "scaling") && uid != "" {
				uids = append(uids, uid)
			}
		}
	}
	return uids
}
