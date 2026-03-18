// Proxy commands poll Lambda. Returns pending commands from S3.
// GET /proxy/commands → Lambda → S3 list pending/ → JSON response

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

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	userID := req.RequestContext.Authorizer["principalId"]
	if userID == nil || userID.(string) == "" {
		return respond(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": "aws config"})
	}

	store := s3store.New(s3.NewFromConfig(cfg))
	commands, err := store.ListPendingCommands(ctx, userID.(string))
	if err != nil {
		return respond(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return respond(http.StatusOK, map[string]any{"commands": commands})
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
