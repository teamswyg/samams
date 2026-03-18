// Package proxystore implements S3-backed storage for proxy communication.
// S3 key layout (SPEC §3.5):
//
//	proxy-commands/{userID}/pending/{commandId}.json
//	proxy-commands/{userID}/completed/{commandId}.json
//	proxy-state/{userID}/heartbeat.json
//	proxy-state/{userID}/last-seen.json
package proxystore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Store provides S3 operations for proxy state and commands.
type Store struct {
	client *s3.Client
	bucket string
}

// New creates a proxystore backed by S3.
// Reads bucket name from PERSISTENCE_BUCKET env var.
func New(client *s3.Client) *Store {
	bucket := os.Getenv("PERSISTENCE_BUCKET")
	if bucket == "" {
		bucket = "go-samams-persistence-942056910697"
	}
	return &Store{client: client, bucket: bucket}
}

// SaveHeartbeat stores the heartbeat snapshot to S3.
func (s *Store) SaveHeartbeat(ctx context.Context, userID string, data any) error {
	return s.putJSON(ctx, fmt.Sprintf("proxy-state/%s/heartbeat.json", userID), data)
}

// UpdateLastSeen records the current time to S3.
func (s *Store) UpdateLastSeen(ctx context.Context, userID string, t time.Time) error {
	return s.putJSON(ctx, fmt.Sprintf("proxy-state/%s/last-seen.json", userID), map[string]string{
		"lastSeen": t.UTC().Format(time.RFC3339),
	})
}

// PendingCommand represents a command awaiting proxy pickup.
type PendingCommand struct {
	ID        string          `json:"id"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"createdAt"`
}

// ListPendingCommands lists all pending commands for a user.
func (s *Store) ListPendingCommands(ctx context.Context, userID string) ([]PendingCommand, error) {
	prefix := fmt.Sprintf("proxy-commands/%s/pending/", userID)
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("list pending commands: %w", err)
	}

	var commands []PendingCommand
	for _, obj := range out.Contents {
		if obj.Key == nil || !strings.HasSuffix(*obj.Key, ".json") {
			continue
		}
		data, err := s.getJSON(ctx, *obj.Key)
		if err != nil {
			continue // skip unreadable files
		}
		var cmd PendingCommand
		if err := json.Unmarshal(data, &cmd); err != nil {
			continue
		}
		commands = append(commands, cmd)
	}

	return commands, nil
}

// SaveCommand stores a new command in the pending directory.
// Called by run-start Lambda when frontend requests task creation.
func (s *Store) SaveCommand(ctx context.Context, userID string, cmd PendingCommand) error {
	key := fmt.Sprintf("proxy-commands/%s/pending/%s.json", userID, cmd.ID)
	return s.putJSON(ctx, key, cmd)
}

// CompleteCommand moves a command from pending to completed with the response.
func (s *Store) CompleteCommand(ctx context.Context, userID, commandID string, response any) error {
	pendingKey := fmt.Sprintf("proxy-commands/%s/pending/%s.json", userID, commandID)
	completedKey := fmt.Sprintf("proxy-commands/%s/completed/%s.json", userID, commandID)

	// Read the original command.
	data, err := s.getJSON(ctx, pendingKey)
	if err != nil {
		return fmt.Errorf("read pending command: %w", err)
	}

	// Build completed record.
	var original map[string]any
	_ = json.Unmarshal(data, &original)
	original["response"] = response
	original["completedAt"] = time.Now().UTC().Format(time.RFC3339)

	// Write completed.
	if err := s.putJSON(ctx, completedKey, original); err != nil {
		return fmt.Errorf("write completed command: %w", err)
	}

	// Delete pending.
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &pendingKey,
	})
	if err != nil {
		return fmt.Errorf("delete pending command: %w", err)
	}

	return nil
}

// PendingRun holds task nodes waiting for proposal setup to complete.
type PendingRun struct {
	Nodes       []json.RawMessage `json:"nodes"`
	MaxAgents   int               `json:"maxAgents"`
	ProjectName string            `json:"projectName"`
	CreatedAt   string            `json:"createdAt"`
}

// SavePendingRun stores pending task nodes to S3.
// S3 key: proxy-pending-runs/{userID}/{runID}.json
func (s *Store) SavePendingRun(ctx context.Context, userID, runID string, run PendingRun) error {
	key := fmt.Sprintf("proxy-pending-runs/%s/%s.json", userID, runID)
	return s.putJSON(ctx, key, run)
}

// LoadPendingRuns loads all pending runs for a user.
func (s *Store) LoadPendingRuns(ctx context.Context, userID string) (map[string]PendingRun, error) {
	prefix := fmt.Sprintf("proxy-pending-runs/%s/", userID)
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, err
	}

	runs := make(map[string]PendingRun)
	for _, obj := range out.Contents {
		if obj.Key == nil || !strings.HasSuffix(*obj.Key, ".json") {
			continue
		}
		data, err := s.getJSON(ctx, *obj.Key)
		if err != nil {
			continue
		}
		var run PendingRun
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}
		// Extract runID from key.
		parts := strings.Split(*obj.Key, "/")
		runID := strings.TrimSuffix(parts[len(parts)-1], ".json")
		runs[runID] = run
	}
	return runs, nil
}

// DeletePendingRun removes a pending run from S3.
func (s *Store) DeletePendingRun(ctx context.Context, userID, runID string) error {
	key := fmt.Sprintf("proxy-pending-runs/%s/%s.json", userID, runID)
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	return err
}

// AppendTrackLog appends a log entry for a task in the tracks directory.
// S3 key: {userID}/tracks/logs/{taskID}.json
// Loads existing → appends → saves (max 500 entries).
func (s *Store) AppendTrackLog(ctx context.Context, userID, taskID string, entry map[string]any) error {
	key := fmt.Sprintf("%s/tracks/logs/%s.json", userID, taskID)

	var logs []any
	if data, err := s.getJSON(ctx, key); err == nil {
		_ = json.Unmarshal(data, &logs)
	}

	logs = append(logs, entry)
	if len(logs) > 500 {
		logs = logs[len(logs)-500:]
	}

	return s.putJSON(ctx, key, logs)
}

// SaveTaskSummary stores the generated summary for a completed task.
// S3 key: proxy-state/{userID}/summaries/{taskID}.json
func (s *Store) SaveTaskSummary(ctx context.Context, userID, taskID string, summary any) error {
	key := fmt.Sprintf("proxy-state/%s/summaries/%s.json", userID, taskID)
	return s.putJSON(ctx, key, summary)
}

// ── Plans (per-user) ────────────────────────────────────────────

// SavePlan stores a plan document.
// S3 key: {userID}/plans/{planId}.json
func (s *Store) SavePlan(ctx context.Context, userID, planID string, plan any) error {
	return s.putJSON(ctx, fmt.Sprintf("%s/plans/%s.json", userID, planID), plan)
}

// GetPlan retrieves a plan document.
func (s *Store) GetPlan(ctx context.Context, userID, planID string) ([]byte, error) {
	return s.getJSON(ctx, fmt.Sprintf("%s/plans/%s.json", userID, planID))
}

// ListPlans returns all plans for a user.
func (s *Store) ListPlans(ctx context.Context, userID string) ([]json.RawMessage, error) {
	prefix := fmt.Sprintf("%s/plans/", userID)
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &s.bucket, Prefix: &prefix,
	})
	if err != nil {
		return nil, err
	}
	var plans []json.RawMessage
	for _, obj := range out.Contents {
		if obj.Key == nil || strings.Contains(*obj.Key, "/trees/") || !strings.HasSuffix(*obj.Key, ".json") {
			continue
		}
		if data, err := s.getJSON(ctx, *obj.Key); err == nil {
			plans = append(plans, json.RawMessage(data))
		}
	}
	return plans, nil
}

// DeletePlan removes a plan and its tree.
func (s *Store) DeletePlan(ctx context.Context, userID, planID string) error {
	s.deleteKey(ctx, fmt.Sprintf("%s/plans/%s.json", userID, planID))
	s.deleteKey(ctx, fmt.Sprintf("%s/plans/trees/%s.json", userID, planID))
	return nil
}

// SavePlanTree stores a plan's task tree.
func (s *Store) SavePlanTree(ctx context.Context, userID, planID string, tree any) error {
	return s.putJSON(ctx, fmt.Sprintf("%s/plans/trees/%s.json", userID, planID), tree)
}

// GetPlanTree retrieves a plan's task tree.
func (s *Store) GetPlanTree(ctx context.Context, userID, planID string) ([]byte, error) {
	return s.getJSON(ctx, fmt.Sprintf("%s/plans/trees/%s.json", userID, planID))
}

// ── Tracks (per-user) ───────────────────────────────────────────

// SaveTrack stores a forked tree for tracking.
// S3 key: {userID}/tracks/{planId}/tree.json
func (s *Store) SaveTrack(ctx context.Context, userID, planID string, track any) error {
	return s.putJSON(ctx, fmt.Sprintf("%s/tracks/%s/tree.json", userID, planID), track)
}

// GetTrack retrieves a track tree.
func (s *Store) GetTrack(ctx context.Context, userID, planID string) ([]byte, error) {
	return s.getJSON(ctx, fmt.Sprintf("%s/tracks/%s/tree.json", userID, planID))
}

// UpdateTrackNodeStatus updates a node in the tracked tree.
func (s *Store) UpdateTrackNodeStatus(ctx context.Context, userID, planID, nodeUID, status string, extra map[string]string) error {
	key := fmt.Sprintf("%s/tracks/%s/tree.json", userID, planID)
	data, err := s.getJSON(ctx, key)
	if err != nil {
		return err
	}
	var track map[string]any
	if err := json.Unmarshal(data, &track); err != nil {
		return err
	}
	if nodes, ok := track["nodes"].([]any); ok {
		for i, n := range nodes {
			if node, ok := n.(map[string]any); ok {
				if node["uid"] == nodeUID {
					node["status"] = status
					for k, v := range extra {
						node[k] = v
					}
					nodes[i] = node
					break
				}
			}
		}
	}
	return s.putJSON(ctx, key, track)
}

// GetTrackNodeLogs retrieves logs for a node.
func (s *Store) GetTrackNodeLogs(ctx context.Context, userID, planID, nodeUID string) ([]byte, error) {
	return s.getJSON(ctx, fmt.Sprintf("%s/tracks/%s/logs/%s.json", userID, planID, nodeUID))
}

// SaveHistory saves a node's vertical history file.
// S3 key: {userID}/tracks/{planId}/histories/{path}/history.json
func (s *Store) SaveHistory(ctx context.Context, userID, planID, path string, history any) error {
	key := fmt.Sprintf("%s/tracks/%s/histories/%s", userID, planID, path)
	return s.putJSON(ctx, key, history)
}

func (s *Store) deleteKey(ctx context.Context, key string) {
	s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket, Key: &key,
	})
}

func (s *Store) putJSON(ctx context.Context, key string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	ct := "application/json"
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         aws.String(key),
		Body:        bytes.NewReader(b),
		ContentType: &ct,
	})
	return err
}

// ── History Storage ──────────────────────────────────────────

// LoadTrack reads the track tree.json from S3.
func (s *Store) LoadTrack(ctx context.Context, userID, planID string) (map[string]any, error) {
	key := fmt.Sprintf("%s/tracks/%s/tree.json", userID, planID)
	b, err := s.getJSON(ctx, key)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, json.Unmarshal(b, &result)
}

// SaveProposalHistory stores the proposal document to S3.
func (s *Store) SaveProposalHistory(ctx context.Context, userID, planID string, data any) error {
	key := fmt.Sprintf("%s/tracks/%s/histories/proposal.json", userID, planID)
	return s.putJSON(ctx, key, data)
}

// SaveMilestoneHistory stores a milestone history to S3.
func (s *Store) SaveMilestoneHistory(ctx context.Context, userID, planID, milestoneUID string, data any) error {
	key := fmt.Sprintf("%s/tracks/%s/histories/milestones/%s.json", userID, planID, milestoneUID)
	return s.putJSON(ctx, key, data)
}

// LoadMilestoneHistory reads a milestone history from S3.
func (s *Store) LoadMilestoneHistory(ctx context.Context, userID, planID, milestoneUID string) (map[string]any, error) {
	key := fmt.Sprintf("%s/tracks/%s/histories/milestones/%s.json", userID, planID, milestoneUID)
	b, err := s.getJSON(ctx, key)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, json.Unmarshal(b, &result)
}

// SaveTaskHistory stores a task history (self-contained, with frontier) to S3.
func (s *Store) SaveTaskHistory(ctx context.Context, userID, planID, taskUID string, data any) error {
	key := fmt.Sprintf("%s/tracks/%s/histories/tasks/%s.json", userID, planID, taskUID)
	return s.putJSON(ctx, key, data)
}

// LoadTaskHistory reads a task history from S3.
func (s *Store) LoadTaskHistory(ctx context.Context, userID, planID, taskUID string) (map[string]any, error) {
	key := fmt.Sprintf("%s/tracks/%s/histories/tasks/%s.json", userID, planID, taskUID)
	b, err := s.getJSON(ctx, key)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, json.Unmarshal(b, &result)
}

func (s *Store) getJSON(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}
