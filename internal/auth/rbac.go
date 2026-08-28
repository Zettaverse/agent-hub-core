package auth

import "strings"

// Role constants, ordered from least to most privileged.
const (
	RoleViewer   = "viewer"
	RoleOperator = "operator"
	RoleOwner    = "owner"
)

// roleRank returns an integer privilege rank for comparison.
func roleRank(role string) int {
	switch role {
	case RoleOwner:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// HasRole reports whether the actor role is at least as privileged as the
// required role.
func HasRole(actor, required string) bool {
	return roleRank(actor) >= roleRank(required)
}

// RequiredRole returns the minimum role required to perform method against
// path. Non-/api/v1 paths (health, metrics, ws upgrade) are public.
func RequiredRole(method, path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if !strings.HasPrefix(path, "/api/v1/") {
		return RoleViewer
	}

	// Public endpoints.
	if method == httpMethodPost && path == "/api/v1/auth/login" {
		return RoleViewer
	}

	// Operator-capable write endpoints (run flows, create tasks).
	if method == httpMethodPost && path == "/api/v1/tasks" {
		return RoleOperator
	}
	if method == httpMethodPost && strings.HasSuffix(path, "/run") && strings.HasPrefix(path, "/api/v1/flows/") {
		return RoleOperator
	}

	// Read-only verbs are available to everyone.
	if method == httpMethodGet || method == httpMethodHead {
		return RoleViewer
	}

	// Every other write verb requires owner.
	return RoleOwner
}

// HTTP method constants to avoid importing net/http in the hot path.
const (
	httpMethodGet  = "GET"
	httpMethodHead = "HEAD"
	httpMethodPost = "POST"
)

// Allowed reports whether the given role may perform the request.
func Allowed(method, path, role string) bool {
	return HasRole(role, RequiredRole(method, path))
}
