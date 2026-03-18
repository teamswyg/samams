// Persistence writer: SQS-triggered Lambda. Validates event, idempotency, then writes to S3.
// Event schema and idempotency: see docs/infra-persistence.md

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	metadataIdempotencyKey     = "X-Idempotency-Key"     // sent on PUT
	metadataIdempotencyKeyLow  = "x-idempotency-key"    // S3 returns lowercase in HEAD
)

type persistenceEvent struct {
	Key            string          `json:"key"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
	ExpectedETag   *string         `json:"expected_etag,omitempty"`
}

func handler(ctx context.Context, ev events.SQSEvent) (events.SQSEventResponse, error) {
	bucket := os.Getenv("PERSISTENCE_BUCKET")
	if bucket == "" {
		return events.SQSEventResponse{}, fmt.Errorf("PERSISTENCE_BUCKET not set")
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return events.SQSEventResponse{}, err
	}
	client := s3.NewFromConfig(cfg)

	var failures []events.SQSBatchItemFailure
	for _, record := range ev.Records {
		if err := processRecord(ctx, client, bucket, record.Body); err != nil {
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
		}
	}

	return events.SQSEventResponse{BatchItemFailures: failures}, nil
}

func processRecord(ctx context.Context, client *s3.Client, bucket, body string) error {
	var ev persistenceEvent
	if err := json.Unmarshal([]byte(body), &ev); err != nil {
		return err
	}
	if ev.Key == "" || ev.IdempotencyKey == "" {
		return fmt.Errorf("key and idempotency_key required")
	}

	// HEAD to check existing object and idempotency
	head, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(ev.Key),
	})
	if err == nil {
		// Object exists: check idempotency (S3 returns metadata keys lowercase)
		if v := head.Metadata[metadataIdempotencyKeyLow]; v == ev.IdempotencyKey {
			return nil // already applied
		}
		// Conditional write
		if ev.ExpectedETag != nil && head.ETag != nil && *head.ETag != *ev.ExpectedETag {
			return fmt.Errorf("expected_etag mismatch: current %s", aws.ToString(head.ETag))
		}
	}
	// err != nil: object might not exist (NotFound); we'll PUT

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(ev.Key),
		Body:        bytes.NewReader(ev.Payload),
		ContentType: aws.String("application/json"),
		Metadata: map[string]string{
			metadataIdempotencyKey: ev.IdempotencyKey,
		},
	})
	return err
}

func main() {
	lambda.Start(handler)
}
