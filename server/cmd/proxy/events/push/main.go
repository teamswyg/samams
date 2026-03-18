// Proxy events push Lambda. Receives batched events from proxy and forwards to SQS.
// POST /proxy/events → Lambda → SQS persistence pipeline

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"

	proxystore "server/infra/out/proxystore"
)

type proxyEvent struct {
	Type    string          `json:"type"`
	TaskID  string          `json:"taskID"`
	Payload json.RawMessage `json:"payload"`
	TS      int64           `json:"ts"`
}

type requestBody struct {
	Events []proxyEvent `json:"events"`
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	userID := req.RequestContext.Authorizer["principalId"]
	if userID == nil || userID.(string) == "" {
		return respond(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var body requestBody
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}

	queueURL := os.Getenv("SQS_QUEUE_URL")
	if queueURL == "" {
		return respond(http.StatusInternalServerError, map[string]string{"error": "SQS_QUEUE_URL not set"})
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "aws config"})
	}

	sqsClient := sqs.NewFromConfig(cfg)
	lambdaClient := awslambda.NewFromConfig(cfg)
	store := proxystore.New(s3.NewFromConfig(cfg))

	var wg sync.WaitGroup
	accepted := 0
	uid := userID.(string)
	for _, evt := range body.Events {
		msg, _ := json.Marshal(map[string]any{
			"userID":  uid,
			"type":    evt.Type,
			"taskID":  evt.TaskID,
			"payload": evt.Payload,
			"ts":      evt.TS,
		})
		msgStr := string(msg)
		_, err := sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:    &queueURL,
			MessageBody: &msgStr,
		})
		if err != nil {
			fmt.Printf("[proxy-events] SQS send error for event %s: %v\n", evt.Type, err)
			continue
		}
		accepted++

		// Save to tracks (S3) for persistent log storage.
		switch evt.Type {
		case "agent.stateChanged", "agent.logAppended", "maal.record":
			var payloadMap map[string]any
			if evt.Payload != nil {
				_ = json.Unmarshal(evt.Payload, &payloadMap)
			}
			entry := map[string]any{
				"type":      evt.Type,
				"timestamp": evt.TS,
			}
			for k, v := range payloadMap {
				entry[k] = v
			}
			if evt.TaskID != "" {
				_ = store.AppendTrackLog(ctx, uid, evt.TaskID, entry)
			}
		}

		// task.completed → invoke proxy-task-completed Lambda asynchronously.
		if evt.Type == "task.completed" {
			wg.Add(1)
			go func(e proxyEvent) {
				defer wg.Done()
				invokeTaskCompleted(ctx, lambdaClient, uid, e)
			}(evt)
		}

		// milestone.review.completed → invoke milestone-review-completed Lambda.
		if evt.Type == "milestone.review.completed" {
			wg.Add(1)
			go func(e proxyEvent) {
				defer wg.Done()
				invokeMilestoneReviewCompleted(ctx, lambdaClient, uid, e)
			}(evt)
		}

		// milestone.review.failed → auto-approve (update tree status via S3).
		if evt.Type == "milestone.review.failed" {
			var p map[string]any
			if evt.Payload != nil {
				_ = json.Unmarshal(evt.Payload, &p)
			}
			nodeUid, _ := p["nodeUid"].(string)
			projectName, _ := p["projectName"].(string)
			if nodeUid != "" {
				planID := sanitizeName(projectName)
				if track, err := store.LoadTrack(ctx, uid, planID); err == nil {
					if nodes, ok := track["nodes"].([]any); ok {
						for i, n := range nodes {
							node, ok := n.(map[string]any)
							if !ok { continue }
							if u, _ := node["uid"].(string); u == nodeUid {
								node["status"] = "complete"
								node["reviewDecision"] = "auto-approved (review agent failed)"
								nodes[i] = node
								break
							}
						}
						track["nodes"] = nodes
						_ = store.SaveTrack(ctx, uid, planID, track)
					}
				}
				fmt.Printf("[proxy-events] Milestone review failed %s — auto-approved\n", nodeUid)
			}
		}
	}
	wg.Wait()

	return respond(http.StatusOK, map[string]int{"accepted": accepted})
}

// invokeTaskCompleted asynchronously invokes the proxy-task-completed Lambda.
func invokeTaskCompleted(ctx context.Context, client *awslambda.Client, userID string, evt proxyEvent) {
	funcName := os.Getenv("TASK_COMPLETED_LAMBDA")
	if funcName == "" {
		funcName = "go-samams-proxy-task-completed"
	}

	// Extract context and frontier from payload.
	var payloadMap map[string]any
	if evt.Payload != nil {
		_ = json.Unmarshal(evt.Payload, &payloadMap)
	}
	agentContext, _ := payloadMap["context"].(string)
	frontier, _ := payloadMap["frontier"].(string)
	agentID, _ := payloadMap["agentID"].(string)
	nodeType, _ := payloadMap["nodeType"].(string)
	nodeUid, _ := payloadMap["nodeUid"].(string)
	parentBranch, _ := payloadMap["parentBranch"].(string)
	projectName, _ := payloadMap["projectName"].(string)

	// Build the invoke payload as API Gateway proxy format (for handler compatibility).
	invokeBody, _ := json.Marshal(map[string]string{
		"userID":       userID,
		"taskID":       evt.TaskID,
		"agentID":      agentID,
		"context":      agentContext,
		"frontier":     frontier,
		"nodeType":     nodeType,
		"nodeUid":      nodeUid,
		"parentBranch": parentBranch,
		"projectName":  projectName,
	})
	invokePayload, _ := json.Marshal(map[string]any{
		"body": string(invokeBody),
	})

	_, err := client.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName:   &funcName,
		InvocationType: lambdaTypes.InvocationTypeEvent, // async (fire-and-forget)
		Payload:        invokePayload,
	})
	if err != nil {
		fmt.Printf("[proxy-events] Failed to invoke task-completed Lambda for task %s: %v\n", evt.TaskID, err)
	} else {
		fmt.Printf("[proxy-events] Invoked task-completed Lambda for task %s (async)\n", evt.TaskID)
	}
}

// invokeMilestoneReviewCompleted asynchronously invokes the milestone-review-completed Lambda.
func invokeMilestoneReviewCompleted(ctx context.Context, client *awslambda.Client, userID string, evt proxyEvent) {
	funcName := os.Getenv("MILESTONE_REVIEW_LAMBDA")
	if funcName == "" {
		funcName = "go-samams-milestone-review-completed"
	}

	var payloadMap map[string]any
	if evt.Payload != nil {
		_ = json.Unmarshal(evt.Payload, &payloadMap)
	}
	reviewContext, _ := payloadMap["context"].(string)
	nodeUid, _ := payloadMap["nodeUid"].(string)
	projectName, _ := payloadMap["projectName"].(string)

	invokeBody, _ := json.Marshal(map[string]string{
		"userID":      userID,
		"nodeUid":     nodeUid,
		"context":     reviewContext,
		"projectName": projectName,
	})
	invokePayload, _ := json.Marshal(map[string]any{
		"body": string(invokeBody),
	})

	_, err := client.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName:   &funcName,
		InvocationType: lambdaTypes.InvocationTypeEvent,
		Payload:        invokePayload,
	})
	if err != nil {
		fmt.Printf("[proxy-events] Failed to invoke milestone-review-completed Lambda for %s: %v\n", nodeUid, err)
	} else {
		fmt.Printf("[proxy-events] Invoked milestone-review-completed Lambda for %s (async)\n", nodeUid)
	}
}

func sanitizeName(name string) string {
	var b []byte
	prev := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b = append(b, byte(r))
			prev = false
		} else if !prev && len(b) > 0 {
			b = append(b, '-')
			prev = true
		}
	}
	s := string(b)
	for len(s) > 0 && s[len(s)-1] == '-' {
		s = s[:len(s)-1]
	}
	if len(s) > 50 {
		s = s[:50]
	}
	return s
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
