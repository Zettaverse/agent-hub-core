package auth

import "testing"

func TestRequiredRoleMatrix(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		path     string
		required string
	}{
		{"viewer can GET agents", "GET", "/api/v1/agents", RoleViewer},
		{"viewer can GET agent", "GET", "/api/v1/agents/abc-123", RoleViewer},
		{"viewer can GET flows", "GET", "/api/v1/flows", RoleViewer},
		{"viewer can GET runs", "GET", "/api/v1/runs/run-1", RoleViewer},
		{"viewer can GET dashboard", "GET", "/api/v1/dashboard", RoleViewer},
		{"operator can POST run", "POST", "/api/v1/flows/abc/run", RoleOperator},
		{"operator can POST task", "POST", "/api/v1/tasks", RoleOperator},
		{"owner required for POST agents", "POST", "/api/v1/agents", RoleOwner},
		{"owner required for POST mcp-servers", "POST", "/api/v1/mcp-servers", RoleOwner},
		{"owner required for POST flows", "POST", "/api/v1/flows", RoleOwner},
		{"owner required for POST users", "POST", "/api/v1/users", RoleOwner},
		{"owner required for PUT agents", "PUT", "/api/v1/agents/abc", RoleOwner},
		{"owner required for DELETE flows", "DELETE", "/api/v1/flows/abc", RoleOwner},
		{"login is public", "POST", "/api/v1/auth/login", RoleViewer},
		{"health is public", "GET", "/healthz", RoleViewer},
		{"metrics is public", "GET", "/metrics", RoleViewer},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RequiredRole(tc.method, tc.path)
			if got != tc.required {
				t.Fatalf("RequiredRole(%s %s) = %q, want %q", tc.method, tc.path, got, tc.required)
			}
		})
	}
}

func TestAllowedByRole(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		path     string
		viewer   bool
		operator bool
		owner    bool
	}{
		{"GET agents", "GET", "/api/v1/agents", true, true, true},
		{"POST run", "POST", "/api/v1/flows/f1/run", false, true, true},
		{"POST task", "POST", "/api/v1/tasks", false, true, true},
		{"POST agents", "POST", "/api/v1/agents", false, false, true},
		{"PUT agents", "PUT", "/api/v1/agents/a1", false, false, true},
		{"DELETE agents", "DELETE", "/api/v1/agents/a1", false, false, true},
		{"POST users", "POST", "/api/v1/users", false, false, true},
		{"POST flows", "POST", "/api/v1/flows", false, false, true},
		{"POST mcp-servers", "POST", "/api/v1/mcp-servers", false, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Allowed(tc.method, tc.path, RoleViewer); got != tc.viewer {
				t.Fatalf("viewer Allowed(%s %s) = %v, want %v", tc.method, tc.path, got, tc.viewer)
			}
			if got := Allowed(tc.method, tc.path, RoleOperator); got != tc.operator {
				t.Fatalf("operator Allowed(%s %s) = %v, want %v", tc.method, tc.path, got, tc.operator)
			}
			if got := Allowed(tc.method, tc.path, RoleOwner); got != tc.owner {
				t.Fatalf("owner Allowed(%s %s) = %v, want %v", tc.method, tc.path, got, tc.owner)
			}
		})
	}
}

func TestHasRole(t *testing.T) {
	if !HasRole(RoleOwner, RoleOperator) {
		t.Fatal("owner should have operator privileges")
	}
	if !HasRole(RoleOperator, RoleViewer) {
		t.Fatal("operator should have viewer privileges")
	}
	if HasRole(RoleViewer, RoleOwner) {
		t.Fatal("viewer must not have owner privileges")
	}
	if HasRole(RoleOperator, RoleOwner) {
		t.Fatal("operator must not have owner privileges")
	}
	if HasRole("unknown", RoleViewer) {
		t.Fatal("unknown role must not pass")
	}
}
