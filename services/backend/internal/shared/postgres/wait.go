package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WaitReady pings PostgreSQL until it accepts connections or ctx ends.
// pg_isready reports a database as healthy while it still runs crash recovery
// and rejects sessions (SQLSTATE 57P03), so startup must retry, not fail once.
func WaitReady(ctx context.Context, pool *pgxpool.Pool, interval time.Duration) error {
	for {
		err := pool.Ping(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for PostgreSQL: %w", err)
		case <-time.After(interval):
		}
	}
}
