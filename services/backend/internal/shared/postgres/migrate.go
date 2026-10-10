package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies embedded SQL migrations, serialized across API processes.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	files, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	return migrate(ctx, pool, files)
}

func migrate(ctx context.Context, pool *pgxpool.Pool, files fs.FS) error {
	// Use dedicated connections so session locks never leak into the API pool.
	// Copying its pgx config preserves search_path, TLS and connection settings.
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	// Probe frequently while other processes migrate. Goose defaults to a 5s
	// interval, which can leave concurrent startup callers waiting through many
	// migration rounds before they get a chance to acquire the advisory lock.
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, 60))
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files,
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
