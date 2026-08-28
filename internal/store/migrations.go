package store

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Zettaverse/agent-hub-core/migrations"
)

// migrationStatements holds every migration, split into individual executable
// statements, in deterministic (filename) order. Loading is lazy so that
// embed/parse errors can be returned to callers rather than panicking.
var (
	migrationsOnce sync.Once
	migrationStmts []string
	migrationsErr  error
)

// loadMigrationStatements returns the embedded migration statements, loading
// and splitting them exactly once.
func loadMigrationStatements() ([]string, error) {
	migrationsOnce.Do(func() {
		migrationStmts, migrationsErr = parseMigrations()
	})
	return migrationStmts, migrationsErr
}

func parseMigrations() ([]string, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("store: read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var statements []string
	for _, name := range names {
		raw, err := migrations.FS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("store: read migration %s: %w", name, err)
		}
		for _, stmt := range strings.Split(string(raw), ";") {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			statements = append(statements, stmt)
		}
	}
	return statements, nil
}
