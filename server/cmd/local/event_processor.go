// Channel-based sequential event processor for local development mode.
// All state-mutating operations run on a single goroutine (event loop).
// Long-running operations (LLM calls) run in separate goroutines and
// post results back as callback events.
//
// Pattern: Fast-path (inline) / Slow-path (async callback)
// Reference: client/proxy/internal/adapter/outbound/gitbranch/manager.go (mergeQueue)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	appAgent "server/internal/app/agent"
	appControl "server/internal/app/control"
	appStrategy "server/internal/app/strategy"
	appTask "server/internal/app/task"

	"server/infra/in/httpserver"
	"server/infra/persistence/localstore"
)

// eventEnvelope wraps both external WebSocket events and internal callback events.
type eventEnvelope struct {
	// External event fields (from WebSocket)
	ctx     context.Context
	userID  string
	action  string
	payload json.RawMessage

	// Internal callback (non-nil for LLM result callbacks)
	callback func()
}

// eventProcessor is a channel-based sequential event processor.
// Implements httpserver.ProxyEventRouter interface.
type eventProcessor struct {
	ch      chan eventEnvelope
	done    chan struct{}
	closing atomic.Bool
	wg      sync.WaitGroup // tracks in-flight LLM goroutines

	// Dependencies
	control    *appControl.Service
	strategy   *appStrategy.Service
	task       *appTask.Service
	summarizer appAgent.Summarizer
	planner    appAgent.Planner
	runHandler *httpserver.RunHandler
	store      *localstore.Store
}

func newEventProcessor(
	control *appControl.Service,
	strategy *appStrategy.Service,
	task *appTask.Service,
	summarizer appAgent.Summarizer,
	planner appAgent.Planner,
	runHandler *httpserver.RunHandler,
	store *localstore.Store,
) *eventProcessor {
	p := &eventProcessor{
		ch:         make(chan eventEnvelope, 256),
		done:       make(chan struct{}),
		control:    control,
		strategy:   strategy,
		task:       task,
		summarizer: summarizer,
		planner:    planner,
		runHandler: runHandler,
		store:      store,
	}
	go p.run()
	return p
}

// run is the single event loop goroutine. All state mutations happen here.
func (p *eventProcessor) run() {
	defer close(p.done)
	slog.Info("[event-loop] started")
	for ev := range p.ch {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[event-loop] PANIC recovered: %v (action=%s)", r, ev.action)
				}
			}()
			if ev.callback != nil {
				ev.callback()
			} else {
				p.handleEvent(ev)
			}
		}()
	}
	slog.Info("[event-loop] stopped")
}

// RouteEvent implements httpserver.ProxyEventRouter. Non-blocking enqueue.
func (p *eventProcessor) RouteEvent(ctx context.Context, userID, action string, payload json.RawMessage) {
	select {
	case p.ch <- eventEnvelope{ctx: ctx, userID: userID, action: action, payload: payload}:
		slog.Info("[event-loop] queued", "action", action, "userID", userID)
	default:
		slog.Warn("[event-loop] queue full, dropping event", "action", action)
	}
}

// Close drains the event queue and waits for in-flight LLM goroutines.
func (p *eventProcessor) Close() {
	p.closing.Store(true)
	p.wg.Wait()  // wait for in-flight LLM goroutines to finish
	close(p.ch)
	<-p.done // wait for event loop to drain
}

// ── Event Handling ──────────────────────────────────────────

func (p *eventProcessor) handleEvent(ev eventEnvelope) {
	var payload map[string]any
	if len(ev.payload) > 0 {
		_ = json.Unmarshal(ev.payload, &payload)
	}

	switch ev.action {
	case "contextLost":
		projectID, _ := payload["projectID"].(string)
		if projectID == "" {
			projectID = "default"
		}
		if err := p.control.ContextLostDetected(ev.ctx, appControl.LoadContextCommand{ProjectID: projectID}); err != nil {
			log.Printf("[event-loop] ContextLostDetected error: %v", err)
		}

	case "task.failed":
		taskID, _ := payload["taskID"].(string)
		nodeUID, _ := payload["nodeUid"].(string)
		projectName, _ := payload["projectName"].(string)
		reason, _ := payload["reason"].(string)
		if taskID == "" {
			return
		}
		log.Printf("[event-loop] Task failed: task=%s node=%s reason=%s", taskID, nodeUID, reason)

		// Mark node as error in tree.json + release SSOT + check milestone.
		if p.store != nil && nodeUID != "" {
			p.updateTrackNode(nodeUID, taskID, "error", map[string]any{
				"errorMessage": reason,
			})
		}
		if projectName != "" && nodeUID != "" && p.runHandler != nil {
			p.runHandler.RemoveOccupiedNode(projectName, nodeUID)
			p.checkMilestoneCompletion(nodeUID, projectName)
		}

		// Auto-trigger strategy meeting on any task error (merge conflict, agent error, etc.).
		// Only sibling tasks under the same milestone participate.
		if projectName != "" && nodeUID != "" && p.runHandler != nil {
			trigger := "task_error"
			if reason == "merge conflict" {
				trigger = "merge_conflict"
			}
			if p.runHandler.TriggerStrategyMeeting(projectName, trigger, nodeUID) {
				log.Printf("[event-loop] Strategy meeting auto-triggered by %s (node=%s)", trigger, nodeUID)
			} else {
				// No meeting started — continue normal dispatch.
				p.runHandler.DispatchNextPendingTask(projectName)
			}
		}

	case "task.completed":
		p.handleTaskCompleted(ev, payload)

	case "agent.stateChanged":
		agentID, _ := payload["agent_id"].(string)
		newState, _ := payload["new_state"].(string)
		taskID, _ := payload["taskID"].(string)
		nodeUID, _ := payload["nodeUid"].(string)
		projectName, _ := payload["projectName"].(string)
		errorMsg, _ := payload["message"].(string)
		log.Printf("[event-loop] Agent state changed: %s → %s (task=%s node=%s)", agentID, newState, taskID, nodeUID)
		p.appendTrackLog(taskID, map[string]any{
			"type": "state_changed", "agentID": agentID,
			"newState": newState, "message": fmt.Sprintf("Agent %s → %s", agentID, newState),
		})

		// If agent errored, mark task as error in tree.json + release SSOT + check milestone.
		if newState == "error" || errorMsg != "" {
			log.Printf("[event-loop] Agent error detected: %s (node=%s, msg=%s)", agentID, nodeUID, errorMsg)
			if p.store != nil && nodeUID != "" {
				p.updateTrackNode(nodeUID, taskID, "error", map[string]any{
					"errorMessage": errorMsg,
				})
			}
			if projectName != "" && nodeUID != "" && p.runHandler != nil {
				p.runHandler.RemoveOccupiedNode(projectName, nodeUID)
				p.checkMilestoneCompletion(nodeUID, projectName)
				p.runHandler.DispatchNextPendingTask(projectName)
			}
		}

	case "agent.logAppended":
		taskID, _ := payload["taskID"].(string)
		lines, _ := payload["lines"].([]any)
		for _, line := range lines {
			if s, ok := line.(string); ok {
				p.appendTrackLog(taskID, map[string]any{"type": "log", "message": s})
			}
		}
		if msg, ok := payload["message"].(string); ok && msg != "" {
			p.appendTrackLog(taskID, map[string]any{"type": "log", "message": msg})
		}

	case "heartbeat":
		// Informational — no storage needed.

	case "maal.record":
		taskID, _ := payload["taskID"].(string)
		action, _ := payload["action"].(string)
		content, _ := payload["content"].(string)
		p.appendTrackLog(taskID, map[string]any{"type": "maal", "action": action, "message": content})

	case "milestone.review.completed":
		milestoneUID, _ := payload["nodeUid"].(string)
		reviewContext, _ := payload["context"].(string)
		projName, _ := payload["projectName"].(string)
		log.Printf("[event-loop] Milestone review completed: %s", milestoneUID)

		// Explicit capture for closure consistency.
		capturedUID := milestoneUID
		capturedContext := reviewContext
		capturedProjName := projName

		// Slow-path: Claude analysis
		p.asyncLLM(func() (func(), error) {
			result, err := p.processMilestoneReviewAsync(capturedUID, capturedContext)
			if err != nil {
				return nil, err
			}
			return func() {
				p.applyReviewDecision(capturedUID, result, capturedProjName)
				if p.runHandler != nil && capturedProjName != "" {
					p.runHandler.DispatchNextPendingTask(capturedProjName)
				}
			}, nil
		})

	case "milestone.review.failed":
		failedNodeUID, _ := payload["nodeUid"].(string)
		projName, _ := payload["projectName"].(string)
		log.Printf("[event-loop] Review FAILED: %s — auto-approving", failedNodeUID)
		if failedNodeUID != "" {
			failedNodeType := p.getNodeTypeFromTree(failedNodeUID)
			p.completeNode(failedNodeUID, failedNodeType, map[string]any{
				"reviewDecision": "auto-approved (review agent failed)",
			})
			if failedNodeType == "milestone" && p.runHandler != nil && projName != "" {
				_ = p.runHandler.MergeMilestoneToMain(failedNodeUID, projName)
				p.checkProposalCompletion(failedNodeUID, projName)
			}
			if p.runHandler != nil && projName != "" {
				p.runHandler.DispatchNextPendingTask(projName)
			}
		}

	case "milestone.merged":
		milestoneUID, _ := payload["milestoneUID"].(string)
		targetBranch, _ := payload["targetBranch"].(string)
		log.Printf("[event-loop] Milestone merged: %s → %s", milestoneUID, targetBranch)

	case "strategy.allPaused":
		p.handleStrategyAllPaused(payload)

	case "strategy.decisionApplied":
		log.Println("[event-loop] Strategy decision applied by proxy")

	case "memoryIssue":
		agentID, _ := payload["agentID"].(string)
		memMB, _ := payload["memoryMB"].(float64)
		log.Printf("[event-loop] WARNING: Agent %s memory issue: %.0fMB", agentID, memMB)

	default:
		log.Printf("[event-loop] Unhandled event: %s", ev.action)
	}
}

// ── task.completed handler (fast-path + slow-path) ──────────

func (p *eventProcessor) handleTaskCompleted(ev eventEnvelope, payload map[string]any) {
	taskID, _ := payload["taskID"].(string)
	agentID, _ := payload["agentID"].(string)
	agentContext, _ := payload["context"].(string)
	frontier, _ := payload["frontier"].(string)
	nodeType, _ := payload["nodeType"].(string)
	nodeUID, _ := payload["nodeUid"].(string)
	projectName, _ := payload["projectName"].(string)
	log.Printf("[event-loop] Task completed: task=%s agent=%s nodeType=%s", taskID, agentID, nodeType)

	// === Fast-path (inline, single goroutine) ===

	// 1. Update tracks: mark complete + save frontier.
	if p.store != nil && nodeUID != "" {
		p.appendTrackLog(taskID, map[string]any{
			"type": "task_completed", "agentID": agentID,
			"nodeType": nodeType, "nodeUid": nodeUID,
		})
		p.updateTrackNode(nodeUID, taskID, "complete", map[string]any{
			"frontier":    frontier,
			"completedAt": time.Now().UnixMilli(),
		})
	}

	// 2. If proposal: dispatch pending tasks + init milestone histories.
	if nodeType == "proposal" && p.runHandler != nil {
		log.Println("[event-loop] Proposal setup complete — dispatching pending tasks...")
		p.runHandler.DispatchAllPending()
		p.initMilestoneHistories(nodeUID)
	}

	// 3. SSOT: release occupied node.
	if projectName != "" && nodeUID != "" && p.runHandler != nil {
		p.runHandler.RemoveOccupiedNode(projectName, nodeUID)
	}

	// 4. FAST-PATH: check milestone completion + dispatch next task.
	// No need to wait for LLM summarize — tree.json status is already "complete" (step 1).
	// Skip dispatch if a strategy meeting is in progress — meeting controls agent lifecycle.
	if nodeType == "task" && projectName != "" {
		log.Printf("[event-loop] Fast-path: checking milestone completion for %s (project=%s)", nodeUID, projectName)
		p.checkMilestoneCompletion(nodeUID, projectName)
		if p.runHandler != nil && !p.isStrategyMeetingActive() {
			p.runHandler.DispatchNextPendingTask(projectName)
		}
	}

	// === Slow-path (LLM async) — summary + history only ===
	if taskID != "" {
		capturedTaskID := taskID
		capturedNodeUID := nodeUID
		capturedNodeType := nodeType
		capturedAgentContext := agentContext

		p.asyncLLM(func() (func(), error) {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// LLM call (slow, 10-30s) — runs outside event loop.
			summary := capturedAgentContext
			if p.summarizer != nil && capturedAgentContext != "" {
				if s, err := p.summarizer.Summarize(timeoutCtx, capturedAgentContext); err == nil {
					summary = s
					log.Printf("[event-loop] Summary generated for %s (%d chars → %d chars)", capturedTaskID, len(capturedAgentContext), len(summary))
				} else {
					log.Printf("[event-loop] Summarizer failed for %s: %v (using raw context)", capturedTaskID, err)
				}
			}

			capturedSummary := summary

			// Return callback that runs on event loop goroutine.
			return func() {
				// Save summary + history only (milestone check already done in fast-path).
				if p.store != nil && capturedNodeUID != "" {
					p.updateTrackNode(capturedNodeUID, capturedTaskID, "", map[string]any{
						"summary": capturedSummary,
					})
					p.buildAndSaveHistory(capturedNodeUID, capturedNodeType)
				}

				// NOTE: p.task.CompleteAndCascade is skipped in local mode.
				// The proxy generates its own task IDs that don't exist in the
				// server's DDD task repository (designed for Lambda handlers).
				// All necessary work (tree.json update, milestone check, dispatch)
				// is already handled in the fast-path above using nodeUID.
			}, nil
		})
	}
}

// ── Async LLM helper ────────────────────────────────────────

// asyncLLM runs fn in a separate goroutine. fn returns a callback to be
// executed on the event loop goroutine. If fn returns an error, it's logged.
func (p *eventProcessor) asyncLLM(fn func() (callback func(), err error)) {
	if p.closing.Load() {
		return
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		callback, err := fn()
		if err != nil {
			log.Printf("[event-loop] async LLM error: %v", err)
			return
		}
		if callback != nil && !p.closing.Load() {
			p.ch <- eventEnvelope{callback: callback}
		}
	}()
}

// ── Review processing (async part) ──────────────────────────

type reviewDecision struct {
	Decision  string
	Reasoning string
	NewTasks  []struct {
		Summary        string `json:"summary"`
		Detail         string `json:"detail"`
		ParentUID      string `json:"parentUid"`
		Relationship   string `json:"relationship"`
		Priority       string `json:"priority"`
		Reason         string `json:"reason"`
		BoundedContext string `json:"boundedContext"`
	}
}

func (p *eventProcessor) processMilestoneReviewAsync(milestoneUID, reviewContext string) (*reviewDecision, error) {
	if p.planner == nil {
		return &reviewDecision{Decision: "APPROVED", Reasoning: "no planner configured"}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := p.planner.AnalyzeReview(ctx, "## Milestone UID: "+milestoneUID+"\n\n## Review Agent Findings\n"+reviewContext)
	if err != nil {
		return &reviewDecision{Decision: "APPROVED", Reasoning: "analysis failed: " + err.Error()}, nil
	}

	var decision reviewDecision
	cleaned := extractJSON(result)
	if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
		return &reviewDecision{Decision: "APPROVED", Reasoning: "parse error: " + err.Error()}, nil
	}
	return &decision, nil
}

func (p *eventProcessor) applyReviewDecision(nodeUID string, decision *reviewDecision, projectName string) {
	nodeType := p.getNodeTypeFromTree(nodeUID)
	log.Printf("[event-loop] Review %s (type=%s) decision: %s — %s", nodeUID, nodeType, decision.Decision, decision.Reasoning)

	if decision.Decision == "APPROVED" || len(decision.NewTasks) == 0 {
		p.completeNode(nodeUID, nodeType, map[string]any{
			"reviewDecision": "APPROVED: " + decision.Reasoning,
		})
		// Only merge for milestones — proposals are already on main, no merge needed.
		if nodeType == "milestone" && p.runHandler != nil {
			if err := p.runHandler.MergeMilestoneToMain(nodeUID, projectName); err != nil {
				log.Printf("[event-loop] Milestone merge failed %s: %v", nodeUID, err)
			} else {
				log.Printf("[event-loop] Milestone %s merged → main", nodeUID)
			}
		}
		if nodeType == "milestone" {
			p.checkProposalCompletion(nodeUID, projectName)
		} else if nodeType == "proposal" {
			log.Printf("[event-loop] Proposal %s review APPROVED — project complete!", nodeUID)
		}
		return
	}

	// NEEDS_WORK: add new tasks to tree and dispatch.
	log.Printf("[event-loop] %s %s needs work — adding %d new tasks", nodeType, nodeUID, len(decision.NewTasks))

	keys, _ := p.store.List("tracks")
	for _, k := range keys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		var track map[string]any
		if err := p.store.Get(k, &track); err != nil {
			continue
		}
		nodes, _ := track["nodes"].([]any)

		uidToID := make(map[string]string)
		var nodeID string
		reviewCycle := 1
		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			uid, _ := node["uid"].(string)
			id, _ := node["id"].(string)
			uidToID[uid] = id
			if uid == nodeUID {
				nodeID = id
				if rc, ok := node["reviewCycle"].(float64); ok {
					reviewCycle = int(rc)
				}
			}
		}

		for i, nt := range decision.NewTasks {
			newUID := fmt.Sprintf("RVTASK-%s-R%d-%d", nodeUID, reviewCycle, i+1)
			newID := fmt.Sprintf("rv-%d-%d-%d", time.Now().UnixMilli(), reviewCycle, i)

			parentID := nodeID
			if nt.ParentUID != "" {
				if resolved, ok := uidToID[nt.ParentUID]; ok {
					parentID = resolved
				}
			}

			nodes = append(nodes, map[string]any{
				"id": newID, "uid": newUID, "type": "task",
				"summary": nt.Summary, "agent": "Cursor Agent",
				"status": "pending", "priority": nt.Priority,
				"parentId": parentID, "boundedContext": nt.BoundedContext,
				"origin": "review", "reviewCycle": reviewCycle,
				"reason": nt.Reason, "relationship": nt.Relationship,
				"detail": nt.Detail,
			})
		}

		track["nodes"] = nodes
		_ = p.store.Put(k, track)

		p.updateTrackNode(nodeUID, "", "running", map[string]any{
			"reviewDecision": "NEEDS_WORK: " + decision.Reasoning,
		})

		if p.runHandler != nil {
			p.runHandler.DispatchReviewTasks(projectName, nodeUID, decision.NewTasks)
		}
		return
	}
}

