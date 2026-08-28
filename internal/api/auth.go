package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/Zettaverse/agent-hub-core/internal/auth"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

// defaultTenantID is the tenant used for the seeded owner account and for
// username/password login (the login payload does not carry a tenant).
const defaultTenantID = "00000000-0000-0000-0000-000000000001"

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	s.ensureSeed(r.Context())

	user, err := s.Store.GetUserByUsername(r.Context(), defaultTenantID, req.Username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := s.Auth.Issue(user.Username, user.ID, user.TenantID, string(user.Role))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := s.Store.GetUser(r.Context(), claims.TenantID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// ensureSeed creates the default tenant and an owner account when the user
// store is empty.
func (s *Server) ensureSeed(ctx context.Context) {
	users, err := s.Store.ListUsers(ctx, defaultTenantID)
	if err != nil || len(users) > 0 {
		return
	}
	if _, err := s.Store.GetTenant(ctx, defaultTenantID); err != nil {
		_, _ = s.Store.CreateTenant(ctx, store.Tenant{ID: defaultTenantID, Name: "default", CreatedAt: now()})
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.Config.SeedOwnerPassword), bcrypt.DefaultCost)
	if err != nil {
		return
	}
	_, _ = s.Store.CreateUser(ctx, store.User{
		ID:           uuid.NewString(),
		TenantID:     defaultTenantID,
		Username:     s.Config.SeedOwnerUsername,
		PasswordHash: string(hash),
		Role:         store.RoleOwner,
		CreatedAt:    now(),
		UpdatedAt:    now(),
	})
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.Store.ListUsers(r.Context(), tenantFrom(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	user, err := s.Store.GetUser(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	role := store.Role(req.Role)
	if role != store.RoleViewer && role != store.RoleOperator && role != store.RoleOwner {
		role = store.RoleViewer
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	user := store.User{
		ID:           uuid.NewString(),
		TenantID:     tenantFrom(r),
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    now(),
		UpdatedAt:    now(),
	}
	created, err := s.Store.CreateUser(r.Context(), user)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.Store.GetUser(r.Context(), tenantFrom(r), id)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Username != "" {
		existing.Username = req.Username
	}
	if req.Role != "" {
		if role := store.Role(req.Role); role == store.RoleViewer || role == store.RoleOperator || role == store.RoleOwner {
			existing.Role = role
		}
	}
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		existing.PasswordHash = string(hash)
	}
	existing.UpdatedAt = now()
	updated, err := s.Store.UpdateUser(r.Context(), existing)
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	err := s.Store.DeleteUser(r.Context(), tenantFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		status, msg := mapStoreError(err)
		writeError(w, status, msg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
