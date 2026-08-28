package auth

import (
	"context"
	"net/http"
	"strings"
)

// Authenticator is a chi-style middleware that extracts a Bearer token,
// verifies it, and stores claims in the request context. Requests to paths
// that do not require authentication are passed through unchanged.
func (m *Manager) Authenticator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		token := bearerToken(r)
		if token == "" {
			http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
			return
		}
		claims, err := m.Verify(token)
		if err != nil {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}
		ctx := WithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Authorizer enforces the RBAC matrix after authentication.
func Authorizer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		claims, ok := ClaimsFrom(r.Context())
		if !ok {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if !Allowed(r.Method, r.URL.Path, claims.Role) {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the token from the Authorization header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// requiresAuth reports whether a path must be authenticated.
func requiresAuth(path string) bool {
	switch {
	case path == "/healthz", path == "/readyz", path == "/metrics":
		return false
	case path == "/api/v1/auth/login":
		return false
	default:
		return true
	}
}

// contextKey is used for the request ID.
type requestIDKey struct{}

// RequestIDFrom extracts the request id from the context.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithRequestID stores the request id in the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
