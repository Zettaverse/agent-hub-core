package store

import (
	"strings"
	"testing"
)

func TestMigrationsLoaded(t *testing.T) {
	statements, err := loadMigrationStatements()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(statements) == 0 {
		t.Fatal("expected embedded migration statements to be loaded")
	}
	joined := strings.Join(statements, "\n")
	for _, table := range []string{"tenants", "users", "agents", "mcp_servers", "flows", "runs", "tasks"} {
		if !strings.Contains(joined, "CREATE TABLE") || !strings.Contains(joined, table) {
			t.Fatalf("migrations missing table %q", table)
		}
	}
}
