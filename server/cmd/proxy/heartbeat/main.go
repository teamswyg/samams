// Proxy heartbeat Lambda. Receives heartbeat from proxy client via API Gateway.
// Saves heartbeat data and last-seen timestamp to S3.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	s3store "server/infra/out/proxystore"
)

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	userID := req.RequestContext.Authorizer["principalId"]
	if userID == nil || userID.(string) == "" {
		return respond(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var body struct {
		AgentStatuses map[string]string `json:"agentStatuses"`
		Uptime        int64             `json:"uptime"`
		TaskCount     int               `json:"taskCount"`
	}
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "aws config"})
	}

	store := s3store.New(s3.NewFromConfig(cfg))
	uid := userID.(string)

	if err := store.SaveHeartbeat(ctx, uid, body); err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if err := store.UpdateLastSeen(ctx, uid, time.Now()); err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return respond(http.StatusOK, map[string]bool{"ok": true})
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
