package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Zettaverse/agent-hub-core/internal/mcp"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

func (s *Server) listMcpServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.Store.ListMcpServers(r.Context(), tenantFrom(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

func (s *Server) getMcpServer(w http.ResponseWriter, r *http.Request) {
	server, err := s.Store.GetMcpServer(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (s *Server) createMcpServer(w http.ResponseWriter, r *http.Request) {
	var server store.McpServer
	if err := decodeJSON(r, &server); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if server.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(server.Transport) == 0 {
		writeError(w, http.StatusBadRequest, "transport is required")
		return
	}
	server.ID = uuid.NewString()
	server.TenantID = tenantFrom(r)
	server.Status = "unknown"
	server.CreatedAt = now()
	server.UpdatedAt = now()
	created, err := s.Store.CreateMcpServer(r.Context(), server)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateMcpServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.Store.GetMcpServer(r.Context(), tenantFrom(r), id)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	var server store.McpServer
	if err := decodeJSON(r, &server); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	existing.Name = server.Name
	if len(server.Transport) > 0 {
		existing.Transport = server.Transport
	}
	existing.Enabled = server.Enabled
	existing.Status = server.Status
	existing.UpdatedAt = now()
	updated, err := s.Store.UpdateMcpServer(r.Context(), existing)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteMcpServer(w http.ResponseWriter, r *http.Request) {
	err := s.Store.DeleteMcpServer(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testMcpServer(w http.ResponseWriter, r *http.Request) {
	server, err := s.Store.GetMcpServer(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	transport, err := mcp.TransportFromServer(server.Transport)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transport config")
		return
	}
	tools, err := s.MCP.ListTools(r.Context(), transport)
	if err != nil {
		server.Status = "error"
		_, _ = s.Store.UpdateMcpServer(r.Context(), server)
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	server.Status = "connected"
	_, _ = s.Store.UpdateMcpServer(r.Context(), server)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tools": tools})
}
