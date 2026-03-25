//go:build integration

package integration

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMain runs database migrations against the test Postgres before any test
// executes, then tears down cleanly on exit.
func TestMain(m *testing.M) {
	flag.Parse()

	if err := applyMigrations(*dbURL); err != nil {
		fmt.Fprintf(os.Stderr, "integration: migrations failed: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// applyMigrations reads every *.up.sql file from deploy/migrations (sorted by
// filename) and executes them against the test database.  Already-existing
// objects (IF NOT EXISTS / duplicate key on schema_migrations) are ignored so
// the function is idempotent.
func applyMigrations(dsn string) error {
	// Resolve the migrations directory relative to this source file.
	// main_test.go is at  api-gateway/tests/integration/main_test.go
	// migrations are at   deploy/migrations/  (three levels up from this file)
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..")
	migrationsDir := filepath.Join(repoRoot, "deploy", "migrations")

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir %q: %w", migrationsDir, err)
	}

	// Collect *.up.sql files in lexicographic order.
	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, filepath.Join(migrationsDir, e.Name()))
		}
	}
	sort.Strings(upFiles)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return fmt.Errorf("pgxpool: %w", err)
	}
	defer pool.Close()

	for _, f := range upFiles {
		sql, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			// Ignore "already exists" — migrations are idempotent in our schema.
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("exec %s: %w", filepath.Base(f), err)
			}
		}
	}
	return nil
}
