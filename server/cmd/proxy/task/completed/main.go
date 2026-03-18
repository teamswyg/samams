// Proxy task-completed Lambda.
// Invoked asynchronously by events/push Lambda when task.completed event is received.
// Calls Gemini Summarizer to generate summary, stores result to S3,
// and creates follow-up commands if needed.

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

	"server/infra/out/llm/gemini"
	"server/infra/out/proxystore"
	"server/internal/app/orchestration"
)

type taskCompletedPayload struct {
	UserID   string `json:"userID"`
	TaskID   string `json:"taskID"`
	AgentID  string `json:"agentID"`
	Context  string `json:"context"`
	Frontier     string `json:"frontier"`
	NodeType     string `json:"nodeType"`
	NodeUID      string `json:"nodeUid"`
	ParentBranch string `json:"parentBranch"`
	ProjectName  string `json:"projectName"`
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Auth: extract userID from authorizer (or from payload for async invoke).
	var payload taskCompletedPayload

	// Support both API Gateway (with authorizer) and direct Lambda invoke.
	if req.Body != "" {
		if err := json.Unmarshal([]byte(req.Body), &payload); err != nil {
			return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
		}
	}

	// If called via API Gateway, override userID from authorizer.
	if req.RequestContext.Authorizer != nil {
		if uid, ok := req.RequestContext.Authorizer["principalId"].(string); ok && uid != "" {
			payload.UserID = uid
		}
	}

	if payload.UserID == "" || payload.TaskID == "" {
		return respond(http.StatusBadRequest, map[string]string{"error": "userID and taskID required"})
	}

	log.Printf("[task-completed] Processing: user=%s task=%s agent=%s", payload.UserID, payload.TaskID, payload.AgentID)

	// Initialize Gemini Summarizer.
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Println("[task-completed] GEMINI_API_KEY not set, storing raw context as summary")
		return storeRawSummary(ctx, payload)
	}

	summarizer := gemini.NewClient(geminiKey, os.Getenv("GEMINI_MODEL"))

	// Generate summary from agent context.
	summary, err := summarizer.Summarize(ctx, payload.Context)
	if err != nil {
		log.Printf("[task-completed] Summarize failed: %v, using raw context", err)
		summary = payload.Context
	}

	log.Printf("[task-completed] Summary generated for task %s (%d chars)", payload.TaskID, len(summary))

	// Store summary to S3.
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "aws config"})
	}

	store := proxystore.New(s3.NewFromConfig(awsCfg))

	result := map[string]string{
		"taskID":    payload.TaskID,
		"agentID":   payload.AgentID,
		"summary":   summary,
		"frontier":  payload.Frontier,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	if err := store.SaveTaskSummary(ctx, payload.UserID, payload.TaskID, result); err != nil {
		log.Printf("[task-completed] SaveTaskSummary error: %v", err)
		return respond(http.StatusInternalServerError, map[string]string{"error": "save summary failed"})
	}

	// Save summary + frontier to tracks + build history file.
	if payload.NodeUID != "" {
		_ = store.AppendTrackLog(ctx, payload.UserID, payload.TaskID, map[string]any{
			"type":     "summary_generated",
			"nodeUid":  payload.NodeUID,
			"summary":  summary,
			"frontier": payload.Frontier,
		})

		// Build vertical history and save to S3 (same tree path as local mode).
		buildAndSaveHistoryS3(ctx, store, payload, summary)
	}

	// If this was a proposal setup task, dispatch pending tasks.
	if payload.NodeType == "proposal" {
		dispatchedCount := dispatchPendingTasks(ctx, store, payload.UserID)
		log.Printf("[task-completed] Proposal complete — dispatched %d pending tasks", dispatchedCount)
	}

	// Check milestone completion (all sibling tasks done → dispatch review).
	if payload.NodeType == "task" && payload.NodeUID != "" {
		planID := sanitizeProjectName(payload.ProjectName)
		checkAndHandleMilestoneCompletion(ctx, store, payload.UserID, planID, payload.NodeUID)
	}

	return respond(http.StatusOK, map[string]any{
		"ok":      true,
		"taskID":  payload.TaskID,
		"summary": fmt.Sprintf("%.100s...", summary),
	})
}

// storeRawSummary stores the raw context as summary when Gemini is not available.
func storeRawSummary(ctx context.Context, payload taskCompletedPayload) (events.APIGatewayProxyResponse, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "aws config"})
	}

	store := proxystore.New(s3.NewFromConfig(awsCfg))

	result := map[string]string{
		"taskID":    payload.TaskID,
		"agentID":   payload.AgentID,
		"summary":   payload.Context,
		"frontier":  payload.Frontier,
		"source":    "raw",
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	if err := store.SaveTaskSummary(ctx, payload.UserID, payload.TaskID, result); err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "save summary failed"})
	}

	return respond(http.StatusOK, map[string]any{"ok": true, "taskID": payload.TaskID, "source": "raw"})
}

// buildAndSaveHistoryS3 builds the vertical history for a node and saves to S3.
// Uses the same tree-structured path as local mode:
//   histories/proposal.json
//   histories/MLST-0001-A/milestone.json
//   histories/MLST-0001-A/TASK-0001-1/history.json
func buildAndSaveHistoryS3(ctx context.Context, store *proxystore.Store, payload taskCompletedPayload, summary string) {
	userID := payload.UserID
	planID := sanitizeProjectName(payload.ProjectName)
	if planID == "" {
		planID = "active"
	}

	switch payload.NodeType {
	case "proposal":
		// Load proposal from plans and save to histories.
		plans, err := store.ListPlans(ctx, userID)
		if err == nil && len(plans) > 0 {
			_ = store.SaveHistory(ctx, userID, planID, "proposal.json", plans[len(plans)-1])
		}

	case "milestone":
		milestone := map[string]any{
			"uid":     payload.NodeUID,
			"summary": summary,
			"status":  "complete",
		}
		_ = store.SaveMilestoneHistory(ctx, userID, planID, payload.NodeUID, milestone)

	case "task":
		milestoneUID := payload.ParentBranch
		if idx := strings.LastIndex(milestoneUID, "/"); idx >= 0 {
			milestoneUID = milestoneUID[idx+1:]
		}

		history := map[string]any{
			"frontier":  payload.Frontier,
			"summary":   summary,
			"nodeUid":   payload.NodeUID,
			"status":    "complete",
		}
		_ = store.SaveTaskHistory(ctx, userID, planID, payload.NodeUID, history)

		// Bottom-up: update milestone history with this task's summary.
		if mh, err := store.LoadMilestoneHistory(ctx, userID, planID, milestoneUID); err == nil {
			tasks, _ := mh["tasks"].([]any)
			tasks = append(tasks, map[string]any{
				"uid": payload.NodeUID, "summary": summary, "status": "complete",
			})
			mh["tasks"] = tasks
			_ = store.SaveMilestoneHistory(ctx, userID, planID, milestoneUID, mh)
		}

		log.Printf("[task-completed] History saved: tasks/%s.json", payload.NodeUID)
	}
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

// dispatchPendingTasks loads pending runs from S3 and creates commands for each task node.
func dispatchPendingTasks(ctx context.Context, store *proxystore.Store, userID string) int {
	runs, err := store.LoadPendingRuns(ctx, userID)
	if err != nil {
		log.Printf("[task-completed] LoadPendingRuns error: %v", err)
		return 0
	}

	total := 0
	for runID, run := range runs {
		dispatched := 0
		for _, nodeBytes := range run.Nodes {
			if dispatched >= run.MaxAgents {
				break
			}

			// The node bytes are already the createTask payload fields.
			// Wrap them in a command.
			cmdID := fmt.Sprintf("cmd-pending-%d-%d", time.Now().UnixNano(), dispatched)
			cmd := proxystore.PendingCommand{
				ID:        cmdID,
				Action:    "createTask",
				Payload:   nodeBytes,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}

			if err := store.SaveCommand(ctx, userID, cmd); err != nil {
				log.Printf("[task-completed] SaveCommand error for pending task: %v", err)
				continue
			}
			dispatched++
		}
		total += dispatched

		// Clean up pending run.
		if err := store.DeletePendingRun(ctx, userID, runID); err != nil {
			log.Printf("[task-completed] DeletePendingRun error: %v", err)
		}
		log.Printf("[task-completed] Pending run %s: %d tasks dispatched", runID, dispatched)
	}
	return total
}

// checkAndHandleMilestoneCompletion checks tree.json for milestone completion.
func checkAndHandleMilestoneCompletion(ctx context.Context, store *proxystore.Store, userID, planID, taskNodeUID string) {
	// Load tree.json from S3.
	trackData, err := store.LoadTrack(ctx, userID, planID)
	if err != nil {
		return
	}

	nodes, ok := trackData["nodes"].([]any)
	if !ok {
		return
	}

	// Convert to orchestration format.
	var nodeList []map[string]any
	for _, n := range nodes {
		if node, ok := n.(map[string]any); ok {
			nodeList = append(nodeList, node)
		}
	}

	milestoneUID, allComplete := orchestration.CheckMilestoneCompletion(nodeList, taskNodeUID)
	if !allComplete {
		return
	}

	// All tasks under milestone complete → create review command.
	var childSummaries []string
	milestoneID := ""
	milestoneSummary := ""
	for _, node := range nodeList {
		if uid, _ := node["uid"].(string); uid == milestoneUID {
			milestoneID, _ = node["id"].(string)
			milestoneSummary, _ = node["summary"].(string)
			break
		}
	}
	for _, node := range nodeList {
		pid, _ := node["parentId"].(string)
		nType, _ := node["type"].(string)
		if pid == milestoneID && nType == "task" {
			if s, _ := node["summary"].(string); s != "" {
				childSummaries = append(childSummaries, s)
			}
		}
	}

	reviewPrompt := orchestration.BuildReviewPrompt("milestone", milestoneSummary, childSummaries)

	// Save review command.
	cmdID := fmt.Sprintf("cmd-review-%s-%d", milestoneUID, time.Now().UnixMilli())
	payload, _ := json.Marshal(map[string]any{
		"name":         "Code Review: " + milestoneUID,
		"prompt":       reviewPrompt,
		"numAgents":    1,
		"tags":         []string{milestoneUID, "review"},
		"agentType":    "cursor",
		"mode":         "review",
		"nodeType":     "review",
		"nodeUid":      milestoneUID,
		"parentBranch": "dev/" + milestoneUID,
		"projectName":  planID,
	})
	cmd := proxystore.PendingCommand{
		ID:        cmdID,
		Action:    "createReviewTask",
		Payload:   payload,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.SaveCommand(ctx, userID, cmd); err != nil {
		log.Printf("[task-completed] Failed to save review command: %v", err)
	} else {
		log.Printf("[task-completed] Milestone %s complete — review dispatched", milestoneUID)
	}

	// Update milestone status to "reviewing" in tree.json.
	for _, node := range nodeList {
		if uid, _ := node["uid"].(string); uid == milestoneUID {
			node["status"] = "reviewing"
			break
		}
	}
	trackData["nodes"] = nodeList
	_ = store.SaveTrack(ctx, userID, planID, trackData)
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
