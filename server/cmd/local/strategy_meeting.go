// Strategy meeting orchestrator — manages the full meeting lifecycle on the event loop.
// State is persisted in localstore at strategy-meetings/current/meta.json.
//
// State machine:
//   [trigger] → pausing → analyzing → dispatching → idle
//
// The proxy handles watch agent lifecycle internally (interrupt workers, spawn watch agents,
// collect .samams-context.md, then emit strategy.allPaused with discussionContexts payload).
// The server only does LLM analysis and sends a single strategyApplyDecision command.
//
// The REST endpoints (startStrategyMeeting, strategyMeetingStatus) live in run_handler.go.
// This file contains the event-driven orchestration on the eventProcessor.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"server/infra/in/httpserver"
	"server/infra/out/llm/prompt"
	"server/internal/domain/llm"
)

// ── Meeting Types ────────────────────────────────────────────

// meetingMeta aliases the shared type from httpserver to keep local references short.
type meetingMeta = httpserver.MeetingMeta

type strategyDecision struct {
	Reasoning   string       `json:"reasoning"`
	TaskActions []taskAction `json:"taskActions"`
}

type taskAction struct {
	NodeUID   string `json:"nodeUid"`
	Action    string `json:"action"` // keep, reset_and_retry, cancel
	NewPrompt string `json:"newPrompt,omitempty"`
}

const meetingMetaKey = "strategy-meetings/current/meta.json"

// extractJSON extracts the top-level JSON object from LLM responses that may be
// wrapped in markdown code fences or have trailing explanation text.
// Uses brace-depth counting to find the matching closing '}' for the first '{',
// rather than relying on LastIndex which can capture trailing text with braces.
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	start := strings.Index(s, "{")
	if start == -1 {
		return s
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inString {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	// Fallback: unbalanced braces — return from first '{' to end.
	return s[start:]
}

// ── State Helpers ────────────────────────────────────────────

func (p *eventProcessor) getMeetingMeta() *meetingMeta {
	if p.store == nil {
		return nil
	}
	var meta meetingMeta
	if err := p.store.Get(meetingMetaKey, &meta); err != nil {
		return nil
	}
	return &meta
}

// isStrategyMeetingActive returns true if a strategy meeting is in progress (not idle).
func (p *eventProcessor) isStrategyMeetingActive() bool {
	meta := p.getMeetingMeta()
	return meta != nil && meta.Status != "" && meta.Status != "idle"
}

func (p *eventProcessor) saveMeetingMeta(meta *meetingMeta) {
	if p.store != nil {
		if err := p.store.Put(meetingMetaKey, meta); err != nil {
			log.Printf("[strategy] Failed to save meeting meta: %v", err)
		}
	}
}

// ── Proxy Command Helper ─────────────────────────────────────

func (p *eventProcessor) sendProxyCommand(action string, payload any) (json.RawMessage, error) {
	if p.runHandler == nil {
		return nil, fmt.Errorf("run handler not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.runHandler.SendProxyCommand(ctx, action, payload)
}

// ── Handle strategy.allPaused ────────────────────────────────
// Proxy sends this event after: interrupting all workers, spawning watch agents,
// collecting .samams-context.md from each, and including discussionContexts in the payload.

func (p *eventProcessor) handleStrategyAllPaused(payload map[string]any) {
	meta := p.getMeetingMeta()
	if meta == nil || meta.Status != "pausing" {
		log.Println("[strategy] allPaused received but no meeting in pausing state")
		return
	}

	// Extract discussionContexts from event payload (proxy collected these).
	contexts := make(map[string]string)
	if raw, ok := payload["discussionContexts"].(map[string]any); ok {
		for k, v := range raw {
			if s, ok := v.(string); ok {
				contexts[k] = s
			}
		}
	}

	meta.DiscussionContexts = contexts
	meta.Status = "analyzing"
	p.saveMeetingMeta(meta)

	log.Printf("[strategy] All agents interrupted, %d contexts collected — starting LLM analysis", len(contexts))
	p.analyzeStrategyDiscussions(meta.SessionID)
}

// ── LLM Analysis (async) ────────────────────────────────────

func (p *eventProcessor) analyzeStrategyDiscussions(sessionID string) {
	meta := p.getMeetingMeta()
	if meta == nil {
		log.Println("[strategy] analyzeStrategyDiscussions: meeting meta not found")
		return
	}
	capturedSessionID := sessionID
	capturedParticipants := append([]string{}, meta.ParticipantNodeUIDs...)
	capturedContexts := make(map[string]string, len(meta.DiscussionContexts))
	for k, v := range meta.DiscussionContexts {
		capturedContexts[k] = v
	}

	p.asyncLLM(func() (func(), error) {
		// Build discussion summaries from proxy-collected contexts.
		var discussions []string
		for _, uid := range capturedParticipants {
			ctx, ok := capturedContexts[uid]
			if !ok || ctx == "" {
				continue
			}
			discussions = append(discussions, fmt.Sprintf("## Agent %s Analysis\n%s", uid, ctx))
		}

		if len(discussions) == 0 {
			return func() {
				p.resolveStrategyMeeting(capturedSessionID, &strategyDecision{
					Reasoning: "no discussion results collected — resuming all",
					TaskActions: buildKeepAll(capturedParticipants),
				})
			}, nil
		}

		userPrompt := fmt.Sprintf("## Strategy Meeting %s\n\n## Participants: %s\n\n## Agent Analyses\n\n%s\n\nAnalyze the above reports and produce your decision.",
			capturedSessionID,
			strings.Join(capturedParticipants, ", "),
			strings.Join(discussions, "\n\n---\n\n"))

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		resp, err := p.planner.Complete(ctx, llm.CompletionRequest{
			SystemPrompt: prompt.StrategyAnalysis,
			UserPrompt:   userPrompt,
			MaxTokens:    4096,
			Temperature:  0.3,
		})
		if err != nil {
			return func() {
				p.resolveStrategyMeeting(capturedSessionID, &strategyDecision{
					Reasoning: "LLM analysis failed: " + err.Error(),
					TaskActions: buildKeepAll(capturedParticipants),
				})
			}, nil
		}

		var decision strategyDecision
		cleaned := extractJSON(resp.Content)
		if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
			return func() {
				p.resolveStrategyMeeting(capturedSessionID, &strategyDecision{
					Reasoning: "JSON parse error: " + err.Error(),
					TaskActions: buildKeepAll(capturedParticipants),
				})
			}, nil
		}

		capturedDecision := decision
		return func() {
			p.resolveStrategyMeeting(capturedSessionID, &capturedDecision)
		}, nil
	})
}

// buildKeepAll creates a "keep" action for every participant (fallback when analysis fails).
func buildKeepAll(participants []string) []taskAction {
	actions := make([]taskAction, len(participants))
	for i, uid := range participants {
		actions[i] = taskAction{NodeUID: uid, Action: "keep"}
	}
	return actions
}

// ── Apply Decision ──────────────────────────────────────────

func (p *eventProcessor) resolveStrategyMeeting(sessionID string, decision *strategyDecision) {
	meta := p.getMeetingMeta()
	if meta == nil {
		return
	}

	log.Printf("[strategy] Decision for %s: %s", sessionID, decision.Reasoning)

	// Persist decision.
	decisionKey := fmt.Sprintf("strategy-meetings/%s/decision.json", sessionID)
	if err := p.store.Put(decisionKey, decision); err != nil {
		log.Printf("[strategy] Failed to persist decision for %s: %v", sessionID, err)
	}

	// Ensure every participant has an action — LLM may skip participants with empty context.
	// Missing participants default to "keep" (resume as-is).
	covered := make(map[string]bool, len(decision.TaskActions))
	for _, ta := range decision.TaskActions {
		covered[ta.NodeUID] = true
	}
	for _, uid := range meta.ParticipantNodeUIDs {
		if !covered[uid] {
			log.Printf("[strategy] Participant %s missing from LLM decision — defaulting to keep", uid)
			decision.TaskActions = append(decision.TaskActions, taskAction{NodeUID: uid, Action: "keep"})
		}
	}

	// Always restructure: send per-task actions to proxy in a single command.
	meta.Status = "dispatching"
	p.saveMeetingMeta(meta)

	capturedDecision := *decision
	capturedMeta := *meta
	p.asyncLLM(func() (func(), error) {
		// Send strategyApplyDecision to proxy — proxy handles keep/reset_and_retry/cancel per task.
		if _, err := p.sendProxyCommand(httpserver.ActionStrategyApplyDecision, map[string]any{
			"taskActions": capturedDecision.TaskActions,
		}); err != nil {
			log.Printf("[strategy] Failed to send applyDecision: %v", err)
		}
		return func() {
			p.finalizeMeeting(&capturedMeta, &capturedDecision)
		}, nil
	})
}

// finalizeMeeting transitions the meeting to idle and persists the decision.
func (p *eventProcessor) finalizeMeeting(meta *meetingMeta, decision *strategyDecision) {
	m := p.getMeetingMeta()
	if m == nil {
		return
	}
	m.Status = "idle"
	m.Decision = "restructure"
	m.DecisionReasoning = decision.Reasoning
	p.saveMeetingMeta(m)
	log.Printf("[strategy] Meeting %s resolved", m.SessionID)
}

// ── Tree Lookup Helpers ─────────────────────────────────────

// buildNodeUIDMap builds a nodeUID→taskID map from all tracks in one pass.
func (p *eventProcessor) buildNodeUIDMap() map[string]string {
	m := make(map[string]string)
	if p.store == nil {
		return m
	}
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
		for _, n := range nodes {
			node, _ := n.(map[string]any)
			uid, _ := node["uid"].(string)
			taskID, _ := node["taskID"].(string)
			if uid != "" && taskID != "" {
				m[uid] = taskID
			}
		}
	}
	return m
}

func (p *eventProcessor) findParentBranch(nodeUID string) string {
	if p.store == nil {
		return ""
	}
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
		nodeByID := make(map[string]map[string]any)
		var targetNode map[string]any
		for _, n := range nodes {
			node, _ := n.(map[string]any)
			id, _ := node["id"].(string)
			nodeByID[id] = node
			if uid, _ := node["uid"].(string); uid == nodeUID {
				targetNode = node
			}
		}
		if targetNode == nil {
			continue
		}
		parentID, _ := targetNode["parentId"].(string)
		if parent, ok := nodeByID[parentID]; ok {
			if parentUID, _ := parent["uid"].(string); parentUID != "" {
				return "dev/" + parentUID
			}
		}
	}
	return "main"
}
