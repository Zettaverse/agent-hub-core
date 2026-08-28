// Package api wires the chi router, middleware, and HTTP handlers.
package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Zettaverse/agent-hub-core/internal/auth"
	"github.com/Zettaverse/agent-hub-core/internal/config"
	"github.com/Zettaverse/agent-hub-core/internal/flow"
	"github.com/Zettaverse/agent-hub-core/internal/mcp"
	"github.com/Zettaverse/agent-hub-core/internal/metrics"
	"github.com/Zettaverse/agent-hub-core/internal/orchestrator"
	"github.com/Zettaverse/agent-hub-core/internal/store"
	"github.com/Zettaverse/agent-hub-core/internal/ws"
)

// Server holds all dependencies shared by the HTTP handlers.
type Server struct {
	Store        store.Store
	Auth         *auth.Manager
	MCP          mcp.Client
	Orchestrator *orchestrator.Orchestrator
	Flow         *flow.Engine
	Hub          *ws.Hub
	Metrics      *metrics.Metrics
	Logger       *slog.Logger
	Config       config.Config
}

// NewServer assembles a Server from a Store and configuration.
func NewServer(cfg config.Config, st store.Store, logger *slog.Logger) *Server {
	mcpClient := mcp.NewClient()
	sideEffects := flow.NewMemorySideEffectStore()
	return &Server{
		Store:        st,
		Auth:         auth.NewManager(cfg.JWTSecret, cfg.JWTExpiry),
		MCP:          mcpClient,
		Orchestrator: orchestrator.New(st, mcpClient, orchestrator.WithWorkers(cfg.TaskWorkerPool), orchestrator.WithTimeout(cfg.TaskTimeout)),
		Flow:         flow.NewEngine(st, mcpClient, sideEffects),
		Hub:          ws.NewHub(),
		Metrics:      metrics.New(),
		Logger:       logger,
		Config:       cfg,
	}
}

// Router builds the fully-wired HTTP handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.requestIDMiddleware)
	r.Use(s.loggingMiddleware)
	r.Use(s.metricsMiddleware)

	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)
	r.Get("/metrics", promhttp.HandlerFor(s.Metrics.Registry, promhttp.HandlerOpts{}).ServeHTTP)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Get("/ws", s.handleWS)

		r.Group(func(r chi.Router) {
			r.Use(s.Auth.Authenticator)
			r.Use(auth.Authorizer)

			r.Get("/me", s.handleMe)

			// Agents
			r.Get("/agents", s.listAgents)
			r.Post("/agents", s.createAgent)
			r.Get("/agents/{id}", s.getAgent)
			r.Put("/agents/{id}", s.updateAgent)
			r.Delete("/agents/{id}", s.deleteAgent)

			// MCP servers
			r.Get("/mcp-servers", s.listMcpServers)
			r.Post("/mcp-servers", s.createMcpServer)
			r.Get("/mcp-servers/{id}", s.getMcpServer)
			r.Put("/mcp-servers/{id}", s.updateMcpServer)
			r.Delete("/mcp-servers/{id}", s.deleteMcpServer)
			r.Post("/mcp-servers/{id}/test", s.testMcpServer)

			// Flows
			r.Get("/flows", s.listFlows)
			r.Post("/flows", s.createFlow)
			r.Get("/flows/{id}", s.getFlow)
			r.Put("/flows/{id}", s.updateFlow)
			r.Delete("/flows/{id}", s.deleteFlow)
			r.Post("/flows/{id}/run", s.runFlow)
			r.Get("/flows/{id}/runs", s.listFlowRuns)

			// Runs
			r.Get("/runs/{run_id}", s.getRun)

			// Tasks
			r.Post("/tasks", s.createTask)
			r.Get("/tasks/{id}", s.getTask)

			// Users (owner only via RBAC)
			r.Get("/users", s.listUsers)
			r.Post("/users", s.createUser)
			r.Get("/users/{id}", s.getUser)
			r.Put("/users/{id}", s.updateUser)
			r.Delete("/users/{id}", s.deleteUser)

			// Dashboard
			r.Get("/dashboard", s.handleDashboard)
		})
	})

	return r
}

// --- Middleware ---

type responseRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// Hijack delegates to the underlying ResponseWriter so WebSocket upgrades
// continue to work through the middleware chain.
func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
}

// Flush delegates to the underlying ResponseWriter when it supports it.
func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := auth.WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		dur := time.Since(start)
		s.Logger.Info("http request",
			"request_id", auth.RequestIDFrom(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", float64(dur.Microseconds())/1000.0,
		)
	})
}

func (s *Server) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		status := httpStatusLabel(rec.status)
		s.Metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
		s.Metrics.HTTPDuration.WithLabelValues(r.Method, r.URL.Path).Observe(time.Since(start).Seconds())
	})
}

func httpStatusLabel(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

// --- Health ---

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

func tenantFrom(r *http.Request) string {
	return auth.TenantIDFrom(r.Context())
}

func roleFrom(r *http.Request) string {
	return auth.RoleFrom(r.Context())
}

func mapStoreError(err error) (int, string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, store.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, store.ErrInvalid):
		return http.StatusBadRequest, "invalid input"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

func now() time.Time {
	return time.Now().UTC()
}
