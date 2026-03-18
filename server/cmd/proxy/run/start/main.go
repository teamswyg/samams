// Run-start Lambda (deploy mode). Handles proposal → pending → dispatch flow.
//
// Flow:
//   1. If proposal node exists: dispatch proposal only, save tasks as pending in S3
//   2. If no proposal: dispatch all task nodes directly
//
// When the proposal setup agent completes, task-completed Lambda detects
// nodeType="proposal" and dispatches the pending tasks.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	s3store "server/infra/out/proxystore"
	"server/internal/app/orchestration"
)

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

type dispatched struct {
	NodeID    string `json:"nodeId"`
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	userID := req.RequestContext.Authorizer["principalId"]
	if userID == nil || userID.(string) == "" {
		return respond(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var body struct {
		Nodes       []treeNode     `json:"nodes"`
		MaxAgents   int            `json:"max_agents"`
		ProjectName string         `json:"project_name"`
		Plan        *planSpec      `json:"plan"`
	}
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if len(body.Nodes) == 0 {
		return respond(http.StatusBadRequest, map[string]string{"error": "nodes are required"})
	}

	maxAgents := body.MaxAgents
	if maxAgents <= 0 || maxAgents > 6 {
		maxAgents = 6
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "aws config"})
	}

	store := s3store.New(s3.NewFromConfig(cfg))
	uid := userID.(string)

	// Build node lookup for branch resolution.
	nodeByID := make(map[string]treeNode, len(body.Nodes))
	for _, n := range body.Nodes {
		nodeByID[n.ID] = n
	}

	var results []dispatched
	var errors []string

	// Check if proposal node exists.
	var proposalNode *treeNode
	var taskNodes []treeNode
	for i, n := range body.Nodes {
		switch n.Type {
		case "proposal":
			proposalNode = &body.Nodes[i]
		case "task":
			taskNodes = append(taskNodes, n)
		}
	}

	if proposalNode != nil {
		// Phase 0: Save pending tasks to S3, dispatch proposal.
		runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())

		var pendingNodeBytes []json.RawMessage
		for _, n := range taskNodes {
			b, _ := json.Marshal(n)
			pendingNodeBytes = append(pendingNodeBytes, b)
		}
		pendingRun := s3store.PendingRun{
			Nodes:       pendingNodeBytes,
			MaxAgents:   maxAgents,
			ProjectName: body.ProjectName,
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		}
		if err := store.SavePendingRun(ctx, uid, runID, pendingRun); err != nil {
			log.Printf("[run-start] Failed to save pending run: %v", err)
		} else {
			log.Printf("[run-start] Saved %d pending tasks (runID: %s)", len(taskNodes), runID)
		}

		// Try skeleton creation (proxy-side, no agent).
		skeleton := orchestration.BuildSkeletonFromPlan(&orchestration.PlanSpec{
			Title: body.Plan.Title, Goal: body.Plan.Goal,
			Description: body.Plan.Description, TechSpec: body.Plan.TechSpec,
		}, body.ProjectName)

		// Also check structuredSkeleton from plan.
		if skeleton == nil && body.Plan != nil && body.Plan.StructuredSkeleton != nil && len(body.Plan.StructuredSkeleton.Files) > 0 {
			files := make([]map[string]string, 0, len(body.Plan.StructuredSkeleton.Files))
			for _, f := range body.Plan.StructuredSkeleton.Files {
				files = append(files, map[string]string{"path": f.Path, "purpose": f.Purpose})
			}
			skeleton = &orchestration.SkeletonPayload{
				Module:      map[string]string{"type": body.Plan.StructuredSkeleton.Module.Type, "name": body.Plan.StructuredSkeleton.Module.Name},
				Files:       files,
				ProjectName: body.ProjectName,
				ProjectGoal: body.Plan.Goal,
			}
		}

		if skeleton != nil && len(skeleton.Files) > 0 {
			// Skeleton path: send createSkeleton command.
			cmdID, err := saveCommand(ctx, store, uid, proposalNode.UID, "createSkeleton", skeleton)
			if err != nil {
				log.Printf("[run-start] Skeleton command save failed: %v — falling back to agent", err)
			} else {
				results = append(results, dispatched{NodeID: proposalNode.ID, CommandID: cmdID, Status: "skeleton"})
				log.Printf("[run-start] Skeleton command saved for %s", proposalNode.UID)
				// Note: history generation + task dispatch happens in task-completed Lambda
				// when it detects nodeType="proposal" completion.
			}
		}

		if len(results) == 0 {
			// Fallback: agent-based setup.
			setupPrompt := buildSetupPrompt(proposalNode.Summary, body.Nodes, body.Plan)
			payload := createTaskPayload{
				Name:         proposalNode.Summary,
				Prompt:       setupPrompt,
				NumAgents:    1,
				Tags:         []string{proposalNode.UID, "setup"},
				AgentType:    mapAgent(proposalNode.Agent),
				Mode:         "execute",
				NodeType:     "proposal",
				NodeUID:      proposalNode.UID,
				ParentBranch: "main",
				ProjectName:  body.ProjectName,
			}
			cmdID, err := saveCommand(ctx, store, uid, proposalNode.UID, "createTask", payload)
			if err != nil {
				errors = append(errors, proposalNode.UID+": "+err.Error())
			} else {
				results = append(results, dispatched{NodeID: proposalNode.ID, CommandID: cmdID, Status: "pending"})
				log.Printf("[run-start] Proposal setup dispatched: %s (pending: %d tasks)", proposalNode.UID, len(taskNodes))
			}
		}
	} else {
		// No proposal — dispatch all tasks directly.
		dispatchCount := 0
		for _, n := range taskNodes {
			if dispatchCount >= maxAgents {
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
			if h, err := store.LoadTaskHistory(ctx, uid, "", n.UID); err == nil {
				if f, ok := h["frontier"].(string); ok && f != "" {
					taskPrompt = f
				}
			}
			taskPrompt += orchestration.ContextMdInstructions

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
				ProjectName:      body.ProjectName,
			}
			cmdID, err := saveCommand(ctx, store, uid, n.UID, "createTask", payload)
			if err != nil {
				errors = append(errors, n.UID+": "+err.Error())
				continue
			}
			results = append(results, dispatched{NodeID: n.ID, CommandID: cmdID, Status: "pending"})
			dispatchCount++
		}
	}

	return respond(http.StatusOK, map[string]any{
		"dispatched": results,
		"errors":     errors,
		"total":      len(results),
	})
}

func saveCommand(ctx context.Context, store *s3store.Store, userID, nodeUID, action string, payload any) (string, error) {
	payloadBytes, _ := json.Marshal(payload)
	cmdID := fmt.Sprintf("cmd-%s-%d", nodeUID, time.Now().UnixMilli())
	cmd := s3store.PendingCommand{
		ID:        cmdID,
		Action:    action,
		Payload:   payloadBytes,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return cmdID, store.SaveCommand(ctx, userID, cmd)
}

type planSpec struct {
	Title               string         `json:"title"`
	Goal                string         `json:"goal"`
	Description         string         `json:"description"`
	TechSpec            map[string]any `json:"techSpec"`
	AbstractSpec        map[string]any `json:"abstractSpec"`
	StructuredSkeleton  *skeletonSpec  `json:"structuredSkeleton,omitempty"`
}

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

	if plan != nil && len(plan.TechSpec) > 0 {
		sb.WriteString("## Technical Specification\n")
		for key, val := range plan.TechSpec {
			if s, ok := val.(string); ok && s != "" {
				sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", key, s))
			}
		}
	}

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

func respond(status int, body any) (events.APIGatewayProxyResponse, error) {
	b, _ := json.Marshal(body)
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}, nil
}

func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

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

func main() {
	lambda.Start(handler)
}
