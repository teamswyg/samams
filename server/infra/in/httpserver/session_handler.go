package httpserver

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"server/internal/domain/shared"
)

// SessionHandler provides in-memory chat session management for local development.
// In production this would be backed by a real persistence layer.
type SessionHandler struct {
	mu       sync.RWMutex
	sessions map[string]*session
}

type session struct {
	ID        string    `json:"id"`
	CreatedAt string    `json:"created_at"`
	Messages  []message `json:"messages,omitempty"`
}

type message struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func NewSessionHandler() *SessionHandler {
	defaultID := shared.GenerateID()
	return &SessionHandler{
		sessions: map[string]*session{
			defaultID: {
				ID:        defaultID,
				CreatedAt: time.Now().Format(time.RFC3339),
				Messages: []message{
					{
						ID:        shared.GenerateID(),
						Role:      "system",
						Content:   "Welcome to SAMAMS. How can I help you?",
						CreatedAt: time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	}
}

func (h *SessionHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /sessions", h.list)
	mux.HandleFunc("GET /sessions/{id}", h.detail)
	mux.HandleFunc("GET /sessions/{id}/messages", h.getMessages)
	mux.HandleFunc("POST /sessions/{id}/messages", h.sendMessage)
}

func (h *SessionHandler) list(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []map[string]string
	for _, s := range h.sessions {
		result = append(result, map[string]string{
			"id":         s.ID,
			"created_at": s.CreatedAt,
		})
	}
	if result == nil {
		result = []map[string]string{}
	}
	WriteOK(w, result)
}

func (h *SessionHandler) detail(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r, "id")
	h.mu.RLock()
	defer h.mu.RUnlock()

	s, ok := h.sessions[id]
	if !ok {
		WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	WriteOK(w, s)
}

func (h *SessionHandler) getMessages(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r, "id")
	h.mu.RLock()
	defer h.mu.RUnlock()

	s, ok := h.sessions[id]
	if !ok {
		WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	WriteOK(w, s.Messages)
}

func (h *SessionHandler) sendMessage(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r, "id")

	var req struct {
		Content string `json:"content"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	h.mu.Lock()
	s, ok := h.sessions[id]
	if !ok {
		// Auto-create session
		s = &session{
			ID:        id,
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		h.sessions[id] = s
	}

	userMsg := message{
		ID:        shared.GenerateID(),
		Role:      "user",
		Content:   req.Content,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.Messages = append(s.Messages, userMsg)

	// Echo back a simple assistant response for local dev.
	assistantMsg := message{
		ID:        shared.GenerateID(),
		Role:      "assistant",
		Content:   "Received: " + req.Content,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.Messages = append(s.Messages, assistantMsg)
	h.mu.Unlock()

	WriteOK(w, assistantMsg)
}

// extractPathParam extracts a named path parameter from Go 1.22+ routing.
func extractPathParam(r *http.Request, name string) string {
	v := r.PathValue(name)
	if v != "" {
		return v
	}
	// Fallback: manual extraction for /sessions/{id}/messages pattern
	parts := strings.Split(r.URL.Path, "/")
	for i, p := range parts {
		if p == "sessions" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
