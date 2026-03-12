package metering

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sandbox0-ai/sandbox0/pkg/metering/migrations"
	"github.com/sandbox0-ai/sandbox0/pkg/migrate"
)

type Logger interface {
	Printf(format string, args ...any)
	Fatalf(format string, args ...any)
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, logger Logger) error {
	if err := migrate.Up(ctx, pool, ".",
		migrate.WithBaseFS(migrations.FS),
		migrate.WithLogger(logger),
		migrate.WithSchema(SchemaName),
	); err != nil {
		return fmt.Errorf("run metering migrations: %w", err)
	}
	return nil
}
