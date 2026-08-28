package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Zettaverse/agent-hub-core/internal/store"
)

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.Store.ListAgents(r.Context(), tenantFrom(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	agent, err := s.Store.GetAgent(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	var agent store.Agent
	if err := decodeJSON(r, &agent); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if agent.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	agent.ID = uuid.NewString()
	agent.TenantID = tenantFrom(r)
	agent.CreatedAt = now()
	agent.UpdatedAt = now()
	if len(agent.Skills) == 0 {
		agent.Skills = []store.Skill{}
	}
	created, err := s.Store.CreateAgent(r.Context(), agent)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.Store.GetAgent(r.Context(), tenantFrom(r), id)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	var agent store.Agent
	if err := decodeJSON(r, &agent); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	existing.Name = agent.Name
	existing.Profile = agent.Profile
	existing.SystemPrompt = agent.SystemPrompt
	existing.Skills = agent.Skills
	existing.Enabled = agent.Enabled
	existing.UpdatedAt = now()
	updated, err := s.Store.UpdateAgent(r.Context(), existing)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	err := s.Store.DeleteAgent(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
