package migrate_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sandbox0-ai/sandbox0/pkg/dbpool"
	"github.com/sandbox0-ai/sandbox0/pkg/metering"
	"github.com/sandbox0-ai/sandbox0/pkg/migrate"
)

type noopLogger struct{}

func (noopLogger) Printf(string, ...any) {}
func (noopLogger) Fatalf(string, ...any) {}

func TestUpWithSchemaRestoresPoolSearchPath(t *testing.T) {
	dbURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("missing INTEGRATION_DATABASE_URL or TEST_DATABASE_URL")
	}

	ctx := context.Background()
	appSchema := fmt.Sprintf("migrate_test_%s", strings.ReplaceAll(uuid.NewString(), "-", ""))

	pool, err := dbpool.New(ctx, dbpool.Options{
		DatabaseURL: dbURL,
		Schema:      appSchema,
	})
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", appSchema))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", metering.SchemaName))
	})

	migrationsDir := t.TempDir()
	migration := `-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS users;
`
	if err := os.WriteFile(filepath.Join(migrationsDir, "00001_create_users.sql"), []byte(migration), 0o644); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	if err := migrate.Up(ctx, pool, migrationsDir, migrate.WithSchema(appSchema)); err != nil {
		t.Fatalf("run app migrations: %v", err)
	}
	if err := metering.RunMigrations(ctx, pool, noopLogger{}); err != nil {
		t.Fatalf("run metering migrations: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("query users with restored search_path: %v", err)
	}
}
