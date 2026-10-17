package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

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
	Action         string `json:"action"` // "candidate_found", "dry_run_recover", "job_recovered", "skipped_token_mismatch", "skipped_active"
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

func RecoverJobs(ctx context.Context, db DB, opts Options, out io.Writer) (Summary, error) {
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

	var sum Summary

	logAudit := func(evt AuditEvent) {
		evt.Timestamp = opts.Now.UTC().Format(time.RFC3339)
		evt.Actor = opts.Actor
		evt.DryRun = opts.DryRun
		data, err := json.Marshal(evt)
		if err == nil && out != nil {
			_, _ = fmt.Fprintln(out, string(data))
		}
	}

	if stage == "all" || stage == "ocr" {
		s, err := recoverOCRJobs(ctx, db, opts, logAudit)
		if err != nil {
			return sum, fmt.Errorf("recover OCR jobs: %w", err)
		}
		sum.CandidatesFound += s.CandidatesFound
		sum.Recovered += s.Recovered
		sum.Skipped += s.Skipped
	}

	if stage == "all" || stage == "cleaning" {
		s, err := recoverCleaningJobs(ctx, db, opts, logAudit)
		if err != nil {
			return sum, fmt.Errorf("recover cleaning jobs: %w", err)
		}
		sum.CandidatesFound += s.CandidatesFound
		sum.Recovered += s.Recovered
		sum.Skipped += s.Skipped
	}

	if stage == "all" || stage == "graph" || stage == "course_graph" {
		s, err := recoverGraphJobs(ctx, db, opts, logAudit)
		if err != nil {
			return sum, fmt.Errorf("recover graph jobs: %w", err)
		}
		sum.CandidatesFound += s.CandidatesFound
		sum.Recovered += s.Recovered
		sum.Skipped += s.Skipped
	}

	return sum, nil
}

type jobRecord struct {
	id           string
	subjectID    string
	status       string
	stage        string
	attempts     int
	availableAt  int64
	leaseUntil   int64
	leaseToken   string
	errorMessage string
	updatedAt    int64
}

func recoverOCRJobs(ctx context.Context, db DB, opts Options, logAudit func(AuditEvent)) (Summary, error) {
	var sum Summary
	nowUnix := opts.Now.Unix()

	query := `
		SELECT j.material_id, m.subject_id, j.status, coalesce(j.stage, 'ocr'), j.attempts,
		       j.available_at, j.lease_until, j.lease_token, j.error_message, j.updated_at
		FROM material_ocr_jobs j
		JOIN subject_materials m ON m.id = j.material_id
		WHERE ($1 = '' OR j.material_id = $1)
		  AND ($2 = '' OR m.subject_id = $2)
		  AND (
		      j.status = 'failed'
		      OR ($3::bool AND j.status = 'processing' AND j.lease_until <= $4)
		  )
		ORDER BY j.updated_at ASC;`

	rows, err := db.Query(ctx, query, opts.MaterialID, opts.SubjectID, opts.Stalled, nowUnix)
	if err != nil {
		return sum, err
	}
	defer rows.Close()

	var jobs []jobRecord
	for rows.Next() {
		var j jobRecord
		if err := rows.Scan(&j.id, &j.subjectID, &j.status, &j.stage, &j.attempts,
			&j.availableAt, &j.leaseUntil, &j.leaseToken, &j.errorMessage, &j.updatedAt); err != nil {
			return sum, err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return sum, err
	}

	for _, j := range jobs {
		sum.CandidatesFound++

		if j.status == "processing" && opts.Token != "" && j.leaseToken != opts.Token {
			sum.Skipped++
			logAudit(AuditEvent{
				Action:         "skipped_token_mismatch",
				Stage:          "ocr",
				ID:             j.id,
				SubjectID:      j.subjectID,
				PreviousStatus: j.status,
				LeaseToken:     j.leaseToken,
				Details:        fmt.Sprintf("expected token %s, got %s", opts.Token, j.leaseToken),
			})
			continue
		}

		if opts.DryRun {
			sum.Recovered++
			logAudit(AuditEvent{
				Action:         "dry_run_recover",
				Stage:          "ocr",
				ID:             j.id,
				SubjectID:      j.subjectID,
				PreviousStatus: j.status,
				PreviousStage:  j.stage,
				Attempts:       j.attempts,
				LeaseToken:     j.leaseToken,
				ErrorMessage:   j.errorMessage,
				NewStatus:      "pending",
				Details:        "would reset job to pending and reset failed subject_materials status",
			})
			continue
		}

		var tag pgconn.CommandTag
		if j.status == "failed" {
			tag, err = db.Exec(ctx, `
				UPDATE material_ocr_jobs
				SET status = 'pending', attempts = 0, error_message = '', available_at = $2, lease_until = 0, lease_token = '', updated_at = $2
				WHERE material_id = $1 AND status = 'failed'`,
				j.id, nowUnix)
		} else {
			tag, err = db.Exec(ctx, `
				UPDATE material_ocr_jobs
				SET status = 'pending', attempts = 0, error_message = '', available_at = $2, lease_until = 0, lease_token = '', updated_at = $2
				WHERE material_id = $1 AND status = 'processing' AND lease_until <= $2 AND ($3 = '' OR lease_token = $3)`,
				j.id, nowUnix, opts.Token)
		}
		if err != nil {
			return sum, fmt.Errorf("update material_ocr_jobs %s: %w", j.id, err)
		}

		if tag.RowsAffected() > 0 {
			_, _ = db.Exec(ctx, `
				UPDATE subject_materials
				SET status = 'pending', error_message = NULL, updated_at = $2
				WHERE id = $1 AND status = 'failed'`,
				j.id, nowUnix)

			sum.Recovered++
			logAudit(AuditEvent{
				Action:         "job_recovered",
				Stage:          "ocr",
				ID:             j.id,
				SubjectID:      j.subjectID,
				PreviousStatus: j.status,
				PreviousStage:  j.stage,
				Attempts:       j.attempts,
				LeaseToken:     j.leaseToken,
				ErrorMessage:   j.errorMessage,
				NewStatus:      "pending",
			})
		} else {
			sum.Skipped++
		}
	}
	return sum, nil
}

func recoverCleaningJobs(ctx context.Context, db DB, opts Options, logAudit func(AuditEvent)) (Summary, error) {
	var sum Summary
	nowUnix := opts.Now.Unix()

	query := `
		SELECT j.material_id, m.subject_id, j.status, j.attempts,
		       j.available_at, j.lease_until, j.lease_token, j.error_message, j.updated_at
		FROM material_cleaning_jobs j
		JOIN subject_materials m ON m.id = j.material_id
		WHERE ($1 = '' OR j.material_id = $1)
		  AND ($2 = '' OR m.subject_id = $2)
		  AND (
		      j.status = 'failed'
		      OR ($3::bool AND j.status = 'processing' AND j.lease_until <= $4)
		  )
		ORDER BY j.updated_at ASC;`

	rows, err := db.Query(ctx, query, opts.MaterialID, opts.SubjectID, opts.Stalled, nowUnix)
	if err != nil {
		return sum, err
	}
	defer rows.Close()

	var jobs []jobRecord
	for rows.Next() {
		var j jobRecord
		if err := rows.Scan(&j.id, &j.subjectID, &j.status, &j.attempts,
			&j.availableAt, &j.leaseUntil, &j.leaseToken, &j.errorMessage, &j.updatedAt); err != nil {
			return sum, err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return sum, err
	}

	for _, j := range jobs {
		sum.CandidatesFound++

		if j.status == "processing" && opts.Token != "" && j.leaseToken != opts.Token {
			sum.Skipped++
			logAudit(AuditEvent{
				Action:         "skipped_token_mismatch",
				Stage:          "cleaning",
				ID:             j.id,
				SubjectID:      j.subjectID,
				PreviousStatus: j.status,
				LeaseToken:     j.leaseToken,
				Details:        fmt.Sprintf("expected token %s, got %s", opts.Token, j.leaseToken),
			})
			continue
		}

		if opts.DryRun {
			sum.Recovered++
			logAudit(AuditEvent{
				Action:         "dry_run_recover",
				Stage:          "cleaning",
				ID:             j.id,
				SubjectID:      j.subjectID,
				PreviousStatus: j.status,
				Attempts:       j.attempts,
				LeaseToken:     j.leaseToken,
				ErrorMessage:   j.errorMessage,
				NewStatus:      "pending",
				Details:        "would reset job to pending while reusing parsed markdown artifacts",
			})
			continue
		}

		var tag pgconn.CommandTag
		if j.status == "failed" {
			tag, err = db.Exec(ctx, `
				UPDATE material_cleaning_jobs
				SET status = 'pending', attempts = 0, error_message = '', available_at = $2, lease_until = 0, lease_token = '', updated_at = $2
				WHERE material_id = $1 AND status = 'failed'`,
				j.id, nowUnix)
		} else {
			tag, err = db.Exec(ctx, `
				UPDATE material_cleaning_jobs
				SET status = 'pending', attempts = 0, error_message = '', available_at = $2, lease_until = 0, lease_token = '', updated_at = $2
				WHERE material_id = $1 AND status = 'processing' AND lease_until <= $2 AND ($3 = '' OR lease_token = $3)`,
				j.id, nowUnix, opts.Token)
		}
		if err != nil {
			return sum, fmt.Errorf("update material_cleaning_jobs %s: %w", j.id, err)
		}

		if tag.RowsAffected() > 0 {
			sum.Recovered++
			logAudit(AuditEvent{
				Action:         "job_recovered",
				Stage:          "cleaning",
				ID:             j.id,
				SubjectID:      j.subjectID,
				PreviousStatus: j.status,
				Attempts:       j.attempts,
				LeaseToken:     j.leaseToken,
				ErrorMessage:   j.errorMessage,
				NewStatus:      "pending",
			})
		} else {
			sum.Skipped++
		}
	}
	return sum, nil
}

func recoverGraphJobs(ctx context.Context, db DB, opts Options, logAudit func(AuditEvent)) (Summary, error) {
	var sum Summary
	nowUnix := opts.Now.Unix()

	query := `
		SELECT j.subject_id, j.status, j.attempts,
		       j.available_at, j.lease_until, j.lease_token, j.error_message, j.updated_at
		FROM course_graph_jobs j
		WHERE ($1 = '' OR j.subject_id = $1)
		  AND (
		      j.status = 'failed'
		      OR ($2::bool AND j.status = 'processing' AND j.lease_until <= $3)
		  )
		ORDER BY j.updated_at ASC;`

	rows, err := db.Query(ctx, query, opts.SubjectID, opts.Stalled, nowUnix)
	if err != nil {
		return sum, err
	}
	defer rows.Close()

	var jobs []jobRecord
	for rows.Next() {
		var j jobRecord
		if err := rows.Scan(&j.subjectID, &j.status, &j.attempts,
			&j.availableAt, &j.leaseUntil, &j.leaseToken, &j.errorMessage, &j.updatedAt); err != nil {
			return sum, err
		}
		j.id = j.subjectID
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return sum, err
	}

	for _, j := range jobs {
		sum.CandidatesFound++

		if j.status == "processing" && opts.Token != "" && j.leaseToken != opts.Token {
			sum.Skipped++
			logAudit(AuditEvent{
				Action:         "skipped_token_mismatch",
				Stage:          "course_graph",
				ID:             j.subjectID,
				SubjectID:      j.subjectID,
				PreviousStatus: j.status,
				LeaseToken:     j.leaseToken,
				Details:        fmt.Sprintf("expected token %s, got %s", opts.Token, j.leaseToken),
			})
			continue
		}

		if opts.DryRun {
			sum.Recovered++
			logAudit(AuditEvent{
				Action:         "dry_run_recover",
				Stage:          "course_graph",
				ID:             j.subjectID,
				SubjectID:      j.subjectID,
				PreviousStatus: j.status,
				Attempts:       j.attempts,
				LeaseToken:     j.leaseToken,
				ErrorMessage:   j.errorMessage,
				NewStatus:      "pending",
				Details:        "would reset build job to pending while reusing cleaned topics/reports",
			})
			continue
		}

		var tag pgconn.CommandTag
		if j.status == "failed" {
			tag, err = db.Exec(ctx, `
				UPDATE course_graph_jobs
				SET status = 'pending', attempts = 0, error_message = '', available_at = $2, lease_until = 0, lease_token = '', updated_at = $2
				WHERE subject_id = $1 AND status = 'failed'`,
				j.subjectID, nowUnix)
		} else {
			tag, err = db.Exec(ctx, `
				UPDATE course_graph_jobs
				SET status = 'pending', attempts = 0, error_message = '', available_at = $2, lease_until = 0, lease_token = '', updated_at = $2
				WHERE subject_id = $1 AND status = 'processing' AND lease_until <= $2 AND ($3 = '' OR lease_token = $3)`,
				j.subjectID, nowUnix, opts.Token)
		}
		if err != nil {
			return sum, fmt.Errorf("update course_graph_jobs %s: %w", j.subjectID, err)
		}

		if tag.RowsAffected() > 0 {
			sum.Recovered++
			logAudit(AuditEvent{
				Action:         "job_recovered",
				Stage:          "course_graph",
				ID:             j.subjectID,
				SubjectID:      j.subjectID,
				PreviousStatus: j.status,
				Attempts:       j.attempts,
				LeaseToken:     j.leaseToken,
				ErrorMessage:   j.errorMessage,
				NewStatus:      "pending",
			})
		} else {
			sum.Skipped++
		}
	}
	return sum, nil
}
