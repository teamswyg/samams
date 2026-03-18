package httpserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"server/infra/persistence/localstore"
)

// validID matches safe ID strings: alphanumeric, dash, underscore, dot only.
var validID = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

// PlanHandler handles /plans/* endpoints for planning documents and task trees.
// Data is stored as raw JSON in localstore — no domain model needed.
type PlanHandler struct {
	store *localstore.Store
}

func NewPlanHandler(store *localstore.Store) *PlanHandler {
	return &PlanHandler{store: store}
}

func (h *PlanHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /plans", h.list)
	mux.HandleFunc("POST /plans", h.save)
	mux.HandleFunc("GET /plans/{id}", h.get)
	mux.HandleFunc("DELETE /plans/{id}", h.remove)
	mux.HandleFunc("GET /plans/{id}/tree", h.getTree)
	mux.HandleFunc("POST /plans/{id}/tree", h.saveTree)

	// Tracking — forked tree with live status + logs per node.
	mux.HandleFunc("GET /tracks/{planId}", h.getTrack)
	mux.HandleFunc("POST /tracks/{planId}", h.saveTrack)
	mux.HandleFunc("POST /tracks/{planId}/nodes/{nodeUid}/status", h.updateNodeStatus)
	mux.HandleFunc("POST /tracks/{planId}/nodes/{nodeUid}/log", h.appendNodeLog)
	mux.HandleFunc("GET /tracks/{planId}/nodes/{nodeUid}/logs", h.getNodeLogs)
}

// list returns all saved plans.
func (h *PlanHandler) list(w http.ResponseWriter, r *http.Request) {
	keys, err := h.store.List("plans")
	if err != nil {
		log.Printf("[plans] list error: %v", err)
		WriteOK(w, []any{})
		return
	}

	var plans []json.RawMessage
	for _, k := range keys {
		// Skip tree files.
		if strings.Contains(k, "/trees/") {
			continue
		}
		var doc json.RawMessage
		if err := h.store.Get(k, &doc); err == nil {
			plans = append(plans, doc)
		}
	}

	if plans == nil {
		plans = []json.RawMessage{}
	}
	WriteOK(w, plans)
}

// get returns a single plan by ID.
func (h *PlanHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || !validID.MatchString(id) {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var doc json.RawMessage
	if err := h.store.Get("plans/"+id+".json", &doc); err != nil {
		WriteError(w, http.StatusNotFound, "plan not found")
		return
	}
	WriteOK(w, doc)
}

// save creates or updates a plan.
func (h *PlanHandler) save(w http.ResponseWriter, r *http.Request) {
	var doc map[string]any
	if err := DecodeJSON(r, &doc); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}

	id, _ := doc["id"].(string)
	if id == "" {
		id = fmt.Sprintf("%d", time.Now().UnixMilli())
		doc["id"] = id
	}
	if !validID.MatchString(id) {
		WriteError(w, http.StatusBadRequest, "invalid plan id")
		return
	}

	if _, ok := doc["createdAt"]; !ok {
		doc["createdAt"] = time.Now().UnixMilli()
	}
	doc["updatedAt"] = time.Now().UnixMilli()

	if err := h.store.Put("plans/"+id+".json", doc); err != nil {
		WriteError(w, http.StatusInternalServerError, "save failed")
		return
	}

	WriteOK(w, doc)
}

// remove deletes a plan and its tree.
func (h *PlanHandler) remove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || !validID.MatchString(id) {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	h.store.Delete("plans/" + id + ".json")
	h.store.Delete("plans/trees/" + id + ".json")
	WriteOK(w, map[string]string{"deleted": id})
}

// getTree returns the task tree for a plan.
func (h *PlanHandler) getTree(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || !validID.MatchString(id) {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var tree json.RawMessage
	if err := h.store.Get("plans/trees/"+id+".json", &tree); err != nil {
		WriteError(w, http.StatusNotFound, "tree not found")
		return
	}
	WriteOK(w, tree)
}

// saveTree saves the task tree for a plan.
func (h *PlanHandler) saveTree(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || !validID.MatchString(id) {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var tree any
	if err := DecodeJSON(r, &tree); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if err := h.store.Put("plans/trees/"+id+".json", tree); err != nil {
		WriteError(w, http.StatusInternalServerError, "save tree failed")
		return
	}

	WriteOK(w, map[string]string{"saved": id})
}

// ── Tracking endpoints ────────────────────────────────────────

// getTrack returns the forked tree with live status.
func (h *PlanHandler) getTrack(w http.ResponseWriter, r *http.Request) {
	planId := r.PathValue("planId")
	if planId == "" || !validID.MatchString(planId) {
		WriteError(w, http.StatusBadRequest, "invalid planId")
		return
	}

	var track json.RawMessage
	if err := h.store.Get("tracks/"+planId+"/tree.json", &track); err != nil {
		WriteError(w, http.StatusNotFound, "track not found")
		return
	}
	WriteOK(w, track)
}

// saveTrack forks the tree into tracks/{planId}/tree.json.
func (h *PlanHandler) saveTrack(w http.ResponseWriter, r *http.Request) {
	planId := r.PathValue("planId")
	if planId == "" || !validID.MatchString(planId) {
		WriteError(w, http.StatusBadRequest, "invalid planId")
		return
	}

	var tree any
	if err := DecodeJSON(r, &tree); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if err := h.store.Put("tracks/"+planId+"/tree.json", tree); err != nil {
		WriteError(w, http.StatusInternalServerError, "save track failed")
		return
	}
	WriteOK(w, map[string]string{"saved": planId})
}

// updateNodeStatus updates a single node's status in the tracked tree.
func (h *PlanHandler) updateNodeStatus(w http.ResponseWriter, r *http.Request) {
	planId := r.PathValue("planId")
	nodeUid := r.PathValue("nodeUid")
	if planId == "" || nodeUid == "" || !validID.MatchString(planId) || !validID.MatchString(nodeUid) {
		WriteError(w, http.StatusBadRequest, "invalid planId or nodeUid")
		return
	}

	var req struct {
		Status     string `json:"status"`
		BranchName string `json:"branchName,omitempty"`
		TaskID     string `json:"taskId,omitempty"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}

	// Load current track tree.
	var track map[string]any
	if err := h.store.Get("tracks/"+planId+"/tree.json", &track); err != nil {
		WriteError(w, http.StatusNotFound, "track not found")
		return
	}

	// Update the matching node.
	if nodes, ok := track["nodes"].([]any); ok {
		for i, n := range nodes {
			if node, ok := n.(map[string]any); ok {
				if node["uid"] == nodeUid {
					node["status"] = req.Status
					if req.BranchName != "" {
						node["branchName"] = req.BranchName
					}
					if req.TaskID != "" {
						node["taskId"] = req.TaskID
					}
					node["updatedAt"] = time.Now().UnixMilli()
					nodes[i] = node
					break
				}
			}
		}
	}

	if err := h.store.Put("tracks/"+planId+"/tree.json", track); err != nil {
		WriteError(w, http.StatusInternalServerError, "update status failed")
		return
	}
	WriteOK(w, map[string]string{"updated": nodeUid})
}

// appendNodeLog appends a log entry for a node.
func (h *PlanHandler) appendNodeLog(w http.ResponseWriter, r *http.Request) {
	planId := r.PathValue("planId")
	nodeUid := r.PathValue("nodeUid")
	if planId == "" || nodeUid == "" || !validID.MatchString(planId) || !validID.MatchString(nodeUid) {
		WriteError(w, http.StatusBadRequest, "invalid planId or nodeUid")
		return
	}

	var entry map[string]any
	if err := DecodeJSON(r, &entry); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	entry["timestamp"] = time.Now().UnixMilli()

	// Load existing logs or start fresh.
	key := "tracks/" + planId + "/logs/" + nodeUid + ".json"
	var logs []any
	_ = h.store.Get(key, &logs) // ignore error (file may not exist)

	logs = append(logs, entry)

	// Keep last 500 logs per node.
	if len(logs) > 500 {
		logs = logs[len(logs)-500:]
	}

	if err := h.store.Put(key, logs); err != nil {
		WriteError(w, http.StatusInternalServerError, "save log failed")
		return
	}
	WriteOK(w, map[string]int{"count": len(logs)})
}

// getNodeLogs returns logs for a specific node.
func (h *PlanHandler) getNodeLogs(w http.ResponseWriter, r *http.Request) {
	planId := r.PathValue("planId")
	nodeUid := r.PathValue("nodeUid")
	if planId == "" || nodeUid == "" || !validID.MatchString(planId) || !validID.MatchString(nodeUid) {
		WriteError(w, http.StatusBadRequest, "invalid planId or nodeUid")
		return
	}

	key := "tracks/" + planId + "/logs/" + nodeUid + ".json"
	var logs []json.RawMessage
	if err := h.store.Get(key, &logs); err != nil {
		WriteOK(w, []any{})
		return
	}
	WriteOK(w, logs)
}
