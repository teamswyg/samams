// Plans + Tracks API Lambda (deploy mode).
// Handles all /plans/* and /tracks/* routes via API Gateway proxy integration.
// All data stored in S3 under {userID}/ prefix.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"server/infra/out/proxystore"
)

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	userID, _ := req.RequestContext.Authorizer["principalId"].(string)
	if userID == "" {
		return respond(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "aws config"})
	}
	store := proxystore.New(s3.NewFromConfig(cfg))

	path := req.Path
	method := req.HTTPMethod

	// Route: /plans
	if path == "/plans" && method == "GET" {
		return handleListPlans(ctx, store, userID)
	}
	if path == "/plans" && method == "POST" {
		return handleSavePlan(ctx, store, userID, req.Body)
	}

	// Route: /plans/{id}
	if strings.HasPrefix(path, "/plans/") && !strings.Contains(path, "/tree") {
		planID := req.PathParameters["id"]
		if planID == "" {
			planID = strings.TrimPrefix(path, "/plans/")
		}
		if method == "GET" {
			return handleGetPlan(ctx, store, userID, planID)
		}
		if method == "DELETE" {
			return handleDeletePlan(ctx, store, userID, planID)
		}
	}

	// Route: /plans/{id}/tree
	if strings.HasSuffix(path, "/tree") && strings.HasPrefix(path, "/plans/") {
		planID := req.PathParameters["id"]
		if method == "GET" {
			return handleGetPlanTree(ctx, store, userID, planID)
		}
		if method == "POST" {
			return handleSavePlanTree(ctx, store, userID, planID, req.Body)
		}
	}

	// Route: /tracks/{planId}
	if strings.HasPrefix(path, "/tracks/") && !strings.Contains(path, "/nodes/") {
		planID := req.PathParameters["planId"]
		if method == "GET" {
			return handleGetTrack(ctx, store, userID, planID)
		}
		if method == "POST" {
			return handleSaveTrack(ctx, store, userID, planID, req.Body)
		}
	}

	// Route: /tracks/{planId}/nodes/{nodeUid}/status
	if strings.Contains(path, "/nodes/") && strings.HasSuffix(path, "/status") {
		planID := req.PathParameters["planId"]
		nodeUid := req.PathParameters["nodeUid"]
		return handleUpdateNodeStatus(ctx, store, userID, planID, nodeUid, req.Body)
	}

	// Route: /tracks/{planId}/nodes/{nodeUid}/log
	if strings.Contains(path, "/nodes/") && strings.HasSuffix(path, "/log") {
		planID := req.PathParameters["planId"]
		nodeUid := req.PathParameters["nodeUid"]
		return handleAppendNodeLog(ctx, store, userID, planID, nodeUid, req.Body)
	}

	// Route: /tracks/{planId}/nodes/{nodeUid}/logs
	if strings.Contains(path, "/nodes/") && strings.HasSuffix(path, "/logs") {
		planID := req.PathParameters["planId"]
		nodeUid := req.PathParameters["nodeUid"]
		return handleGetNodeLogs(ctx, store, userID, planID, nodeUid)
	}

	return respond(http.StatusNotFound, map[string]string{"error": "not found"})
}

// ── Plan handlers ──

func handleListPlans(ctx context.Context, store *proxystore.Store, userID string) (events.APIGatewayProxyResponse, error) {
	plans, err := store.ListPlans(ctx, userID)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if plans == nil {
		plans = []json.RawMessage{}
	}
	return respondOK(plans)
}

func handleSavePlan(ctx context.Context, store *proxystore.Store, userID, body string) (events.APIGatewayProxyResponse, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	id, _ := doc["id"].(string)
	if id == "" {
		id = strings.ReplaceAll(time.Now().Format("20060102-150405.000"), ".", "")
		doc["id"] = id
	}
	doc["updatedAt"] = time.Now().UnixMilli()

	if err := store.SavePlan(ctx, userID, id, doc); err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondOK(doc)
}

func handleGetPlan(ctx context.Context, store *proxystore.Store, userID, planID string) (events.APIGatewayProxyResponse, error) {
	data, err := store.GetPlan(ctx, userID, planID)
	if err != nil {
		return respond(http.StatusNotFound, map[string]string{"error": "plan not found"})
	}
	return respondRaw(data)
}

func handleDeletePlan(ctx context.Context, store *proxystore.Store, userID, planID string) (events.APIGatewayProxyResponse, error) {
	store.DeletePlan(ctx, userID, planID)
	return respondOK(map[string]string{"deleted": planID})
}

func handleGetPlanTree(ctx context.Context, store *proxystore.Store, userID, planID string) (events.APIGatewayProxyResponse, error) {
	data, err := store.GetPlanTree(ctx, userID, planID)
	if err != nil {
		return respond(http.StatusNotFound, map[string]string{"error": "tree not found"})
	}
	return respondRaw(data)
}

func handleSavePlanTree(ctx context.Context, store *proxystore.Store, userID, planID, body string) (events.APIGatewayProxyResponse, error) {
	var tree any
	if err := json.Unmarshal([]byte(body), &tree); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if err := store.SavePlanTree(ctx, userID, planID, tree); err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondOK(map[string]string{"saved": planID})
}

// ── Track handlers ──

func handleGetTrack(ctx context.Context, store *proxystore.Store, userID, planID string) (events.APIGatewayProxyResponse, error) {
	data, err := store.GetTrack(ctx, userID, planID)
	if err != nil {
		return respond(http.StatusNotFound, map[string]string{"error": "track not found"})
	}
	return respondRaw(data)
}

func handleSaveTrack(ctx context.Context, store *proxystore.Store, userID, planID, body string) (events.APIGatewayProxyResponse, error) {
	var track any
	if err := json.Unmarshal([]byte(body), &track); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if err := store.SaveTrack(ctx, userID, planID, track); err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondOK(map[string]string{"saved": planID})
}

func handleUpdateNodeStatus(ctx context.Context, store *proxystore.Store, userID, planID, nodeUID, body string) (events.APIGatewayProxyResponse, error) {
	var req struct {
		Status     string `json:"status"`
		BranchName string `json:"branchName"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	extra := map[string]string{}
	if req.BranchName != "" {
		extra["branchName"] = req.BranchName
	}
	if err := store.UpdateTrackNodeStatus(ctx, userID, planID, nodeUID, req.Status, extra); err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondOK(map[string]string{"updated": nodeUID})
}

func handleAppendNodeLog(ctx context.Context, store *proxystore.Store, userID, planID, nodeUID, body string) (events.APIGatewayProxyResponse, error) {
	var entry map[string]any
	if err := json.Unmarshal([]byte(body), &entry); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	entry["timestamp"] = time.Now().UnixMilli()
	taskKey := planID + "/" + nodeUID
	if err := store.AppendTrackLog(ctx, userID, taskKey, entry); err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondOK(map[string]string{"appended": nodeUID})
}

func handleGetNodeLogs(ctx context.Context, store *proxystore.Store, userID, planID, nodeUID string) (events.APIGatewayProxyResponse, error) {
	data, err := store.GetTrackNodeLogs(ctx, userID, planID, nodeUID)
	if err != nil {
		return respondOK([]any{})
	}
	return respondRaw(data)
}

// ── Response helpers ──

func respond(status int, body any) (events.APIGatewayProxyResponse, error) {
	b, _ := json.Marshal(body)
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}, nil
}

func respondOK(data any) (events.APIGatewayProxyResponse, error) {
	b, _ := json.Marshal(map[string]any{"ok": true, "data": data})
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}, nil
}

func respondRaw(data []byte) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       `{"ok":true,"data":` + string(data) + `}`,
	}, nil
}

func main() {
	lambda.Start(handler)
}
