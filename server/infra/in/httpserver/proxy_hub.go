package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"server/internal/domain/shared"
)

var ErrProxyNotConnected = errors.New("proxy not connected")

// ProxyEventRouter routes proxy events to domain services.
// Implemented by the wiring layer (cmd/local) to decouple ProxyHub from domain imports.
type ProxyEventRouter interface {
	// RouteEvent processes a proxy event and dispatches it to the appropriate domain service.
	// action: the event action (e.g., "contextLost", "task.completed")
	// userID: the proxy owner
	// payload: raw JSON payload from the proxy
	RouteEvent(ctx context.Context, userID, action string, payload json.RawMessage)
}

// noopRouter is the default router that only logs events.
type noopRouter struct{}

func (noopRouter) RouteEvent(_ context.Context, userID, action string, _ json.RawMessage) {
	log.Printf("[proxy-hub] Event from %s: %s (no router configured)", userID, action)
}

// ProxyHub manages WebSocket connections from proxy clients.
// Server sends commands to proxies; proxies send events back.
type ProxyHub struct {
	auth   *AuthHandler
	router ProxyEventRouter

	mu    sync.RWMutex
	conns map[string]*proxyConn // userID → connection

	// pending tracks command requests waiting for a response.
	pending sync.Map // messageID → chan *WSMessage
}

type proxyConn struct {
	ws        *websocket.Conn
	userID    string
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func NewProxyHub(auth *AuthHandler) *ProxyHub {
	return &ProxyHub{
		auth:   auth,
		router: noopRouter{},
		conns:  make(map[string]*proxyConn),
	}
}

// SetEventRouter sets the router for dispatching proxy events to domain services.
func (h *ProxyHub) SetEventRouter(r ProxyEventRouter) {
	if r != nil {
		h.router = r
	}
}

// HandleWebSocket upgrades HTTP to WebSocket and registers the proxy connection.
func (h *ProxyHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Authenticate via Bearer token.
	userID := h.auth.ExtractUser(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow cross-origin for local dev
	})
	if err != nil {
		log.Printf("[proxy-hub] WebSocket accept error: %v", err)
		return
	}
	ws.SetReadLimit(4 * 1024 * 1024) // 4MB — strategy meeting allPaused can carry multiple agent contexts

	ctx, cancel := context.WithCancel(r.Context())
	conn := &proxyConn{ws: ws, userID: userID, cancel: cancel}

	// Register connection (replace existing if any).
	h.mu.Lock()
	if old, ok := h.conns[userID]; ok {
		old.closeOnce.Do(func() {
			old.cancel()
			old.ws.Close(websocket.StatusGoingAway, "replaced by new connection")
		})
	}
	h.conns[userID] = conn
	h.mu.Unlock()

	log.Printf("[proxy-hub] Proxy connected (userID: %s)", userID)

	// Read pump: handle incoming messages from proxy.
	h.readPump(ctx, conn)

	// Cleanup on disconnect.
	h.mu.Lock()
	if h.conns[userID] == conn {
		delete(h.conns, userID)
	}
	h.mu.Unlock()
	conn.closeOnce.Do(func() {
		cancel()
		ws.Close(websocket.StatusNormalClosure, "disconnected")
	})
	log.Printf("[proxy-hub] Proxy disconnected (userID: %s)", userID)
}

// readPump reads messages from the proxy WebSocket connection.
func (h *ProxyHub) readPump(ctx context.Context, conn *proxyConn) {
	for {
		var msg WSMessage
		err := wsjson.Read(ctx, conn.ws, &msg)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("[proxy-hub] Read error (userID: %s): %v", conn.userID, err)
			}
			return
		}

		switch msg.Type {
		case WSTypeResponse:
			// Match response to pending command.
			if ch, ok := h.pending.LoadAndDelete(msg.ID); ok {
				ch.(chan *WSMessage) <- &msg
			}
		case WSTypeEvent:
			h.handleEvent(conn.userID, &msg)
		default:
			log.Printf("[proxy-hub] Unknown message type: %s", msg.Type)
		}
	}
}

// handleEvent processes events from the proxy by delegating to the EventRouter.
func (h *ProxyHub) handleEvent(userID string, msg *WSMessage) {
	log.Printf("[proxy-hub] Event from %s: %s", userID, msg.Action)
	h.router.RouteEvent(context.Background(), userID, msg.Action, msg.Payload)
}

// SendCommand sends a command to a proxy and waits for the response.
func (h *ProxyHub) SendCommand(ctx context.Context, userID, action string, payload any) (json.RawMessage, error) {
	h.mu.RLock()
	conn, ok := h.conns[userID]
	h.mu.RUnlock()
	if !ok {
		return nil, ErrProxyNotConnected
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	msgID := string(shared.GenerateID())
	msg := WSMessage{
		ID:        msgID,
		Type:      WSTypeCommand,
		Action:    action,
		Payload:   payloadBytes,
		Timestamp: time.Now().UnixMilli(),
	}

	// Register pending response channel.
	respCh := make(chan *WSMessage, 1)
	h.pending.Store(msgID, respCh)
	defer h.pending.Delete(msgID)

	// Send command.
	if err := wsjson.Write(ctx, conn.ws, msg); err != nil {
		return nil, fmt.Errorf("write command: %w", err)
	}

	// Wait for response with timeout.
	timeout := 10 * time.Second
	select {
	case resp := <-respCh:
		if resp.Error != "" {
			return nil, fmt.Errorf("proxy error: %s", resp.Error)
		}
		return resp.Payload, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("proxy command timeout (%s)", action)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// IsConnected returns true if a proxy is connected for the given user.
func (h *ProxyHub) IsConnected(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[userID]
	return ok
}

// AnyConnected returns true if at least one proxy is connected.
func (h *ProxyHub) AnyConnected() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns) > 0
}

// FirstUserID returns the first connected user ID (for single-user local dev).
func (h *ProxyHub) FirstUserID() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for uid := range h.conns {
		return uid
	}
	return ""
}

// Close shuts down all proxy connections.
func (h *ProxyHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, conn := range h.conns {
		conn.closeOnce.Do(func() {
			conn.cancel()
			conn.ws.Close(websocket.StatusGoingAway, "server shutting down")
		})
	}
	h.conns = make(map[string]*proxyConn)
}
