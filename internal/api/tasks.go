package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task string `json:"task"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Task == "" {
		writeError(w, http.StatusBadRequest, "task text is required")
		return
	}

	task, err := s.Orchestrator.Distribute(r.Context(), req.Task)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Metrics.TaskTotal.WithLabelValues(string(task.Status)).Inc()
	s.Hub.Broadcast(task.TenantID, "task_update", map[string]any{
		"task_id": task.ID,
		"status":  string(task.Status),
	})
	writeJSON(w, http.StatusOK, map[string]string{"task_id": task.ID})
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.Store.GetTask(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, task)
}
