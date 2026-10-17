package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Options struct {
	DryRun     bool
	Stage      string // "all", "ocr", "cleaning", "graph"
	MaterialID string
	SubjectID  string
	Stalled    bool
	Token      string
	Actor      string
	Now        time.Time
}

type AuditEvent struct {
	Timestamp      string `json:"timestamp"`
	Actor          string `json:"actor"`
	Action         string `json:"action"` // "dry_run_recover", "job_recovered", "skipped_token_mismatch", "skipped_changed"
	Stage          string `json:"stage"`  // "ocr", "cleaning", "course_graph"
	ID             string `json:"id"`
	SubjectID      string `json:"subject_id,omitempty"`
	PreviousStatus string `json:"previous_status"`
	PreviousStage  string `json:"previous_stage,omitempty"`
	Attempts       int    `json:"attempts"`
	LeaseToken     string `json:"lease_token,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	NewStatus      string `json:"new_status,omitempty"`
	DryRun         bool   `json:"dry_run"`
	Details        string `json:"details,omitempty"`
}

type Summary struct {
	CandidatesFound int
	Recovered       int
	Skipped         int
}

// candidate is one job selected for recovery; reset applies it with the
// status/lease check in SQL, so a job that changed meanwhile is skipped.
type candidate struct {
	event AuditEvent
	reset func(context.Context) (int64, error)
}

func RecoverJobs(ctx context.Context, conn db.DBTX, opts Options, out io.Writer) (Summary, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.Actor == "" {
		opts.Actor = "admin"
	}
	stage := strings.ToLower(strings.TrimSpace(opts.Stage))
	if stage == "" {
		stage = "all"
	}
	switch stage {
	case "all", "ocr", "cleaning", "graph", "course_graph":
	default:
		return Summary{}, fmt.Errorf("unknown stage %q", opts.Stage)
	}
	q := db.New(conn)
	now := opts.Now.Unix()

	var candidates []candidate
	if stage == "all" || stage == "ocr" {
		rows, err := q.ListRecoverableOCR(ctx, db.ListRecoverableOCRParams{MaterialID: opts.MaterialID, SubjectID: opts.SubjectID, Stalled: opts.Stalled, Now: now})
		if err != nil {
			return Summary{}, fmt.Errorf("recover OCR jobs: %w", err)
		}
		for _, r := range rows {
			id := r.MaterialID
			candidates = append(candidates, candidate{
				event: AuditEvent{Stage: "ocr", ID: id, SubjectID: r.SubjectID, PreviousStatus: r.Status, PreviousStage: r.Stage,
					Attempts: int(r.Attempts), LeaseToken: r.LeaseToken, ErrorMessage: r.ErrorMessage,
					Details: "reset job to pending and return a failed material to pending"},
				reset: func(ctx context.Context) (int64, error) {
					return q.RecoverOCR(ctx, db.RecoverOCRParams{MaterialID: id, Now: now, Token: opts.Token})
				},
			})
		}
	}
	if stage == "all" || stage == "cleaning" {
		rows, err := q.ListRecoverableCleaning(ctx, db.ListRecoverableCleaningParams{MaterialID: opts.MaterialID, SubjectID: opts.SubjectID, Stalled: opts.Stalled, Now: now})
		if err != nil {
			return Summary{}, fmt.Errorf("recover cleaning jobs: %w", err)
		}
		for _, r := range rows {
			id := r.MaterialID
			candidates = append(candidates, candidate{
				event: AuditEvent{Stage: "cleaning", ID: id, SubjectID: r.SubjectID, PreviousStatus: r.Status,
					Attempts: int(r.Attempts), LeaseToken: r.LeaseToken, ErrorMessage: r.ErrorMessage,
					Details: "reset job to pending while reusing parsed markdown"},
				reset: func(ctx context.Context) (int64, error) {
					return q.RecoverCleaning(ctx, db.RecoverCleaningParams{MaterialID: id, Now: now, Token: opts.Token})
				},
			})
		}
	}
	if stage == "all" || stage == "graph" || stage == "course_graph" {
		rows, err := q.ListRecoverableCourseGraphs(ctx, db.ListRecoverableCourseGraphsParams{MaterialID: opts.MaterialID, SubjectID: opts.SubjectID, Stalled: opts.Stalled, Now: now})
		if err != nil {
			return Summary{}, fmt.Errorf("recover graph jobs: %w", err)
		}
		for _, r := range rows {
			id := r.SubjectID
			candidates = append(candidates, candidate{
				event: AuditEvent{Stage: "course_graph", ID: id, SubjectID: id, PreviousStatus: r.Status,
					Attempts: int(r.Attempts), LeaseToken: r.LeaseToken, ErrorMessage: r.ErrorMessage,
					Details: "reset build job to pending while reusing cleaned topics"},
				reset: func(ctx context.Context) (int64, error) {
					return q.RecoverCourseGraph(ctx, db.RecoverCourseGraphParams{SubjectID: id, Now: now, Token: opts.Token})
				},
			})
		}
	}

	logAudit := func(evt AuditEvent) {
		evt.Timestamp = opts.Now.UTC().Format(time.RFC3339)
		evt.Actor = opts.Actor
		evt.DryRun = opts.DryRun
		data, err := json.Marshal(evt)
		if err == nil && out != nil {
			_, _ = fmt.Fprintln(out, string(data))
		}
	}
	var sum Summary
	for _, c := range candidates {
		sum.CandidatesFound++
		evt := c.event
		if evt.PreviousStatus == "processing" && opts.Token != "" && evt.LeaseToken != opts.Token {
			sum.Skipped++
			evt.Action, evt.Details = "skipped_token_mismatch", fmt.Sprintf("expected token %s, got %s", opts.Token, evt.LeaseToken)
			logAudit(evt)
			continue
		}
		evt.NewStatus = "pending"
		if opts.DryRun {
			sum.Recovered++
			evt.Action, evt.Details = "dry_run_recover", "would "+evt.Details
			logAudit(evt)
			continue
		}
		n, err := c.reset(ctx)
		if err != nil {
			return sum, fmt.Errorf("recover %s job %s: %w", evt.Stage, evt.ID, err)
		}
		if n == 0 {
			sum.Skipped++
			evt.Action, evt.NewStatus, evt.Details = "skipped_changed", "", "job changed since it was selected"
			logAudit(evt)
			continue
		}
		sum.Recovered++
		evt.Action, evt.Details = "job_recovered", ""
		logAudit(evt)
	}
	return sum, nil
}
