package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var (
		dryRun      = flag.Bool("dry-run", false, "Simulate recovery without applying changes")
		stage       = flag.String("stage", "all", "Stage to recover (all, ocr, cleaning, graph)")
		materialID  = flag.String("material-id", "", "Optional filter by material ID")
		subjectID   = flag.String("subject-id", "", "Optional filter by subject ID")
		stalled     = flag.Bool("stalled", false, "Also recover stalled jobs with expired leases")
		token       = flag.String("token", "", "Expected lease token CAS when recovering stalled jobs")
		actor       = flag.String("actor", "", "Actor name/ID for audit log (defaults to USER env)")
		databaseURL = flag.String("database-url", "", "PostgreSQL database URL")
	)
	flag.Parse()

	dbURL := strings.TrimSpace(*databaseURL)
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("BACKEND_DATABASE_URL"))
	}
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	}
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/ai_tutor?sslmode=disable"
	}

	act := strings.TrimSpace(*actor)
	if act == "" {
		act = os.Getenv("USER")
	}
	if act == "" {
		act = "admin"
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to PostgreSQL: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error pinging PostgreSQL: %v\n", err)
		os.Exit(1)
	}

	opts := Options{
		DryRun:     *dryRun,
		Stage:      *stage,
		MaterialID: strings.TrimSpace(*materialID),
		SubjectID:  strings.TrimSpace(*subjectID),
		Stalled:    *stalled,
		Token:      strings.TrimSpace(*token),
		Actor:      act,
		Now:        time.Now(),
	}

	summary, err := RecoverJobs(ctx, pool, opts, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Recovery error: %v\n", err)
		os.Exit(1)
	}

	mode := "APPLIED"
	if opts.DryRun {
		mode = "DRY-RUN (no changes made)"
	}
	fmt.Fprintf(os.Stderr, "Recovery completed [%s]: %d candidate(s) found, %d recovered, %d skipped\n",
		mode, summary.CandidatesFound, summary.Recovered, summary.Skipped)
}
