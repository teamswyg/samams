// Package httppoll implements port.ServerConnection via HTTPS polling.
// Used in deploy mode (SAMAMS_MODE=deploy) to communicate with API Gateway + Lambda.
package httppoll

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"proxy/internal/port"
)

// HeartbeatProvider returns current proxy state for heartbeat.
type HeartbeatProvider func() HeartbeatPayload

// HeartbeatPayload matches wsconn.HeartbeatPayload.
type HeartbeatPayload struct {
	AgentStatuses map[string]string `json:"agentStatuses"`
	Uptime        int64             `json:"uptime"`
	TaskCount     int               `json:"taskCount"`
}

// Config holds HTTPS adapter configuration.
type Config struct {
	ServerURL          string
	Token              string
	PollIntervalActive time.Duration
	PollIntervalIdle   time.Duration
	HeartbeatInterval  time.Duration
}

// ActiveChecker returns true if any agent is active (running/starting).
type ActiveChecker func() bool

// Client implements port.ServerConnection via HTTPS polling.
type Client struct {
	cfg       Config
	handler   port.CommandHandler
	heartbeat HeartbeatProvider
	isActive  ActiveChecker
	startedAt time.Time

	httpClient *http.Client

	// Event queue for retry on failure (max 100, drop oldest).
	mu       sync.Mutex
	eventBuf []pendingEvent
}

type pendingEvent struct {
	Action   string `json:"type"`
	TaskID   string `json:"taskID,omitempty"`
	Payload  any    `json:"payload"`
	TS       int64  `json:"ts"`
	Critical bool   `json:"-"` // never drop critical events
}

// criticalActions are events that must never be dropped from the buffer.
var criticalActions = map[string]bool{
	"task.completed":              true,
	"task.failed":                 true,
	"milestone.review.completed":  true,
	"milestone.review.failed":     true,
	"milestone.merged":            true,
	"contextLost":                 true,
}

// PendingCommand is a command from the server awaiting execution.
type PendingCommand struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

func New(cfg Config, handler port.CommandHandler) *Client {
	return &Client{
		cfg:       cfg,
		handler:   handler,
		startedAt: time.Now(),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) SetHandler(h port.CommandHandler) { c.handler = h }

func (c *Client) SetHeartbeatProvider(p HeartbeatProvider) { c.heartbeat = p }

func (c *Client) SetActiveChecker(f ActiveChecker) { c.isActive = f }

// UpdateToken replaces the Bearer token for all future HTTP requests.
func (c *Client) UpdateToken(token string) {
	c.mu.Lock()
	c.cfg.Token = token
	c.mu.Unlock()
}

func (c *Client) Run(ctx context.Context) error {
	log.Println("[https] Starting HTTPS polling adapter")

	c.sendHeartbeat(ctx)
	go c.heartbeatLoop(ctx)
	c.pollLoop(ctx)

	return ctx.Err()
}

func (c *Client) SendEvent(action string, payload any) error {
	evt := pendingEvent{
		Action:   action,
		Payload:  payload,
		TS:       time.Now().UnixMilli(),
		Critical: criticalActions[action],
	}

	if err := c.pushEvents(context.Background(), []pendingEvent{evt}); err != nil {
		c.mu.Lock()
		c.eventBuf = append(c.eventBuf, evt)
		if len(c.eventBuf) > 100 {
			c.eventBuf = trimNonCritical(c.eventBuf, 100)
		}
		c.mu.Unlock()
		log.Printf("[https] Event queued (push failed: %v), buffer size: %d", err, len(c.eventBuf))
		return nil
	}
	return nil
}

// trimNonCritical drops non-critical events from the front until buffer <= maxSize.
// Critical events are never dropped.
func trimNonCritical(buf []pendingEvent, maxSize int) []pendingEvent {
	for len(buf) > maxSize {
		dropped := false
		for i := 0; i < len(buf); i++ {
			if !buf[i].Critical {
				buf = append(buf[:i], buf[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			break // all events are critical, can't drop any
		}
	}
	return buf
}

func (c *Client) Close() {
	log.Println("[https] Closing HTTPS adapter")
}

func (c *Client) pollLoop(ctx context.Context) {
	for {
		interval := c.currentPollInterval()
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}

		commands, err := c.fetchCommands(ctx)
		if err != nil {
			log.Printf("[https] Poll error: %v", err)
			continue
		}

		for _, cmd := range commands {
			respPayload, handlerErr := c.handler(cmd.Action, cmd.Payload)
			c.sendCommandResponse(ctx, cmd.ID, respPayload, handlerErr)
		}

		c.flushEventBuffer(ctx)
	}
}

func (c *Client) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendHeartbeat(ctx)
		}
	}
}

// currentPollInterval returns the adaptive polling interval.
// TRADEOFF: Polling 간격 vs Lambda 비용 vs 명령 응답 지연
//
// active 3초 = 20 req/min, idle 10초 = 6 req/min
// Lambda 1M 무료 요청/월 → 하루 ~33,000 요청까지 무료
func (c *Client) currentPollInterval() time.Duration {
	if c.isActive != nil && c.isActive() {
		return c.cfg.PollIntervalActive
	}
	return c.cfg.PollIntervalIdle
}

func (c *Client) sendHeartbeat(ctx context.Context) {
	if c.heartbeat == nil {
		return
	}
	payload := c.heartbeat()
	payload.Uptime = int64(time.Since(c.startedAt).Seconds())

	body, _ := json.Marshal(payload)
	resp, err := c.doRequest(ctx, "POST", "/proxy/heartbeat", body)
	if err != nil {
		log.Printf("[https] Heartbeat error: %v", err)
		return
	}
	resp.Body.Close()
}

func (c *Client) fetchCommands(ctx context.Context) ([]PendingCommand, error) {
	resp, err := c.doRequest(ctx, "GET", "/proxy/commands", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Commands []PendingCommand `json:"commands"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode commands response: %w", err)
	}
	return result.Commands, nil
}

func (c *Client) sendCommandResponse(ctx context.Context, commandID string, payload json.RawMessage, handlerErr error) {
	body := map[string]any{
		"payload": payload,
	}
	if handlerErr != nil {
		body["error"] = handlerErr.Error()
	}
	b, _ := json.Marshal(body)

	resp, err := c.doRequest(ctx, "POST", "/proxy/commands/"+commandID+"/response", b)
	if err != nil {
		log.Printf("[https] Command response error for %s: %v", commandID, err)
		return
	}
	resp.Body.Close()
}

func (c *Client) pushEvents(ctx context.Context, events []pendingEvent) error {
	if len(events) == 0 {
		return nil
	}
	body, _ := json.Marshal(map[string]any{"events": events})
	resp, err := c.doRequest(ctx, "POST", "/proxy/events", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) flushEventBuffer(ctx context.Context) {
	c.mu.Lock()
	if len(c.eventBuf) == 0 {
		c.mu.Unlock()
		return
	}
	events := make([]pendingEvent, len(c.eventBuf))
	copy(events, c.eventBuf)
	c.eventBuf = c.eventBuf[:0]
	c.mu.Unlock()

	if err := c.pushEvents(ctx, events); err != nil {
		c.mu.Lock()
		c.eventBuf = append(events, c.eventBuf...)
		if len(c.eventBuf) > 100 {
			c.eventBuf = trimNonCritical(c.eventBuf, 100)
		}
		c.mu.Unlock()
		log.Printf("[https] Event flush failed: %v, %d events re-queued", err, len(events))
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := c.cfg.ServerURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}
