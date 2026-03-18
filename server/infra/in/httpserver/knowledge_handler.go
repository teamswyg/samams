package httpserver

import (
	"net/http"

	appKnowledge "server/internal/app/knowledge"
	domainKnowledge "server/internal/domain/knowledge"
	"server/internal/domain/shared"
)

type KnowledgeHandler struct {
	svc *appKnowledge.Service
}

func NewKnowledgeHandler(svc *appKnowledge.Service) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc}
}

func (h *KnowledgeHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /knowledge/entries", h.create)
	mux.HandleFunc("GET /knowledge/entries/{id}", h.getByID)
	mux.HandleFunc("PUT /knowledge/entries/{id}", h.update)
	mux.HandleFunc("PUT /knowledge/entries/{id}/confidence", h.setConfidence)
	mux.HandleFunc("POST /knowledge/entries/{id}/usage", h.recordUsage)
	mux.HandleFunc("DELETE /knowledge/entries/{id}", h.deactivate)
	mux.HandleFunc("GET /knowledge/projects/{projectID}/entries", h.listByProject)
	mux.HandleFunc("POST /knowledge/projects/{projectID}/search", h.search)
}

func (h *KnowledgeHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID      string   `json:"tenant_id"`
		ProjectID     string   `json:"project_id"`
		Title         string   `json:"title"`
		Content       string   `json:"content"`
		Category      string   `json:"category"`
		Tags          []string `json:"tags"`
		SourceAgentID string   `json:"source_agent_id"`
		SourceTaskID  string   `json:"source_task_id"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cat := domainKnowledge.ParseCategory(req.Category)
	entry, err := h.svc.Create(r.Context(), appKnowledge.CreateEntryCommand{
		TenantID:      shared.TenantID(req.TenantID),
		ProjectID:     shared.ProjectID(req.ProjectID),
		Title:         req.Title,
		Content:       req.Content,
		Category:      cat,
		Tags:          req.Tags,
		SourceAgentID: req.SourceAgentID,
		SourceTaskID:  req.SourceTaskID,
	})
	if err != nil {
		MapDomainError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "data": entryToDTO(entry)})
}

func (h *KnowledgeHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id := domainKnowledge.ID(r.PathValue("id"))
	entry, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		MapDomainError(w, err)
		return
	}
	WriteOK(w, entryToDTO(entry))
}

func (h *KnowledgeHandler) update(w http.ResponseWriter, r *http.Request) {
	id := domainKnowledge.ID(r.PathValue("id"))
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := h.svc.Update(r.Context(), appKnowledge.UpdateEntryCommand{
		ID:      id,
		Title:   req.Title,
		Content: req.Content,
	})
	if err != nil {
		MapDomainError(w, err)
		return
	}
	WriteOK(w, map[string]string{"status": "updated"})
}

func (h *KnowledgeHandler) setConfidence(w http.ResponseWriter, r *http.Request) {
	id := domainKnowledge.ID(r.PathValue("id"))
	var req struct {
		Confidence int32 `json:"confidence"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := h.svc.SetConfidence(r.Context(), appKnowledge.SetConfidenceCommand{
		ID:         id,
		Confidence: domainKnowledge.Confidence(req.Confidence),
	})
	if err != nil {
		MapDomainError(w, err)
		return
	}
	WriteOK(w, map[string]string{"status": "updated"})
}

func (h *KnowledgeHandler) recordUsage(w http.ResponseWriter, r *http.Request) {
	id := domainKnowledge.ID(r.PathValue("id"))
	if err := h.svc.RecordUsage(r.Context(), id); err != nil {
		MapDomainError(w, err)
		return
	}
	WriteOK(w, map[string]string{"status": "recorded"})
}

func (h *KnowledgeHandler) deactivate(w http.ResponseWriter, r *http.Request) {
	id := domainKnowledge.ID(r.PathValue("id"))
	if err := h.svc.Deactivate(r.Context(), id); err != nil {
		MapDomainError(w, err)
		return
	}
	WriteOK(w, map[string]string{"status": "deactivated"})
}

func (h *KnowledgeHandler) listByProject(w http.ResponseWriter, r *http.Request) {
	projectID := shared.ProjectID(r.PathValue("projectID"))
	entries, err := h.svc.ListByProject(r.Context(), projectID)
	if err != nil {
		MapDomainError(w, err)
		return
	}
	dtos := make([]knowledgeEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, entryToDTO(e))
	}
	WriteOK(w, dtos)
}

func (h *KnowledgeHandler) search(w http.ResponseWriter, r *http.Request) {
	projectID := shared.ProjectID(r.PathValue("projectID"))
	var req struct {
		Query    string   `json:"query"`
		Category string   `json:"category"`
		Tags     []string `json:"tags"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	entries, err := h.svc.Search(r.Context(), appKnowledge.SearchCommand{
		ProjectID: projectID,
		Query:     req.Query,
		Category:  domainKnowledge.ParseCategory(req.Category),
		Tags:      req.Tags,
	})
	if err != nil {
		MapDomainError(w, err)
		return
	}
	dtos := make([]knowledgeEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, entryToDTO(e))
	}
	WriteOK(w, dtos)
}

type knowledgeEntryDTO struct {
	ID            string   `json:"id"`
	TenantID      string   `json:"tenant_id"`
	ProjectID     string   `json:"project_id"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Category      string   `json:"category"`
	Tags          []string `json:"tags"`
	SourceAgentID string   `json:"source_agent_id"`
	SourceTaskID  string   `json:"source_task_id"`
	Confidence    int32    `json:"confidence"`
	UsageCount    int32    `json:"usage_count"`
	Active        bool     `json:"active"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

func entryToDTO(e *domainKnowledge.Entry) knowledgeEntryDTO {
	tags := e.Tags
	if tags == nil {
		tags = []string{}
	}
	return knowledgeEntryDTO{
		ID:            string(e.ID),
		TenantID:      string(e.TenantID),
		ProjectID:     string(e.ProjectID),
		Title:         e.Title,
		Content:       e.Content,
		Category:      e.Category.String(),
		Tags:          tags,
		SourceAgentID: e.SourceAgentID,
		SourceTaskID:  e.SourceTaskID,
		Confidence:    int32(e.Confidence),
		UsageCount:    e.UsageCount,
		Active:        e.Active,
		CreatedAt:     e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     e.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
