package httpserver

import (
	"net/http"
	"sync"
	"time"

	"server/internal/app/control"
	"server/internal/app/strategy"
)

// SentinelHandler handles /sentinel/control/* endpoints.
type SentinelHandler struct {
	controlSvc  *control.Service
	strategySvc *strategy.Service

	// In-memory state for local dev status/alerts/logs
	mu     sync.RWMutex
	alerts []alertEntry
	logs   []logEntry
}

type alertEntry struct {
	ID        string `json:"id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

type logEntry struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func NewSentinelHandler(controlSvc *control.Service, strategySvc *strategy.Service) *SentinelHandler {
	return &SentinelHandler{
		controlSvc:  controlSvc,
		strategySvc: strategySvc,
	}
}

func (h *SentinelHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /sentinel/control/status", h.status)
	mux.HandleFunc("GET /sentinel/control/alerts", h.getAlerts)
	mux.HandleFunc("GET /sentinel/control/logs", h.getLogs)
	mux.HandleFunc("GET /sentinel/control/token-usage", h.tokenUsage)
	mux.HandleFunc("POST /sentinel/control/task-done", h.taskDone)
	mux.HandleFunc("POST /sentinel/control/reset", h.reset)
	mux.HandleFunc("POST /sentinel/control/resume", h.resume)
}

func (h *SentinelHandler) status(w http.ResponseWriter, _ *http.Request) {
	WriteOK(w, map[string]any{
		"status":     "active",
		"uptime":     time.Since(startTime).String(),
		"started_at": startTime.Format(time.RFC3339),
		"agents":     0,
		"tasks":      0,
	})
}

var startTime = time.Now()

func (h *SentinelHandler) getAlerts(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	alerts := h.alerts
	if alerts == nil {
		alerts = []alertEntry{}
	}
	WriteOK(w, alerts)
}

func (h *SentinelHandler) getLogs(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	logs := h.logs
	if logs == nil {
		logs = []logEntry{}
	}
	WriteOK(w, logs)
}

func (h *SentinelHandler) tokenUsage(w http.ResponseWriter, _ *http.Request) {
	WriteOK(w, map[string]any{
		"used_tokens":      0,
		"available_tokens": 100000,
		"used_ratio":       0.0,
	})
}

func (h *SentinelHandler) taskDone(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SubjectID string `json:"subject_id"`
		Threshold int    `json:"threshold"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Threshold == 0 {
		req.Threshold = 3
	}

	err := h.strategySvc.RecordTaskDone(r.Context(), strategy.RecordTaskDoneCommand{
		SubjectID: req.SubjectID,
		Threshold: req.Threshold,
	})
	if err != nil {
		MapDomainError(w, err)
		return
	}
	WriteOK(w, map[string]string{"status": "recorded"})
}

func (h *SentinelHandler) reset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID string `json:"project_id"`
		Reason    string `json:"reason"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProjectID == "" {
		req.ProjectID = "default"
	}
	if req.Reason == "" {
		req.Reason = "manual reset"
	}

	stateID, err := h.controlSvc.ContextReset(r.Context(), control.ResetCommand{
		ProjectID: req.ProjectID,
		Reason:    req.Reason,
	})
	if err != nil {
		MapDomainError(w, err)
		return
	}
	WriteOK(w, map[string]string{"state_id": stateID})
}

func (h *SentinelHandler) resume(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateID string `json:"state_id"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := h.controlSvc.ContextLoaded(r.Context(), control.ResumeCommand{
		StateID: req.StateID,
	})
	if err != nil {
		MapDomainError(w, err)
		return
	}
	WriteOK(w, map[string]string{"status": "resumed"})
}

// AddAlert appends an alert (used internally by event publishers).
func (h *SentinelHandler) AddAlert(id, level, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.alerts = append(h.alerts, alertEntry{
		ID:        id,
		Level:     level,
		Message:   message,
		CreatedAt: time.Now().Format(time.RFC3339),
	})
}

// AddLog appends a log entry (used internally by event publishers).
func (h *SentinelHandler) AddLog(id, source, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, logEntry{
		ID:        id,
		Source:    source,
		Message:   message,
		CreatedAt: time.Now().Format(time.RFC3339),
	})
}

