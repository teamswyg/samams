// Package orchestration provides shared functions for task dispatch, history generation,
// and milestone management. Used by both local server (run_handler.go) and Lambda handlers.
package orchestration

import (
	"context"
	"fmt"
	"strings"

	"server/internal/domain/llm"
	prompt_pkg "server/infra/out/llm/prompt"
)

// Planner generates milestoneProposals and frontiers via LLM.
type Planner interface {
	Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)
}

// TreeNode is a simplified node for orchestration logic (shared between local and Lambda).
type TreeNode struct {
	ID             string  `json:"id"`
	UID            string  `json:"uid"`
	Type           string  `json:"type"`
	Summary        string  `json:"summary"`
	Status         string  `json:"status"`
	Priority       string  `json:"priority"`
	ParentID       *string `json:"parentId"`
	BoundedContext string  `json:"boundedContext"`
}

// SkeletonPayload is the JSON sent to the proxy for deterministic skeleton creation.
type SkeletonPayload struct {
	Module      map[string]string   `json:"module"`
	Files       []map[string]string `json:"files"`
	ProjectName string              `json:"projectName"`
	ProjectGoal string              `json:"projectGoal"`
}

// PlanSpec holds plan data for skeleton/history generation.
type PlanSpec struct {
	Title       string
	Goal        string
	Description string
	TechSpec    map[string]any
}

// ── Skeleton ────────────────────────────────────────────────

// BuildSkeletonFromPlan constructs a SkeletonPayload from plan data.
// Returns nil if no folder structure is available.
func BuildSkeletonFromPlan(plan *PlanSpec, projectName string) *SkeletonPayload {
	if plan == nil {
		return nil
	}

	fsRaw, ok := plan.TechSpec["folderStructure"]
	if !ok {
		return nil
	}
	fsText, ok := fsRaw.(string)
	if !ok || strings.TrimSpace(fsText) == "" {
		return nil
	}

	moduleType := "go"
	moduleName := projectName
	if ts, ok := plan.TechSpec["techStack"].(string); ok {
		lower := strings.ToLower(ts)
		if strings.Contains(lower, "react") || strings.Contains(lower, "node") || strings.Contains(lower, "typescript") {
			moduleType = "node"
		} else if strings.Contains(lower, "python") || strings.Contains(lower, "django") || strings.Contains(lower, "flask") {
			moduleType = "python"
		}
	}

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
		cleaned := strings.NewReplacer("├─", "  ", "└─", "  ", "│", " ", "├", " ", "└", " ", "─", " ").Replace(line)
		stripped := strings.TrimLeft(cleaned, " \t")
		if stripped == "" || strings.HasPrefix(stripped, "#") || strings.HasPrefix(stripped, "//") {
			continue
		}
		depth := len(cleaned) - len(stripped)

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

		for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
			stack = stack[:len(stack)-1]
		}

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

		if !isDir {
			files = append(files, map[string]string{"path": fullPath.String(), "purpose": purpose})
		}
	}

	if len(files) == 0 {
		return nil
	}

	return &SkeletonPayload{
		Module:      map[string]string{"type": moduleType, "name": moduleName},
		Files:       files,
		ProjectName: projectName,
		ProjectGoal: plan.Goal,
	}
}

// ── Milestone Proposal ──────────────────────────────────────

// GenerateMilestoneProposal generates a detailed milestone specification via LLM.
func GenerateMilestoneProposal(ctx context.Context, planner Planner, proposal, milestoneSummary string, taskSummaries []string) (string, error) {
	tasksText := strings.Join(taskSummaries, "\n- ")
	userPrompt := fmt.Sprintf(`Based on the following project proposal and milestone summary, generate a detailed milestone specification.

## Project Proposal
%s

## Milestone Summary
%s

## Child Tasks
- %s

Generate a comprehensive milestone specification that includes:
1. Milestone objective and scope
2. Technical approach and architecture decisions
3. Key deliverables
4. Dependencies and constraints
5. Success criteria

Output plain text with markdown headers. Be specific and actionable.`, proposal, milestoneSummary, tasksText)

	resp, err := planner.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: "You are a technical architect generating detailed milestone specifications for a multi-agent software development system.",
		UserPrompt:   userPrompt,
		MaxTokens:    4096,
		Temperature:  0.3,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ── Task Frontier ───────────────────────────────────────────

// GenerateTaskFrontier generates a DDD bounded-context atomic frontier command via LLM.
func GenerateTaskFrontier(ctx context.Context, planner Planner, proposal, milestoneProposal, taskSummary string, siblingTasks []string) (string, error) {
	siblingsText := "None"
	if len(siblingTasks) > 0 {
		siblingsText = strings.Join(siblingTasks, "\n- ")
	}
	userPrompt := fmt.Sprintf(`## Project Proposal
%s

## Milestone Specification
%s

## Task to Generate Frontier For
%s

## Sibling Tasks (DO NOT touch these — isolation boundary)
- %s

Generate a DDD Bounded Context-level atomic frontier command for this task.`, proposal, milestoneProposal, taskSummary, siblingsText)

	resp, err := planner.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt_pkg.FrontierCommand,
		UserPrompt:   userPrompt,
		MaxTokens:    4096,
		Temperature:  0.3,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ── Milestone Completion ────────────────────────────────────

// CheckMilestoneCompletion checks if all tasks under a milestone are complete.
// Returns milestoneUID and true if all complete, empty and false otherwise.
func CheckMilestoneCompletion(nodes []map[string]any, completedTaskUID string) (milestoneUID string, allComplete bool) {
	// Find the completed task node.
	var taskNode map[string]any
	for _, node := range nodes {
		if uid, _ := node["uid"].(string); uid == completedTaskUID {
			taskNode = node
			break
		}
	}
	if taskNode == nil {
		return "", false
	}

	parentID, _ := taskNode["parentId"].(string)
	if parentID == "" {
		return "", false
	}

	// Find parent milestone.
	var milestoneNode map[string]any
	for _, node := range nodes {
		id, _ := node["id"].(string)
		nType, _ := node["type"].(string)
		if id == parentID && nType == "milestone" {
			milestoneNode = node
			break
		}
	}
	if milestoneNode == nil {
		return "", false
	}

	milestoneStatus, _ := milestoneNode["status"].(string)
	if milestoneStatus == "reviewing" || milestoneStatus == "complete" {
		return "", false
	}

	milestoneID, _ := milestoneNode["id"].(string)
	milestoneUID, _ = milestoneNode["uid"].(string)

	allComplete = true
	for _, node := range nodes {
		pid, _ := node["parentId"].(string)
		nType, _ := node["type"].(string)
		if pid == milestoneID && nType == "task" {
			status, _ := node["status"].(string)
			if status != "complete" {
				allComplete = false
				break
			}
		}
	}

	if !allComplete {
		return "", false
	}
	return milestoneUID, true
}

// ── Review Prompt ───────────────────────────────────────────

// BuildReviewPrompt builds the prompt for a milestone code review agent.
func BuildReviewPrompt(nodeType, nodeSummary string, childSummaries []string) string {
	var sb strings.Builder
	sb.WriteString(prompt_pkg.MilestoneCodeReview)
	sb.WriteString("\n\n---\n\n")
	if nodeType == "proposal" {
		sb.WriteString("## Proposal (Project-Level) Review\n")
		sb.WriteString(nodeSummary + "\n\n")
		sb.WriteString("## Completed Milestone Summaries\n")
	} else {
		sb.WriteString("## Milestone Specification\n")
		sb.WriteString(nodeSummary + "\n\n")
		sb.WriteString("## Completed Task Summaries\n")
	}
	for i, s := range childSummaries {
		sb.WriteString(fmt.Sprintf("### %d\n%s\n\n", i+1, s))
	}
	sb.WriteString("\n## YOUR TASK\nReview ALL code in this worktree following the checklist above. Output your review in the specified format.\n")
	return sb.String()
}

// ContextMdInstructions is the suffix appended to every task prompt.
const ContextMdInstructions = `

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
