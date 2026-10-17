package postgres

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWaitReadyRetriesUntilDeadline(t *testing.T) {
	// A closed local port refuses connections the way a starting database does.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	pool, err := pgxpool.New(context.Background(), "postgres://user@"+addr+"/db?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := WaitReady(ctx, pool, 50*time.Millisecond); err == nil {
		t.Fatal("WaitReady succeeded without a database")
	}
	if elapsed := time.Since(start); elapsed < 250*time.Millisecond {
		t.Fatalf("WaitReady gave up after %v, before the deadline", elapsed)
	}
}

func TestWaitReadyReturnsWhenDatabaseAccepts(t *testing.T) {
	pool := migrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := WaitReady(ctx, pool, 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
}
