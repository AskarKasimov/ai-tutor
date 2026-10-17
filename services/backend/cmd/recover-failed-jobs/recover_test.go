package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type mockDB struct {
	queries []string
	execs   []string
	rowsMap map[string][]jobRecord
	execRes map[string]int64
}

func (m *mockDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	m.queries = append(m.queries, sql)
	for key, rows := range m.rowsMap {
		if strings.Contains(sql, key) {
			return &mockRows{records: rows}, nil
		}
	}
	return &mockRows{}, nil
}

func (m *mockDB) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	m.execs = append(m.execs, sql)
	var count int64 = 1
	for key, n := range m.execRes {
		if strings.Contains(sql, key) {
			count = n
			break
		}
	}
	return pgconn.NewCommandTag(strings.Fields(sql)[0] + " " + string(rune('0'+count))), nil
}

type mockRows struct {
	records []jobRecord
	idx     int
	err     error
}

func (r *mockRows) Close()                                         {}
func (r *mockRows) Err() error                                     { return r.err }
func (r *mockRows) CommandTag() pgconn.CommandTag                  { return pgconn.CommandTag{} }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription   { return nil }
func (r *mockRows) Next() bool                                     { return r.idx < len(r.records) }
func (r *mockRows) RawValues() [][]byte                            { return nil }
func (r *mockRows) Conn() *pgx.Conn                                { return nil }
func (r *mockRows) Values() ([]any, error)                         { return nil, nil }
func (r *mockRows) TypeMap() *pgtype.Map                           { return pgtype.NewMap() }

func (r *mockRows) Scan(dest ...any) error {
	rec := r.records[r.idx]
	r.idx++
	// Depending on destination length:
	// OCR: 10 fields (id, subjectID, status, stage, attempts, availableAt, leaseUntil, leaseToken, errorMessage, updatedAt)
	// Cleaning: 9 fields (id, subjectID, status, attempts, availableAt, leaseUntil, leaseToken, errorMessage, updatedAt)
	// Graph: 8 fields (subjectID, status, attempts, availableAt, leaseUntil, leaseToken, errorMessage, updatedAt)
	if len(dest) == 10 {
		*dest[0].(*string) = rec.id
		*dest[1].(*string) = rec.subjectID
		*dest[2].(*string) = rec.status
		*dest[3].(*string) = rec.stage
		*dest[4].(*int) = rec.attempts
		*dest[5].(*int64) = rec.availableAt
		*dest[6].(*int64) = rec.leaseUntil
		*dest[7].(*string) = rec.leaseToken
		*dest[8].(*string) = rec.errorMessage
		*dest[9].(*int64) = rec.updatedAt
	} else if len(dest) == 9 {
		*dest[0].(*string) = rec.id
		*dest[1].(*string) = rec.subjectID
		*dest[2].(*string) = rec.status
		*dest[3].(*int) = rec.attempts
		*dest[4].(*int64) = rec.availableAt
		*dest[5].(*int64) = rec.leaseUntil
		*dest[6].(*string) = rec.leaseToken
		*dest[7].(*string) = rec.errorMessage
		*dest[8].(*int64) = rec.updatedAt
	} else if len(dest) == 8 {
		*dest[0].(*string) = rec.subjectID
		*dest[1].(*string) = rec.status
		*dest[2].(*int) = rec.attempts
		*dest[3].(*int64) = rec.availableAt
		*dest[4].(*int64) = rec.leaseUntil
		*dest[5].(*string) = rec.leaseToken
		*dest[6].(*string) = rec.errorMessage
		*dest[7].(*int64) = rec.updatedAt
	}
	return nil
}

func TestRecoverJobs_DryRun(t *testing.T) {
	mock := &mockDB{
		rowsMap: map[string][]jobRecord{
			"material_ocr_jobs": {
				{
					id:           "mat_ocr_1",
					subjectID:    "sub_1",
					status:       "failed",
					stage:        "ocr",
					attempts:     3,
					errorMessage: "timeout error",
				},
			},
			"material_cleaning_jobs": {
				{
					id:           "mat_clean_1",
					subjectID:    "sub_1",
					status:       "failed",
					attempts:     3,
					errorMessage: "cleaning error",
				},
			},
			"course_graph_jobs": {
				{
					id:           "sub_1",
					subjectID:    "sub_1",
					status:       "failed",
					attempts:     3,
					errorMessage: "graph build error",
				},
			},
		},
	}

	var buf bytes.Buffer
	opts := Options{
		DryRun: true,
		Stage:  "all",
		Actor:  "test_admin",
		Now:    time.Unix(1700000000, 0),
	}

	summary, err := RecoverJobs(context.Background(), mock, opts, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.CandidatesFound != 3 {
		t.Errorf("got %d candidates, want 3", summary.CandidatesFound)
	}
	if summary.Recovered != 3 {
		t.Errorf("got %d recovered, want 3", summary.Recovered)
	}
	// In DryRun, no UPDATE Exec calls must be made
	if len(mock.execs) != 0 {
		t.Errorf("expected 0 Exec calls in dry-run, got %d", len(mock.execs))
	}

	// Verify audit log JSON
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 audit lines, got %d", len(lines))
	}
	for _, line := range lines {
		var evt AuditEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Errorf("invalid json: %v", err)
		}
		if evt.Action != "dry_run_recover" {
			t.Errorf("action = %s, want dry_run_recover", evt.Action)
		}
		if !evt.DryRun {
			t.Errorf("dry_run = false, want true")
		}
		if evt.Actor != "test_admin" {
			t.Errorf("actor = %s, want test_admin", evt.Actor)
		}
	}
}

func TestRecoverJobs_ApplyExecution(t *testing.T) {
	mock := &mockDB{
		rowsMap: map[string][]jobRecord{
			"material_ocr_jobs": {
				{
					id:           "mat_ocr_1",
					subjectID:    "sub_1",
					status:       "failed",
					stage:        "ocr",
					attempts:     3,
					errorMessage: "ocr failure",
				},
			},
		},
	}

	var buf bytes.Buffer
	opts := Options{
		DryRun: false,
		Stage:  "ocr",
		Actor:  "admin",
		Now:    time.Unix(1700000000, 0),
	}

	summary, err := RecoverJobs(context.Background(), mock, opts, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.CandidatesFound != 1 || summary.Recovered != 1 {
		t.Fatalf("summary = %+v, want candidates=1, recovered=1", summary)
	}

	// In apply execution, Exec must have been called
	if len(mock.execs) == 0 {
		t.Fatal("expected Exec calls, got 0")
	}

	// Verify audit log has job_recovered
	var evt AuditEvent
	if err := json.Unmarshal(buf.Bytes(), &evt); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if evt.Action != "job_recovered" {
		t.Errorf("action = %s, want job_recovered", evt.Action)
	}
	if evt.NewStatus != "pending" {
		t.Errorf("new_status = %s, want pending", evt.NewStatus)
	}
}

func TestRecoverJobs_StalledTokenCAS(t *testing.T) {
	mock := &mockDB{
		rowsMap: map[string][]jobRecord{
			"material_cleaning_jobs": {
				{
					id:         "clean_1",
					subjectID:  "sub_1",
					status:     "processing",
					attempts:   1,
					leaseUntil: 1699999000, // expired lease
					leaseToken: "tok_old",
				},
			},
		},
	}

	var buf bytes.Buffer
	// Try with wrong token
	opts := Options{
		DryRun:  false,
		Stage:   "cleaning",
		Stalled: true,
		Token:   "tok_expected",
		Now:     time.Unix(1700000000, 0),
	}

	summary, err := RecoverJobs(context.Background(), mock, opts, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.CandidatesFound != 1 || summary.Skipped != 1 || summary.Recovered != 0 {
		t.Fatalf("summary = %+v, want candidate=1, skipped=1, recovered=0", summary)
	}

	// Verify skipped_token_mismatch was logged
	var evt AuditEvent
	if err := json.Unmarshal(buf.Bytes(), &evt); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if evt.Action != "skipped_token_mismatch" {
		t.Errorf("action = %s, want skipped_token_mismatch", evt.Action)
	}
}
