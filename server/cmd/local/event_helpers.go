// Event processor helper methods — track logging, history building, milestone checks.
// These were previously on localEventRouter; now on eventProcessor.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	prompt_pkg "server/infra/out/llm/prompt"
)

// getNodeTypeFromTree looks up a node's type from tree.json by UID.
// Returns "milestone" as default if not found.
func (p *eventProcessor) getNodeTypeFromTree(nodeUID string) string {
	if p.store == nil {
		return "milestone"
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
		nodes, ok := track["nodes"].([]any)
		if !ok {
			continue
		}
		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			if uid, _ := node["uid"].(string); uid == nodeUID {
				if nType, _ := node["type"].(string); nType != "" {
					return nType
				}
				return "milestone"
			}
		}
	}
	return "milestone"
}

// appendTrackLog saves a log entry to tracks/{planId}/logs/{nodeUid}.json.
func (p *eventProcessor) appendTrackLog(taskID string, entry map[string]any) {
	if p.store == nil || taskID == "" {
		return
	}
	entry["timestamp"] = time.Now().UnixMilli()

	keys, err := p.store.List("tracks")
	if err != nil || len(keys) == 0 {
		return
	}
	for _, k := range keys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		trackDir := strings.TrimSuffix(k, "/tree.json")
		logKey := trackDir + "/logs/" + taskID + ".json"

		var logs []any
		_ = p.store.Get(logKey, &logs)
		logs = append(logs, entry)
		if len(logs) > 500 {
			logs = logs[len(logs)-500:]
		}
		_ = p.store.Put(logKey, logs)
		return
	}
}

// updateTrackNode updates a node in the tracked tree by nodeUid or taskID.
func (p *eventProcessor) updateTrackNode(nodeUID, taskID, status string, extra map[string]any) {
	if p.store == nil {
		return
	}
	keys, err := p.store.List("tracks")
	if err != nil {
		return
	}
	for _, k := range keys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		var track map[string]any
		if err := p.store.Get(k, &track); err != nil {
			continue
		}
		nodes, ok := track["nodes"].([]any)
		if !ok {
			continue
		}
		updated := false
		for i, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			uid, _ := node["uid"].(string)
			tid, _ := node["taskId"].(string)
			if (nodeUID != "" && uid == nodeUID) || (taskID != "" && tid == taskID) {
				if status != "" {
					node["status"] = status
				}
				for k, v := range extra {
					node[k] = v
				}
				nodes[i] = node
				updated = true
				break
			}
		}
		if updated {
			_ = p.store.Put(k, track)
		}
		return
	}
}

// buildAndSaveHistory builds the vertical history for a node.
func (p *eventProcessor) buildAndSaveHistory(nodeUID, nodeType string) {
	if p.store == nil {
		return
	}

	keys, _ := p.store.List("tracks")
	for _, k := range keys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		trackDir := strings.TrimSuffix(k, "/tree.json")

		var track map[string]any
		if err := p.store.Get(k, &track); err != nil {
			continue
		}
		nodes, ok := track["nodes"].([]any)
		if !ok {
			continue
		}

		nodeMap := make(map[string]map[string]any)
		for _, n := range nodes {
			if node, ok := n.(map[string]any); ok {
				if uid, ok := node["uid"].(string); ok {
					nodeMap[uid] = node
				}
			}
		}

		thisNode, ok := nodeMap[nodeUID]
		if !ok {
			return
		}

		projectName, _ := track["projectName"].(string)
		var proposal json.RawMessage
		planKeys, _ := p.store.List("plans")
		for _, pk := range planKeys {
			if strings.Contains(pk, "/trees/") || !strings.HasSuffix(pk, ".json") {
				continue
			}
			var doc map[string]any
			if err := p.store.Get(pk, &doc); err == nil {
				title, _ := doc["title"].(string)
				if title == projectName || len(planKeys) == 1 {
					b, _ := json.Marshal(doc)
					proposal = b
					break
				}
			}
		}

		historiesBase := trackDir + "/histories"

		var proposalClean json.RawMessage
		if proposal != nil {
			var doc map[string]any
			if json.Unmarshal(proposal, &doc) == nil {
				delete(doc, "features")
				b, _ := json.Marshal(doc)
				proposalClean = b
			} else {
				proposalClean = proposal
			}
		}

		switch nodeType {
		case "proposal":
			if proposalClean != nil {
				_ = p.store.Put(historiesBase+"/proposal.json", json.RawMessage(proposalClean))
			}

		case "milestone":
			milestoneID, _ := thisNode["id"].(string)
			var childTasks []map[string]any
			for _, n := range nodes {
				node, ok := n.(map[string]any)
				if !ok {
					continue
				}
				pid, _ := node["parentId"].(string)
				nType, _ := node["type"].(string)
				if pid == milestoneID && nType == "task" {
					childTasks = append(childTasks, map[string]any{
						"uid": node["uid"], "summary": node["summary"],
						"status": node["status"], "frontier": node["frontier"],
						"completedAt": node["completedAt"],
					})
				}
			}
			milestone := map[string]any{
				"proposal":          json.RawMessage(proposalClean),
				"milestoneProposal": thisNode["summary"],
				"uid":               nodeUID,
				"status":            thisNode["status"],
				"reviewDecision":    thisNode["reviewDecision"],
				"tasks":             childTasks,
			}
			_ = p.store.Put(historiesBase+"/milestones/"+nodeUID+".json", milestone)
			log.Printf("[event-loop] Saved milestone history for %s (%d tasks)", nodeUID, len(childTasks))

		case "task":
			parentID, _ := thisNode["parentId"].(string)
			var milestoneNode map[string]any
			var milestoneUID string
			for _, n := range nodes {
				node, ok := n.(map[string]any)
				if !ok {
					continue
				}
				if id, _ := node["id"].(string); id == parentID {
					milestoneUID, _ = node["uid"].(string)
					milestoneNode = node
					break
				}
			}

			var milestoneClean map[string]any
			if milestoneNode != nil {
				milestoneClean = make(map[string]any, len(milestoneNode))
				for mk, mv := range milestoneNode {
					if mk != "features" {
						milestoneClean[mk] = mv
					}
				}
			}

			var summaries []string
			if milestoneNode != nil {
				if s, ok := milestoneNode["summary"].(string); ok && s != "" {
					summaries = append(summaries, s)
				}
			}

			// Preserve existing frontier and milestoneProposal from initial save
			// (LLM-generated values that would be lost if overwritten with tree.json data).
			frontier := thisNode["frontier"]
			milestoneProposal := ""
			var existingHistory map[string]any
			if err := p.store.Get(historiesBase+"/tasks/"+nodeUID+".json", &existingHistory); err == nil {
				if ef, ok := existingHistory["frontier"].(string); ok && ef != "" {
					if f, _ := frontier.(string); f == "" {
						frontier = ef
					}
				}
				if mp, ok := existingHistory["milestoneProposal"].(string); ok && mp != "" {
					milestoneProposal = mp
				}
			}

			history := map[string]any{
				"proposal":          json.RawMessage(proposalClean),
				"milestone":         milestoneClean,
				"milestoneProposal": milestoneProposal,
				"summaries":         summaries,
				"frontier":          frontier,
				"nodeUid":           nodeUID,
				"status":            thisNode["status"],
			}

			_ = p.store.Put(historiesBase+"/tasks/"+nodeUID+".json", history)
			log.Printf("[event-loop] Saved task history for %s", nodeUID)

			if milestoneUID != "" {
				p.updateMilestoneHistoryTask(milestoneUID, thisNode)
			}
		}
		return
	}
}

// initMilestoneHistories creates initial history for each milestone after proposal completes.
func (p *eventProcessor) initMilestoneHistories(proposalUID string) {
	if p.store == nil {
		return
	}
	keys, _ := p.store.List("tracks")
	for _, k := range keys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		trackDir := strings.TrimSuffix(k, "/tree.json")
		var track map[string]any
		if err := p.store.Get(k, &track); err != nil {
			continue
		}
		nodes, ok := track["nodes"].([]any)
		if !ok {
			continue
		}

		projectName, _ := track["projectName"].(string)
		var proposalClean json.RawMessage
		planKeys, _ := p.store.List("plans")
		for _, pk := range planKeys {
			if strings.Contains(pk, "/trees/") || !strings.HasSuffix(pk, ".json") {
				continue
			}
			var doc map[string]any
			if err := p.store.Get(pk, &doc); err == nil {
				title, _ := doc["title"].(string)
				if title == projectName || len(planKeys) == 1 {
					delete(doc, "features")
					b, _ := json.Marshal(doc)
					proposalClean = b
					break
				}
			}
		}

		historiesBase := trackDir + "/histories"
		if proposalClean != nil {
			_ = p.store.Put(historiesBase+"/proposal.json", json.RawMessage(proposalClean))
		}

		var proposalID string
		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			if uid, _ := node["uid"].(string); uid == proposalUID {
				proposalID, _ = node["id"].(string)
				break
			}
		}

		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			pid, _ := node["parentId"].(string)
			nType, _ := node["type"].(string)
			if pid == proposalID && nType == "milestone" {
				milestoneUID, _ := node["uid"].(string)
				milestoneNodeID, _ := node["id"].(string)
				var taskSpecs []map[string]any
				for _, tn := range nodes {
					tNode, ok := tn.(map[string]any)
					if !ok {
						continue
					}
					tPid, _ := tNode["parentId"].(string)
					tType, _ := tNode["type"].(string)
					if tPid == milestoneNodeID && tType == "task" {
						taskSpecs = append(taskSpecs, map[string]any{
							"uid": tNode["uid"], "summary": tNode["summary"], "status": tNode["status"],
						})
					}
				}
				milestone := map[string]any{
					"proposal":          json.RawMessage(proposalClean),
					"milestoneProposal": node["summary"],
					"uid":               milestoneUID,
					"status":            node["status"],
					"tasks":             taskSpecs,
				}
				_ = p.store.Put(historiesBase+"/milestones/"+milestoneUID+".json", milestone)
				slog.Info("[event-loop] milestone history initialized", "milestone", milestoneUID, "tasks", len(taskSpecs))
			}
		}
		return
	}
}

// updateMilestoneHistoryTask updates a milestone's history when a child task completes.
func (p *eventProcessor) updateMilestoneHistoryTask(milestoneUID string, taskNode map[string]any) {
	if p.store == nil {
		return
	}
	keys, _ := p.store.List("tracks")
	for _, k := range keys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		trackDir := strings.TrimSuffix(k, "/tree.json")
		historyKey := trackDir + "/histories/milestones/" + milestoneUID + ".json"

		var milestone map[string]any
		if err := p.store.Get(historyKey, &milestone); err != nil {
			return
		}

		tasks, _ := milestone["tasks"].([]any)
		taskUID, _ := taskNode["uid"].(string)
		updated := false
		for i, t := range tasks {
			entry, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if uid, _ := entry["uid"].(string); uid == taskUID {
				entry["summary"] = taskNode["summary"]
				entry["status"] = taskNode["status"]
				entry["frontier"] = taskNode["frontier"]
				entry["completedAt"] = taskNode["completedAt"]
				tasks[i] = entry
				updated = true
				break
			}
		}
		if !updated {
			tasks = append(tasks, map[string]any{
				"uid": taskUID, "summary": taskNode["summary"],
				"status": taskNode["status"], "frontier": taskNode["frontier"],
				"completedAt": taskNode["completedAt"],
			})
		}
		milestone["tasks"] = tasks
		_ = p.store.Put(historyKey, milestone)
		slog.Info("[event-loop] milestone history updated", "milestone", milestoneUID, "task", taskUID)
		return
	}
}

// completeNode marks a node as complete and builds its history.
func (p *eventProcessor) completeNode(nodeUID, nodeType string, extra map[string]any) {
	if extra == nil {
		extra = map[string]any{}
	}
	extra["completedAt"] = time.Now().UnixMilli()
	p.updateTrackNode(nodeUID, "", "complete", extra)
	p.buildAndSaveHistory(nodeUID, nodeType)
	slog.Info("[event-loop] node completed", "uid", nodeUID, "type", nodeType)
}

// checkMilestoneCompletion checks if all tasks under a milestone are complete.
func (p *eventProcessor) checkMilestoneCompletion(taskNodeUID, projectName string) {
	if p.store == nil || p.runHandler == nil {
		log.Printf("[event-loop] checkMilestoneCompletion: skip (store=%v runHandler=%v)", p.store != nil, p.runHandler != nil)
		return
	}
	log.Printf("[event-loop] checkMilestoneCompletion: taskNodeUID=%s projectName=%s", taskNodeUID, projectName)

	keys, _ := p.store.List("tracks")
	for _, k := range keys {
		if !strings.HasSuffix(k, "/tree.json") {
			continue
		}
		var track map[string]any
		if err := p.store.Get(k, &track); err != nil {
			log.Printf("[event-loop] checkMilestoneCompletion: failed to read %s: %v", k, err)
			continue
		}
		nodes, ok := track["nodes"].([]any)
		if !ok {
			log.Printf("[event-loop] checkMilestoneCompletion: no nodes in %s", k)
			continue
		}

		// Find the completed task node by UID.
		var taskNode, milestoneNode map[string]any
		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			if uid, _ := node["uid"].(string); uid == taskNodeUID {
				taskNode = node
				break
			}
		}
		if taskNode == nil {
			log.Printf("[event-loop] checkMilestoneCompletion: task node %s not found in tree (%d nodes)", taskNodeUID, len(nodes))
			// Log all UIDs for debugging.
			for _, n := range nodes {
				if node, ok := n.(map[string]any); ok {
					uid, _ := node["uid"].(string)
					nType, _ := node["type"].(string)
					status, _ := node["status"].(string)
					log.Printf("[event-loop]   node uid=%s type=%s status=%s", uid, nType, status)
				}
			}
			return
		}

		parentID, _ := taskNode["parentId"].(string)
		if parentID == "" {
			log.Printf("[event-loop] checkMilestoneCompletion: task %s has no parentId", taskNodeUID)
			return
		}

		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			if id, _ := node["id"].(string); id == parentID {
				if nType, _ := node["type"].(string); nType == "milestone" {
					milestoneNode = node
				}
				break
			}
		}
		if milestoneNode == nil {
			log.Printf("[event-loop] checkMilestoneCompletion: parent %s is not a milestone", parentID)
			return
		}

		milestoneStatus, _ := milestoneNode["status"].(string)
		if milestoneStatus == "reviewing" || milestoneStatus == "complete" {
			log.Printf("[event-loop] checkMilestoneCompletion: milestone already %s", milestoneStatus)
			return
		}

		milestoneID, _ := milestoneNode["id"].(string)
		allComplete := true
		taskCount := 0
		completeCount := 0
		errorCount := 0
		var childSummaries []string
		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			pid, _ := node["parentId"].(string)
			nType, _ := node["type"].(string)
			if pid == milestoneID && nType == "task" {
				taskCount++
				status, _ := node["status"].(string)
				uid, _ := node["uid"].(string)
				if status == "complete" {
					completeCount++
				} else if status == "error" {
					errorCount++
					allComplete = false
					log.Printf("[event-loop] checkMilestoneCompletion: task %s status=error (failed)", uid)
				} else {
					allComplete = false
					log.Printf("[event-loop] checkMilestoneCompletion: task %s status=%s (not done)", uid, status)
				}
				if s, _ := node["summary"].(string); s != "" {
					childSummaries = append(childSummaries, s)
				}
			}
		}

		log.Printf("[event-loop] checkMilestoneCompletion: milestone %s — %d/%d tasks complete, %d errors", milestoneID, completeCount, taskCount, errorCount)

		if !allComplete {
			return
		}

		milestoneUID, _ := milestoneNode["uid"].(string)
		milestoneSummary, _ := milestoneNode["summary"].(string)
		log.Printf("[event-loop] All tasks under %s complete — dispatching code review", milestoneUID)

		p.dispatchNodeReview(milestoneUID, milestoneSummary, projectName, childSummaries, "milestone")
		return
	}
}

// checkProposalCompletion checks if all milestones are complete.
func (p *eventProcessor) checkProposalCompletion(milestoneUID, projectName string) {
	if p.store == nil || p.runHandler == nil {
		return
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
		nodes, ok := track["nodes"].([]any)
		if !ok {
			continue
		}

		var milestoneNode, proposalNode map[string]any
		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			if uid, _ := node["uid"].(string); uid == milestoneUID {
				milestoneNode = node
				break
			}
		}
		if milestoneNode == nil {
			return
		}

		parentID, _ := milestoneNode["parentId"].(string)
		if parentID == "" {
			return
		}

		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			if id, _ := node["id"].(string); id == parentID {
				if nType, _ := node["type"].(string); nType == "proposal" {
					proposalNode = node
				}
				break
			}
		}
		if proposalNode == nil {
			return
		}

		proposalStatus, _ := proposalNode["status"].(string)
		if proposalStatus == "reviewing" || proposalStatus == "complete" {
			return
		}

		proposalID, _ := proposalNode["id"].(string)
		allComplete := true
		var childSummaries []string
		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			pid, _ := node["parentId"].(string)
			nType, _ := node["type"].(string)
			if pid == proposalID && nType == "milestone" {
				status, _ := node["status"].(string)
				if status != "complete" {
					allComplete = false
					break
				}
				if s, _ := node["summary"].(string); s != "" {
					childSummaries = append(childSummaries, s)
				}
			}
		}

		if !allComplete {
			return
		}

		proposalUID, _ := proposalNode["uid"].(string)
		proposalSummary, _ := proposalNode["summary"].(string)
		log.Printf("[event-loop] All milestones under %s complete — dispatching proposal-level review", proposalUID)

		p.dispatchNodeReview(proposalUID, proposalSummary, projectName, childSummaries, "proposal")
		return
	}
}

// dispatchNodeReview dispatches a code review agent.
func (p *eventProcessor) dispatchNodeReview(nodeUID, nodeSummary, projectName string, childSummaries []string, nodeType string) {
	reviewCycle := 0
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
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			if uid, _ := node["uid"].(string); uid == nodeUID {
				if rc, ok := node["reviewCycle"].(float64); ok {
					reviewCycle = int(rc)
				}
				break
			}
		}
		break
	}

	if reviewCycle >= 3 {
		log.Printf("[event-loop] %s %s hit max review cycles (3) — auto-approving", nodeType, nodeUID)
		p.completeNode(nodeUID, nodeType, map[string]any{
			"reviewDecision": "auto-approved (max cycles reached)",
		})
		if nodeType == "milestone" && p.runHandler != nil {
			_ = p.runHandler.MergeMilestoneToMain(nodeUID, projectName)
			p.checkProposalCompletion(nodeUID, projectName)
		}
		return
	}

	p.updateTrackNode(nodeUID, "", "reviewing", map[string]any{
		"reviewCycle": reviewCycle + 1,
	})

	var sb strings.Builder
	if nodeType == "proposal" {
		// Proposal review: build + test + full project review on main branch.
		sb.WriteString(prompt_pkg.ProposalCodeReview)
		sb.WriteString("\n\n---\n\n")
		sb.WriteString("## Project Proposal\n")
		sb.WriteString(nodeSummary + "\n\n")
		sb.WriteString("## Completed Milestone Summaries\n")
	} else {
		// Milestone review: read-only code review on milestone branch.
		sb.WriteString(prompt_pkg.MilestoneCodeReview)
		sb.WriteString("\n\n---\n\n")
		sb.WriteString("## Milestone Specification\n")
		sb.WriteString(nodeSummary + "\n\n")
		sb.WriteString("## Completed Task Summaries\n")
	}
	for i, s := range childSummaries {
		sb.WriteString(fmt.Sprintf("### %d\n%s\n\n", i+1, s))
	}
	if nodeType == "proposal" {
		sb.WriteString("\n## YOUR TASK\n1. Run build and test commands FIRST.\n2. Then review ALL code in this worktree.\n3. Output your review in the specified format.\n")
	} else {
		sb.WriteString("\n## YOUR TASK\nReview ALL code in this worktree following the checklist above. Output your review in the specified format.\n")
	}

	reviewBranch := nodeUID
	if nodeType == "proposal" {
		reviewBranch = "main"
	}
	if err := p.runHandler.DispatchReviewTask(reviewBranch, nodeUID, projectName, sb.String()); err != nil {
		log.Printf("[event-loop] Failed to dispatch review for %s: %v — will retry when agent slot opens", nodeUID, err)
		if p.store != nil {
			trackID := localSanitizeTrackID(projectName)
			_ = p.store.Put("tracks/"+trackID+"/pending_reviews/"+nodeUID+".json", map[string]any{
				"nodeUID": nodeUID, "nodeType": nodeType,
				"projectName": projectName, "prompt": sb.String(),
				"reviewBranch": reviewBranch, "failedAt": time.Now().UnixMilli(),
			})
			log.Printf("[event-loop] Saved pending review for %s", nodeUID)
		}
	}
}

func localSanitizeTrackID(name string) string {
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
