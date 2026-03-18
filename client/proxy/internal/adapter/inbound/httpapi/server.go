package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"proxy/internal/domain"
	"proxy/internal/port"
)

type Server struct {
	svc port.TaskService
}

func NewServer(svc port.TaskService) *Server {
	return &Server{svc: svc}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", s.handleHealth)

	mux.HandleFunc("/tasks", s.handleTasks)
	mux.HandleFunc("/tasks/", s.handleTaskByID)

	mux.HandleFunc("/agents", s.handleAgents)
	mux.HandleFunc("/agents/", s.handleAgentByID)

	mux.HandleFunc("/maal/tasks/", s.handleMaalByTask)
	mux.HandleFunc("/maal/agents/", s.handleMaalByAgent)

	mux.HandleFunc("/notifications", s.handleNotifications)

	mux.HandleFunc("/strategy/apply", s.handleStrategyApply)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createTask(w, r)
	case http.MethodGet:
		s.listTasks(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/tasks/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	taskID := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.getTask(w, r, taskID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	action := parts[1]
	switch r.Method {
	case http.MethodPost:
		switch action {
		case "scale":
			s.scaleTask(w, r, taskID)
		case "stop":
			s.stopTask(w, r, taskID)
		case "pause":
			s.pauseTask(w, r, taskID)
		case "resume":
			s.resumeTask(w, r, taskID)
		case "cancel":
			s.cancelTask(w, r, taskID)
		case "reset":
			s.resetTask(w, r, taskID)
		case "retry-increment":
			s.retryIncrement(w, r, taskID)
		case "context-planned":
			s.contextPlanned(w, r, taskID)
		default:
			http.NotFound(w, r)
		}
	case http.MethodPut:
		switch action {
		case "summary":
			s.updateSummary(w, r, taskID)
		default:
			http.NotFound(w, r)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listAgents(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/agents/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	agentID := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.getAgent(w, r, agentID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	action := parts[1]
	switch r.Method {
	case http.MethodPost:
		switch action {
		case "stop":
			s.stopAgent(w, r, agentID)
		case "input":
			s.inputAgent(w, r, agentID)
		default:
			http.NotFound(w, r)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "prompt is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	summary, err := s.svc.CreateTask(ctx, domain.CreateTaskParams{
		Name:             req.Name,
		Prompt:           req.Prompt,
		NumAgents:        req.NumAgents,
		CursorArgs:       req.CursorArgs,
		Tags:             req.Tags,
		BoundedContextID: req.BoundedContextID,
		ParentTaskID:     req.ParentTaskID,
		AgentType:        req.toAgentType(),
		Mode:             req.toMode(),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, summary)
}

func (s *Server) listTasks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.ListTasks())
}

func (s *Server) getTask(w http.ResponseWriter, _ *http.Request, taskID string) {
	summary, err := s.svc.GetTask(taskID)
	if err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) scaleTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req scaleTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	if req.NumAgents <= 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "numAgents must be > 0"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	summary, err := s.svc.ScaleTask(ctx, taskID, domain.ScaleTaskParams{NumAgents: req.NumAgents})
	if err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) stopTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req stopTaskRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	opts := &domain.StopTaskOptions{
		Graceful:    req.Graceful,
		Reason:      req.Reason,
		CancelledBy: req.CancelledBy,
	}
	if err := s.svc.StopTask(ctx, taskID, opts); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) pauseTask(w http.ResponseWriter, r *http.Request, taskID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.svc.PauseTask(ctx, taskID); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) resumeTask(w http.ResponseWriter, r *http.Request, taskID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	summary, err := s.svc.ResumeTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req cancelTaskRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.CancelledBy == "" {
		req.CancelledBy = "user"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.svc.CancelTask(ctx, taskID, req.Reason, req.CancelledBy); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) retryIncrement(w http.ResponseWriter, r *http.Request, taskID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	count, triggered := s.svc.IncrementRetryCount(ctx, taskID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"retryCount":       count,
		"triggeredMeeting": triggered,
	})
}

func (s *Server) updateSummary(w http.ResponseWriter, r *http.Request, taskID string) {
	var req updateSummaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.svc.UpdateTaskSummary(ctx, taskID, req.Summary); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) contextPlanned(w http.ResponseWriter, r *http.Request, taskID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.svc.SetContextPlanned(ctx, taskID); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) resetTask(w http.ResponseWriter, r *http.Request, taskID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	summary, err := s.svc.ResetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) listAgents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.ListAgents())
}

func (s *Server) getAgent(w http.ResponseWriter, _ *http.Request, agentID string) {
	agent, logs, err := s.svc.GetAgent(agentID)
	if err != nil {
		if errors.Is(err, domain.ErrAgentNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"agent": agent, "logs": logs})
}

func (s *Server) stopAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.svc.StopAgent(ctx, agentID); err != nil {
		if errors.Is(err, domain.ErrAgentNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) inputAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	var body struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	if strings.TrimSpace(body.Input) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "input is required"})
		return
	}
	if err := s.svc.AppendAgentInput(agentID, body.Input); err != nil {
		if errors.Is(err, domain.ErrAgentNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMaalByTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	taskID := strings.TrimPrefix(r.URL.Path, "/maal/tasks/")
	if taskID == "" {
		http.NotFound(w, r)
		return
	}
	records := s.svc.GetMaalByTaskID(taskID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"taskId": taskID, "records": records})
}

func (s *Server) handleMaalByAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agentID := strings.TrimPrefix(r.URL.Path, "/maal/agents/")
	if agentID == "" {
		http.NotFound(w, r)
		return
	}
	records := s.svc.GetMaalByAgentID(agentID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"agentId": agentID, "records": records})
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"notifications": s.svc.ListNotifications()})
}

func (s *Server) handleStrategyApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var decision domain.StrategyDecision
	if err := json.NewDecoder(r.Body).Decode(&decision); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := s.svc.StrategyApplyDecision(ctx, decision); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
