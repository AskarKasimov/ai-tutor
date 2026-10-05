package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type memoryRepository struct {
	calls      int
	actorID    string
	parsed     competencymap.Map
	importedAt int64
	result     competencymap.ImportResult
	err        error
}

func (r *memoryRepository) Replace(_ context.Context, actorID string, parsed competencymap.Map, importedAt int64) (competencymap.ImportResult, error) {
	r.calls++
	r.actorID = actorID
	r.parsed = parsed
	r.importedAt = importedAt
	return r.result, r.err
}

type memoryParser struct {
	calls  int
	data   []byte
	parsed competencymap.Map
	err    error
}

func (p *memoryParser) Parse(data []byte) (competencymap.Map, error) {
	p.calls++
	p.data = data
	return p.parsed, p.err
}

func TestUnauthorizedImportDoesNotParseOrReplace(t *testing.T) {
	repo, parser := &memoryRepository{}, &memoryParser{}
	service := New(repo, parser, time.Now)
	_, err := service.Import(context.Background(), user.User{Role: user.Student}, nil, "text/csv")
	var failure *fault.Error
	if !errors.As(err, &failure) || failure.Kind != fault.Forbidden || parser.calls != 0 || repo.calls != 0 {
		t.Fatalf("unauthorized import: error=%v parser=%d repository=%d", err, parser.calls, repo.calls)
	}
}

func TestParserFailureDoesNotReplace(t *testing.T) {
	parseErr := errors.New("invalid CSV")
	repo, parser := &memoryRepository{}, &memoryParser{err: parseErr}
	service := New(repo, parser, time.Now)
	_, err := service.Import(context.Background(), user.User{Role: user.Admin}, []byte("broken"), "text/csv")
	if !errors.Is(err, parseErr) || parser.calls != 1 || repo.calls != 0 {
		t.Fatalf("parser failure: error=%v parser=%d repository=%d", err, parser.calls, repo.calls)
	}
}

func TestImportPropagatesMapActorAndClock(t *testing.T) {
	parsed := competencymap.Map{Competencies: []competencymap.Competency{{Key: "c", Name: "Competency"}}}
	result := competencymap.ImportResult{Revision: 7, ImportedAt: 123, CompetencyCount: 1}
	repo, parser := &memoryRepository{result: result}, &memoryParser{parsed: parsed}
	service := New(repo, parser, func() time.Time { return time.Unix(123, 0) })
	data := []byte("source CSV")
	got, err := service.Import(context.Background(), user.User{ID: "admin-1", Role: user.Admin}, data, "text/csv; charset=utf-8")
	if err != nil || !reflect.DeepEqual(got, result) || repo.calls != 1 || repo.actorID != "admin-1" || repo.importedAt != 123 || !reflect.DeepEqual(repo.parsed, parsed) || !reflect.DeepEqual(parser.data, data) {
		t.Fatalf("import: result=%+v error=%v repository=%+v parser=%+v", got, err, repo, parser)
	}
}

func TestUnsupportedMediaDoesNotParseOrReplace(t *testing.T) {
	repo, parser := &memoryRepository{}, &memoryParser{}
	_, err := New(repo, parser, time.Now).Import(context.Background(), user.User{Role: user.Admin}, nil, "application/json")
	var failure *fault.Error
	if !errors.As(err, &failure) || failure.Kind != fault.Unsupported || parser.calls != 0 || repo.calls != 0 {
		t.Fatalf("unsupported media: %v", err)
	}
}
