// Milestone review completed Lambda (deploy mode).
// Invoked by events/push Lambda when milestone.review.completed event arrives.
// Calls Claude to analyze review → APPROVED or NEEDS_WORK.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"server/infra/out/llm/anthropic"
	prompt_pkg "server/infra/out/llm/prompt"
	"server/infra/out/proxystore"
	"server/internal/app/orchestration"
	"server/internal/domain/llm"
)

type reviewPayload struct {
	UserID      string `json:"userID"`
	NodeUID     string `json:"nodeUid"`
	Context     string `json:"context"`
	ProjectName string `json:"projectName"`
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var payload reviewPayload
	if err := json.Unmarshal([]byte(req.Body), &payload); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if payload.UserID == "" || payload.NodeUID == "" {
		return respond(http.StatusBadRequest, map[string]string{"error": "userID and nodeUid required"})
	}

	log.Printf("[review] Processing review for milestone %s", payload.NodeUID)

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "aws config"})
	}
	store := proxystore.New(s3.NewFromConfig(awsCfg))
	planID := sanitizeProjectName(payload.ProjectName)

	// Call Claude for APPROVED/NEEDS_WORK decision.
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		log.Printf("[review] No ANTHROPIC_API_KEY — auto-approving milestone %s", payload.NodeUID)
		return autoApprove(ctx, store, payload, planID)
	}

	planner := anthropic.NewClient(anthropicKey, os.Getenv("ANTHROPIC_MODEL"))
	result, err := planner.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: prompt_pkg.ReviewAnalysis,
		UserPrompt:   "## Milestone UID: " + payload.NodeUID + "\n\n## Review Agent Findings\n" + payload.Context,
		MaxTokens:    8192,
		Temperature:  0.3,
	})
	if err != nil {
		log.Printf("[review] Claude analysis failed: %v — auto-approving", err)
		return autoApprove(ctx, store, payload, planID)
	}

	// Parse Claude response.
	var decision struct {
		Decision  string `json:"decision"`
		Reasoning string `json:"reasoning"`
		NewTasks  []struct {
			Summary        string `json:"summary"`
			Detail         string `json:"detail"`
			ParentUID      string `json:"parentUid"`
			Relationship   string `json:"relationship"`
			Priority       string `json:"priority"`
			Reason         string `json:"reason"`
			BoundedContext string `json:"boundedContext"`
		} `json:"newTasks"`
	}
	cleaned := strings.TrimSpace(result.Content)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)
	if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
		log.Printf("[review] Failed to parse Claude response: %v — auto-approving", err)
		return autoApprove(ctx, store, payload, planID)
	}

	log.Printf("[review] Milestone %s decision: %s — %s", payload.NodeUID, decision.Decision, decision.Reasoning)

	if decision.Decision == "APPROVED" || len(decision.NewTasks) == 0 {
		// Merge milestone → main.
		mergeCmdID := fmt.Sprintf("cmd-merge-%s-%d", payload.NodeUID, time.Now().UnixMilli())
		mergePayload, _ := json.Marshal(map[string]string{
			"branchName":   "dev/" + payload.NodeUID,
			"targetBranch": "main",
		})
		cmd := proxystore.PendingCommand{
			ID: mergeCmdID, Action: "mergeMilestone",
			Payload: mergePayload, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		_ = store.SaveCommand(ctx, payload.UserID, cmd)

		// Update tree.json.
		updateTreeNodeStatus(ctx, store, payload.UserID, planID, payload.NodeUID, "complete", "APPROVED: "+decision.Reasoning)

		return respond(http.StatusOK, map[string]any{"decision": "APPROVED", "milestone": payload.NodeUID})
	}

	// NEEDS_WORK: generate frontiers for new tasks → save commands.
	log.Printf("[review] Milestone %s needs work — %d new tasks", payload.NodeUID, len(decision.NewTasks))

	for i, nt := range decision.NewTasks {
		uid := fmt.Sprintf("RVTASK-%s-%d-%d", payload.NodeUID, time.Now().UnixMilli(), i)

		// Generate frontier for review task.
		frontier := nt.Summary
		if nt.Detail != "" {
			frontier = nt.Detail
		}

		milestoneProposal := ""
		if mh, err := store.LoadMilestoneHistory(ctx, payload.UserID, planID, payload.NodeUID); err == nil {
			milestoneProposal, _ = mh["milestoneProposal"].(string)
		}

		if f, err := orchestration.GenerateTaskFrontier(ctx, planner, "", milestoneProposal, nt.Summary, nil); err == nil {
			frontier = f
		}

		// Save task history.
		_ = store.SaveTaskHistory(ctx, payload.UserID, planID, uid, map[string]any{
			"milestoneProposal": milestoneProposal,
			"frontier":          frontier,
			"uid":               uid,
			"origin":            "review",
		})

		// Save createTask command.
		taskPayload, _ := json.Marshal(map[string]any{
			"name": nt.Summary, "prompt": frontier + orchestration.ContextMdInstructions,
			"numAgents": 1, "tags": []string{uid, nt.Priority, "review"},
			"boundedContextId": nt.BoundedContext,
			"agentType": "cursor", "mode": "execute",
			"nodeType": "task", "nodeUid": uid,
			"parentBranch": "dev/" + payload.NodeUID,
			"projectName":  payload.ProjectName,
		})
		cmdID := fmt.Sprintf("cmd-rvtask-%s-%d", uid, time.Now().UnixNano()%10000)
		_ = store.SaveCommand(ctx, payload.UserID, proxystore.PendingCommand{
			ID: cmdID, Action: "createTask", Payload: taskPayload,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	}

	updateTreeNodeStatus(ctx, store, payload.UserID, planID, payload.NodeUID, "running", "NEEDS_WORK: "+decision.Reasoning)

	return respond(http.StatusOK, map[string]any{"decision": "NEEDS_WORK", "newTasks": len(decision.NewTasks)})
}

func autoApprove(ctx context.Context, store *proxystore.Store, payload reviewPayload, planID string) (events.APIGatewayProxyResponse, error) {
	mergeCmdID := fmt.Sprintf("cmd-merge-%s-%d", payload.NodeUID, time.Now().UnixMilli())
	mergePayload, _ := json.Marshal(map[string]string{
		"branchName": "dev/" + payload.NodeUID, "targetBranch": "main",
	})
	_ = store.SaveCommand(ctx, payload.UserID, proxystore.PendingCommand{
		ID: mergeCmdID, Action: "mergeMilestone", Payload: mergePayload,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	updateTreeNodeStatus(ctx, store, payload.UserID, planID, payload.NodeUID, "complete", "auto-approved")
	return respond(http.StatusOK, map[string]any{"decision": "APPROVED", "reason": "auto-approved"})
}

func updateTreeNodeStatus(ctx context.Context, store *proxystore.Store, userID, planID, nodeUID, status, reviewDecision string) {
	track, err := store.LoadTrack(ctx, userID, planID)
	if err != nil {
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
			node["reviewDecision"] = reviewDecision
			node["updatedAt"] = time.Now().UnixMilli()
			nodes[i] = node
			break
		}
	}
	track["nodes"] = nodes
	_ = store.SaveTrack(ctx, userID, planID, track)
}

func sanitizeProjectName(name string) string {
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

func respond(status int, body any) (events.APIGatewayProxyResponse, error) {
	b, _ := json.Marshal(body)
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}, nil
}

func main() {
	lambda.Start(handler)
}
