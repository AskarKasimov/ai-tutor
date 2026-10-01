package postgres

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// Migrate serializes initial schema creation, including across API processes.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(1790762400)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY)"); err != nil {
		return err
	}
	var applied bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=1)").Scan(&applied); err != nil {
		return err
	}
	if !applied {
		if _, err = tx.Exec(ctx, schemaSQL); err != nil {
			return fmt.Errorf("migration 1: %w", err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES(1)"); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
