package wsconn

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"proxy/internal/port"
)

// WSMessage mirrors the server's WebSocket message envelope.
type WSMessage struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Error     string          `json:"error,omitempty"`
	Timestamp int64           `json:"ts"`
}

const (
	WSTypeCommand  = "command"
	WSTypeResponse = "response"
	WSTypeEvent    = "event"
)

// HeartbeatProvider supplies data for periodic heartbeat events.
type HeartbeatProvider func() HeartbeatPayload

// HeartbeatPayload is the body of a heartbeat event.
type HeartbeatPayload struct {
	AgentStatuses map[string]string `json:"agentStatuses"`
	Uptime        int64             `json:"uptime"`
	TaskCount     int               `json:"taskCount"`
}

// Client manages the WebSocket connection to the SAMAMS server.
type Client struct {
	serverURL         string
	token             string
	handler           port.CommandHandler
	heartbeat         HeartbeatProvider
	heartbeatInterval time.Duration
	startedAt         time.Time

	mu   sync.Mutex
	conn *websocket.Conn

	ctx    context.Context
	cancel context.CancelFunc
}

func New(serverURL, token string, handler port.CommandHandler) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		serverURL:         serverURL,
		token:             token,
		handler:           handler,
		heartbeatInterval: 10 * time.Second,
		startedAt:         time.Now(),
		ctx:               ctx,
		cancel:            cancel,
	}
}

func (c *Client) SetHandler(h port.CommandHandler) {
	c.handler = h
}

func (c *Client) SetHeartbeatProvider(p HeartbeatProvider) {
	c.heartbeat = p
}

func (c *Client) SetHeartbeatInterval(d time.Duration) {
	c.mu.Lock()
	c.heartbeatInterval = d
	c.mu.Unlock()
}

// UpdateToken replaces the token for future reconnections.
func (c *Client) UpdateToken(token string) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

func (c *Client) Run(ctx context.Context) error {
	c.mu.Lock()
	c.cancel()
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.mu.Unlock()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}

		err := c.connect()
		if err != nil {
			log.Printf("[ws] Connection failed: %v (retry in %s)", err, backoff)
			select {
			case <-time.After(backoff):
			case <-c.ctx.Done():
				return c.ctx.Err()
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		backoff = time.Second
		log.Println("[ws] Connected to server")

		hbCtx, hbCancel := context.WithCancel(c.ctx)
		go c.heartbeatLoop(hbCtx)

		c.readPump()
		hbCancel()

		log.Println("[ws] Disconnected from server, reconnecting...")
	}
}

func (c *Client) connect() error {
	wsURL := c.serverURL
	if strings.HasPrefix(wsURL, "http://") {
		wsURL = "ws://" + strings.TrimPrefix(wsURL, "http://")
	} else if strings.HasPrefix(wsURL, "https://") {
		wsURL = "wss://" + strings.TrimPrefix(wsURL, "https://")
	}
	if !strings.Contains(wsURL, "/ws/proxy") {
		wsURL = strings.TrimSuffix(wsURL, "/") + "/ws/proxy"
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"Authorization": {fmt.Sprintf("Bearer %s", c.token)},
		},
	})
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}

	conn.SetReadLimit(4 * 1024 * 1024) // 4MB — strategy meeting allPaused can carry multiple agent contexts

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	return nil
}

func (c *Client) readPump() {
	for {
		var msg WSMessage
		err := wsjson.Read(c.ctx, c.conn, &msg)
		if err != nil {
			if c.ctx.Err() == nil {
				log.Printf("[ws] Read error: %v", err)
			}
			return
		}

		if msg.Type == WSTypeCommand {
			go c.handleCommand(msg)
		}
	}
}

func (c *Client) handleCommand(msg WSMessage) {
	respPayload, err := c.handler(msg.Action, msg.Payload)

	resp := WSMessage{
		ID:        msg.ID,
		Type:      WSTypeResponse,
		Action:    msg.Action,
		Payload:   respPayload,
		Timestamp: time.Now().UnixMilli(),
	}
	if err != nil {
		resp.Error = err.Error()
	}

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return
	}

	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, conn, resp); err != nil {
		log.Printf("[ws] Write response error: %v", err)
	}
}

func (c *Client) SendEvent(action string, payload any) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	msg := WSMessage{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      WSTypeEvent,
		Action:    action,
		Payload:   payloadBytes,
		Timestamp: time.Now().UnixMilli(),
	}

	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	return wsjson.Write(ctx, conn, msg)
}

func (c *Client) heartbeatLoop(ctx context.Context) {
	if c.heartbeat == nil {
		return
	}
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			payload := c.heartbeat()
			payload.Uptime = int64(time.Since(c.startedAt).Seconds())
			if err := c.SendEvent("heartbeat", payload); err != nil {
				log.Printf("[ws] Heartbeat send error: %v", err)
			}
		}
	}
}

func (c *Client) Close() {
	c.mu.Lock()
	c.cancel()
	if c.conn != nil {
		c.conn.Close(websocket.StatusNormalClosure, "client shutting down")
	}
	c.mu.Unlock()
}
