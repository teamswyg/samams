// Proxy command response Lambda. Moves command from pending to completed in S3.
// POST /proxy/commands/{id}/response → Lambda → S3 move pending → completed

package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	s3store "server/infra/out/proxystore"
)

type responseBody struct {
	Payload json.RawMessage `json:"payload"`
	Error   string          `json:"error,omitempty"`
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	userID := req.RequestContext.Authorizer["principalId"]
	if userID == nil || userID.(string) == "" {
		return respond(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	commandID := req.PathParameters["id"]
	if commandID == "" {
		return respond(http.StatusBadRequest, map[string]string{"error": "missing command id"})
	}

	var body responseBody
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return respond(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "aws config"})
	}

	store := s3store.New(s3.NewFromConfig(cfg))
	if err := store.CompleteCommand(ctx, userID.(string), commandID, body); err != nil {
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
