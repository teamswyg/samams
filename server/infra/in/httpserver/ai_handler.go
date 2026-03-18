package httpserver

import (
	"net/http"

	"server/internal/app/agent"
)

// AIHandler handles /ai/* endpoints.
type AIHandler struct {
	agentSvc *agent.Service
}

func NewAIHandler(agentSvc *agent.Service) *AIHandler {
	return &AIHandler{agentSvc: agentSvc}
}

func (h *AIHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /ai/analyze-logs", h.analyzeLogs)
	mux.HandleFunc("POST /ai/generate-plan", h.generatePlan)
	mux.HandleFunc("POST /ai/convert-to-tree", h.convertToTree)
	mux.HandleFunc("POST /ai/summarize", h.summarize)
	mux.HandleFunc("POST /ai/generate-frontier", h.generateFrontier)
	mux.HandleFunc("POST /ai/chat", h.chat)
}

func (h *AIHandler) analyzeLogs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Logs string `json:"logs"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.agentSvc.AnalyzeLogs(r.Context(), agent.AnalyzeLogsCommand{Logs: req.Logs})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteOK(w, map[string]string{"analysis": result})
}

func (h *AIHandler) generatePlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.agentSvc.GeneratePlan(r.Context(), agent.GeneratePlanCommand{Prompt: req.Prompt})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteOK(w, map[string]string{"plan": result})
}

func (h *AIHandler) convertToTree(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlanDocument string `json:"plan_document"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.agentSvc.ConvertPlanToTree(r.Context(), agent.ConvertPlanToTreeCommand{PlanDocument: req.PlanDocument})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteOK(w, map[string]string{"tree": result})
}

func (h *AIHandler) summarize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.agentSvc.Summarize(r.Context(), agent.SummarizeCommand{Content: req.Content})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteOK(w, map[string]string{"summary": result})
}

func (h *AIHandler) chat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
		Context string `json:"context"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.agentSvc.Chat(r.Context(), agent.ChatCommand{
		Message: req.Message,
		Context: req.Context,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteOK(w, map[string]string{"reply": result})
}

func (h *AIHandler) generateFrontier(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccumulatedSummary string `json:"accumulated_summary"`
		ChildTask          string `json:"child_task"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.agentSvc.GenerateFrontier(r.Context(), agent.GenerateFrontierCommand{
		AccumulatedSummary: req.AccumulatedSummary,
		ChildTask:          req.ChildTask,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteOK(w, map[string]string{"frontier": result})
}
