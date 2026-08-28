package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Zettaverse/agent-hub-core/internal/store"
)

func (s *Server) listFlows(w http.ResponseWriter, r *http.Request) {
	flows, err := s.Store.ListFlows(r.Context(), tenantFrom(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, flows)
}

func (s *Server) getFlow(w http.ResponseWriter, r *http.Request) {
	fl, err := s.Store.GetFlow(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, fl)
}

func (s *Server) createFlow(w http.ResponseWriter, r *http.Request) {
	var fl store.Flow
	if err := decodeJSON(r, &fl); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if fl.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.Flow.Validate(fl.FlowJSON); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	fl.ID = uuid.NewString()
	fl.TenantID = tenantFrom(r)
	fl.CreatedAt = now()
	fl.UpdatedAt = now()
	created, err := s.Store.CreateFlow(r.Context(), fl)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateFlow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.Store.GetFlow(r.Context(), tenantFrom(r), id)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	var fl store.Flow
	if err := decodeJSON(r, &fl); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(fl.FlowJSON) > 0 {
		if err := s.Flow.Validate(fl.FlowJSON); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		existing.FlowJSON = fl.FlowJSON
	}
	existing.Name = fl.Name
	existing.Permissions = fl.Permissions
	existing.Enabled = fl.Enabled
	existing.UpdatedAt = now()
	updated, err := s.Store.UpdateFlow(r.Context(), existing)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteFlow(w http.ResponseWriter, r *http.Request) {
	err := s.Store.DeleteFlow(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runFlow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	fl, err := s.Store.GetFlow(r.Context(), tenantFrom(r), id)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	if !fl.Enabled {
		writeError(w, http.StatusBadRequest, "flow is disabled")
		return
	}

	started := now()
	run := store.Run{
		ID:        uuid.NewString(),
		FlowID:    fl.ID,
		TenantID:  fl.TenantID,
		Status:    store.RunStatusRunning,
		StartedAt: started,
		Logs:      []store.LogEntry{},
	}
	run, err = s.Store.CreateRun(r.Context(), run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res := s.Flow.Execute(r.Context(), fl)
	finished := now()
	run.Status = res.Status
	run.FinishedAt = &finished
	run.Logs = res.Logs
	run.Result = res.Result
	run, err = s.Store.UpdateRun(r.Context(), run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Metrics.FlowRunsTotal.WithLabelValues(string(res.Status)).Inc()
	s.Hub.Broadcast(fl.TenantID, "run_update", map[string]any{
		"run_id":  run.ID,
		"flow_id": fl.ID,
		"status":  string(res.Status),
	})

	writeJSON(w, http.StatusOK, map[string]string{"run_id": run.ID})
}

func (s *Server) listFlowRuns(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	runs, err := s.Store.ListRuns(r.Context(), tenantFrom(r), id)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.Store.GetRun(r.Context(), tenantFrom(r), chi.URLParam(r, "run_id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
