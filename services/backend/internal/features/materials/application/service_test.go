package application_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/translator"
)

type memoryRepo struct {
	items map[string]material.Material
	jobs  []application.OCRJob
}

func (m *memoryRepo) List(ctx context.Context, subjectID string) ([]material.Material, error) {
	var res []material.Material
	for _, item := range m.items {
		if item.SubjectID == subjectID {
			res = append(res, item)
		}
	}
	return res, nil
}

func (m *memoryRepo) Get(ctx context.Context, id string) (material.Material, error) {
	item, ok := m.items[id]
	if !ok {
		return material.Material{}, material.ErrNotFound
	}
	return item, nil
}

func (m *memoryRepo) Insert(ctx context.Context, mat material.Material) (material.Material, error) {
	if m.items == nil {
		m.items = make(map[string]material.Material)
	}
	m.items[mat.ID] = mat
	if mat.Status == material.StatusPending {
		m.jobs = append(m.jobs, application.OCRJob{MaterialID: mat.ID, Token: "lease", Attempts: 1})
	}
	return mat, nil
}

func (m *memoryRepo) ClaimOCR(context.Context, string, time.Time) (application.OCRJob, error) {
	if len(m.jobs) == 0 {
		return application.OCRJob{}, nil
	}
	job := m.jobs[0]
	m.jobs = m.jobs[1:]
	return job, nil
}

func (m *memoryRepo) FinishOCR(_ context.Context, job application.OCRJob, ocrKey, key string, report json.RawMessage, text string, warning *string, now time.Time) (bool, error) {
	mat, ok := m.items[job.MaterialID]
	if !ok {
		return false, nil
	}
	var parsedReport map[string]any
	_ = json.Unmarshal(report, &parsedReport)
	mat.Status, mat.S3OCRKey, mat.S3ParsedKey, mat.OCRReport, mat.ParsedText, mat.UpdatedAt, mat.ErrorMessage = material.StatusParsed, ocrKey, key, parsedReport, text, now.Unix(), warning
	mat.CharCount = len([]rune(text))
	m.items[job.MaterialID] = mat
	return true, nil
}

func (m *memoryRepo) FailOCR(context.Context, application.OCRJob, string, time.Time) error {
	return nil
}

func (m *memoryRepo) Delete(ctx context.Context, subjectID, id string) (material.Material, error) {
	item := m.items[id]
	delete(m.items, id)
	return item, nil
}

type memoryS3 struct {
	objects map[string][]byte
}

func (s *memoryS3) Put(ctx context.Context, key, contentType string, data []byte) error {
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects[key] = data
	return nil
}

func (s *memoryS3) Get(ctx context.Context, key string) ([]byte, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, io.EOF
	}
	return data, nil
}

func (s *memoryS3) Open(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, "", 0, io.EOF
	}
	return io.NopCloser(strings.NewReader(string(data))), "application/pdf", int64(len(data)), nil
}

func (s *memoryS3) Delete(ctx context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

type stubOCR struct {
	markdown string
	err      error
}

func (o *stubOCR) Convert(ctx context.Context, filename string, data []byte) (string, error) {
	if o.err != nil {
		return "", o.err
	}
	if o.markdown != "" {
		return o.markdown, nil
	}
	return "## " + filename + "\n\nФормула: $y = Wx + b$", nil
}

func TestUploadMaterial(t *testing.T) {
	repo := &memoryRepo{}
	s3 := &memoryS3{}
	ocr := &stubOCR{markdown: "# Лекция 1\n\nТест $E = mc^2$"}
	fixedTime := time.Unix(1700000000, 0)
	admin := user.User{ID: "usr-admin", Role: user.Admin}
	student := user.User{ID: "usr-student", Role: user.Student}

	svc := application.New(repo, s3, ocr, func() time.Time { return fixedTime })

	// Test student authorization failure
	_, err := svc.Upload(context.Background(), student, "sub-123", application.UploadInput{
		Filename:    "lecture-1.pdf",
		ContentType: "application/pdf",
		Data:        []byte("%PDF-fake-content"),
	})
	if err == nil {
		t.Fatalf("expected error for student actor on upload, got nil")
	}

	mat, err := svc.Upload(context.Background(), admin, "sub-123", application.UploadInput{
		Filename:    "lecture-1.pdf",
		ContentType: "application/pdf",
		Data:        []byte("%PDF-fake-content"),
	})
	if err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}

	if mat.SubjectID != "sub-123" {
		t.Errorf("expected subject_id sub-123, got %s", mat.SubjectID)
	}
	if mat.Filename != "lecture-1.pdf" {
		t.Errorf("expected filename lecture-1.pdf, got %s", mat.Filename)
	}
	if mat.Status != material.StatusPending {
		t.Errorf("expected status pending, got %s", mat.Status)
	}
	if mat.ParsedText != "" || len(repo.jobs) != 1 {
		t.Errorf("upload should enqueue OCR without running it: text=%q jobs=%d", mat.ParsedText, len(repo.jobs))
	}
	if len(s3.objects) != 1 {
		t.Errorf("expected only raw object before OCR, got %d", len(s3.objects))
	}
	worker := application.NewOCRWorker(repo, s3, ocr, nil)
	if worked, err := worker.ProcessOne(context.Background()); err != nil || !worked {
		t.Fatalf("OCR worker: worked=%v err=%v", worked, err)
	}
	mat, _ = repo.Get(context.Background(), mat.ID)
	if mat.Status != material.StatusParsed || !strings.Contains(mat.ParsedText, "$E = mc^2$") {
		t.Fatalf("OCR result not persisted: %#v", mat)
	}

	// Verify listing by student fails
	if _, err := svc.List(context.Background(), student, "sub-123"); err == nil {
		t.Fatalf("expected error for student actor on list, got nil")
	}

	// Verify listing by admin
	list, err := svc.List(context.Background(), admin, "sub-123")
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 listed item, got %d (err: %v)", len(list), err)
	}

	// Verify delete by student fails
	if err := svc.Delete(context.Background(), student, "sub-123", mat.ID); err == nil {
		t.Fatalf("expected error for student actor on delete, got nil")
	}

	// Verify delete by admin
	err = svc.Delete(context.Background(), admin, "sub-123", mat.ID)
	if err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if len(s3.objects) != 0 {
		t.Errorf("expected S3 objects to be deleted, remaining: %d", len(s3.objects))
	}
}

type failingParsedS3 struct{ memoryS3 }

func (s *failingParsedS3) Put(ctx context.Context, key, contentType string, data []byte) error {
	if strings.Contains(key, "/parsed/") {
		return io.ErrUnexpectedEOF
	}
	return s.memoryS3.Put(ctx, key, contentType, data)
}

func TestFailedParsedStorageRetriesBackgroundOCR(t *testing.T) {
	repo := &memoryRepo{}
	storage := &failingParsedS3{}
	svc := application.New(repo, storage, &stubOCR{}, nil)
	mat, err := svc.Upload(context.Background(), user.User{Role: user.Admin}, "sub", application.UploadInput{Filename: "lecture.md", Data: []byte("Теория")})
	if err != nil {
		t.Fatalf("raw upload failed: %v", err)
	}
	worker := application.NewOCRWorker(repo, storage, &stubOCR{}, nil)
	if worked, err := worker.ProcessOne(context.Background()); !worked || err == nil {
		t.Fatalf("expected background parsed storage failure, worked=%v err=%v", worked, err)
	}
	if len(repo.items) != 1 || len(storage.objects) != 2 || mat.Status != material.StatusPending {
		t.Fatal("failed OCR attempt should preserve raw pending material")
	}
}

type stubTranslator struct {
	translateFn func(ctx context.Context, markdown string) (string, error)
	calls       int
}

func (st *stubTranslator) Translate(ctx context.Context, markdown string) (string, error) {
	st.calls++
	if st.translateFn != nil {
		return st.translateFn(ctx, markdown)
	}
	return "Перевод: " + markdown, nil
}

func TestUploadMaterialWithTranslation(t *testing.T) {
	repo := &memoryRepo{}
	s3 := &memoryS3{}
	ocr := &stubOCR{markdown: "# Lecture 1\n\nBreadth-first search algorithm."}
	fixedTime := time.Unix(1700000000, 0)
	admin := user.User{ID: "usr-admin", Role: user.Admin}

	trans := &stubTranslator{
		translateFn: func(ctx context.Context, markdown string) (string, error) {
			return "# Лекция 1\n\nАлгоритм поиска в ширину.", nil
		},
	}

	svc := application.New(repo, s3, ocr, func() time.Time { return fixedTime }).WithTranslator(trans)

	mat, err := svc.Upload(context.Background(), admin, "sub-cs", application.UploadInput{
		Filename:    "ai-lecture.pdf",
		ContentType: "application/pdf",
		Data:        []byte("%PDF-dummy"),
	})
	if err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}

	if trans.calls != 0 || mat.Status != material.StatusPending {
		t.Fatal("upload must enqueue OCR and translation asynchronously")
	}
	worker := application.NewOCRWorker(repo, s3, ocr, trans)
	if worked, err := worker.ProcessOne(context.Background()); !worked || err != nil {
		t.Fatalf("worker: worked=%v err=%v", worked, err)
	}
	mat, _ = repo.Get(context.Background(), mat.ID)
	if trans.calls != 1 || !strings.Contains(mat.ParsedText, "Алгоритм поиска в ширину") || !strings.Contains(mat.S3ParsedKey, "/translated/") {
		t.Fatalf("translation result missing: %#v calls=%d", mat, trans.calls)
	}
	if mat.S3OCRKey == "" || string(s3.objects[mat.S3OCRKey]) != ocr.markdown || len(mat.OCRReport) == 0 {
		t.Fatalf("original OCR/report not preserved alongside translation: key=%q report=%s", mat.S3OCRKey, mat.OCRReport)
	}

	// Verify S3 objects: raw upload, original OCR, translated parsed text.
	if len(s3.objects) != 3 {
		t.Errorf("expected raw, original OCR and translated objects in S3, got %d: %v", len(s3.objects), s3.objects)
	}

	// Verify Delete cleans up all source and derived S3 objects.
	err = svc.Delete(context.Background(), admin, "sub-cs", mat.ID)
	if err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if len(s3.objects) != 0 {
		t.Errorf("expected all S3 objects to be deleted, remaining: %d (%v)", len(s3.objects), s3.objects)
	}
}

func TestUploadMaterialTranslationFailureFallback(t *testing.T) {
	repo := &memoryRepo{}
	s3 := &memoryS3{}
	ocr := &stubOCR{markdown: "# Lecture 1\n\nOriginal English text."}
	fixedTime := time.Unix(1700000000, 0)
	admin := user.User{ID: "usr-admin", Role: user.Admin}

	trans := &stubTranslator{
		translateFn: func(ctx context.Context, markdown string) (string, error) {
			return "", io.ErrUnexpectedEOF
		},
	}

	svc := application.New(repo, s3, ocr, func() time.Time { return fixedTime }).WithTranslator(trans)

	mat, err := svc.Upload(context.Background(), admin, "sub-cs", application.UploadInput{
		Filename:    "ai-lecture.pdf",
		ContentType: "application/pdf",
		Data:        []byte("%PDF-dummy"),
	})
	if err != nil {
		t.Fatalf("upload should succeed even when translation fails: %v", err)
	}

	worker := application.NewOCRWorker(repo, s3, ocr, trans)
	if worked, err := worker.ProcessOne(context.Background()); !worked || err != nil {
		t.Fatalf("translation fallback should succeed: worked=%v err=%v", worked, err)
	}
	mat, _ = repo.Get(context.Background(), mat.ID)
	if mat.ParsedText != "# Lecture 1\n\nOriginal English text." || !strings.Contains(mat.S3ParsedKey, "/parsed/") || mat.Status != material.StatusParsed || mat.ErrorMessage == nil {
		t.Fatalf("translation fallback incorrect: %#v", mat)
	}
}

type echoCompleter struct{ calls int }

// Complete answers with the untranslated source, as a model that ignored the task.
func (e *echoCompleter) Complete(_ context.Context, _, prompt string) (string, error) {
	e.calls++
	if i := strings.LastIndex(prompt, "Исходник:\n\n"); i >= 0 {
		prompt = prompt[i+len("Исходник:\n\n"):]
	}
	body, _ := json.Marshal(map[string]string{"translation": prompt})
	return string(body), nil
}

func TestUntranslatedOCRKeepsSourceWithWarning(t *testing.T) {
	repo := &memoryRepo{}
	s3 := &memoryS3{}
	ocr := &stubOCR{markdown: "# Lecture 1\n\nSupervised learning maps an input to an output based on example input-output pairs."}
	admin := user.User{ID: "usr-admin", Role: user.Admin}
	svc := application.New(repo, s3, ocr, func() time.Time { return time.Unix(1700000000, 0) })
	mat, err := svc.Upload(context.Background(), admin, "sub-cs", application.UploadInput{
		Filename: "lecture.pdf", ContentType: "application/pdf", Data: []byte("%PDF-dummy"),
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &echoCompleter{}
	worker := application.NewOCRWorker(repo, s3, ocr, translator.NewWithCompleter(model, 1))
	if worked, err := worker.ProcessOne(context.Background()); !worked || err != nil {
		t.Fatalf("worker: worked=%v err=%v", worked, err)
	}
	mat, _ = repo.Get(context.Background(), mat.ID)
	warnings, _ := mat.OCRReport["pipeline_warnings"].([]any)
	if model.calls != 2 || mat.ParsedText != ocr.markdown || mat.ErrorMessage == nil || len(warnings) == 0 {
		t.Fatalf("untranslated OCR passed as translated: calls=%d warning=%v report=%v", model.calls, mat.ErrorMessage, mat.OCRReport)
	}
}

func TestMetadataAndReplacementPreserveDurablePipeline(t *testing.T) {
	ctx := context.Background()
	admin := user.User{Role: user.Admin}
	week, kind := int32(2), material.Kind("lecture")
	repo := &memoryRepo{items: map[string]material.Material{"m": {ID: "m", SubjectID: "s", Status: material.StatusParsed, ParsedText: "old", S3RawKey: "old-raw", S3ParsedKey: "old-parsed", Week: &week, Kind: &kind, UpdatedAt: 7}}}
	storage := &memoryS3{objects: map[string][]byte{"old-raw": []byte("old")}}
	svc := application.New(repo, storage, &stubOCR{err: io.ErrUnexpectedEOF}, func() time.Time { return time.Unix(10, 0) })
	nextWeek := int32(4)
	updated, err := svc.UpdateMetadata(ctx, admin, "s", "m", &nextWeek, nil)
	if err != nil || updated.UpdatedAt != 7 || updated.ParsedText != "old" || len(repo.jobs) != 0 {
		t.Fatalf("metadata reprocessed material: %+v %v", updated, err)
	}
	replaced, err := svc.Replace(ctx, admin, "s", "m", application.ReplaceInput{Filename: "new.pdf", Data: []byte("%PDF-1.7 new")})
	if err != nil || replaced.Status != material.StatusPending || replaced.ParsedText != "" || replaced.Week == nil || *replaced.Week != 4 || len(repo.jobs) != 1 {
		t.Fatalf("replacement must enqueue without synchronous OCR: %+v %v", replaced, err)
	}
	if _, err = svc.Replace(ctx, admin, "other", "m", application.ReplaceInput{Filename: "new.pdf", Data: []byte("%PDF-")}); err == nil {
		t.Fatal("replacement crossed subject boundary")
	}
}

func (m *memoryRepo) ListKinds(context.Context) ([]material.KindOption, error) {
	return []material.KindOption{{Code: "lecture", Label: "Лекция"}, {Code: "lab", Label: "Лабораторная работа"}, {Code: "tutorial", Label: "Семинар"}}, nil
}
func (m *memoryRepo) UpdateMetadata(_ context.Context, subjectID, id string, week *int32, kind *material.Kind) (material.Material, error) {
	v := m.items[id]
	if week != nil {
		v.Week = week
	}
	if kind != nil {
		v.Kind = kind
	}
	m.items[id] = v
	return v, nil
}
func (m *memoryRepo) ReplaceMaterial(_ context.Context, subjectID, id string, v material.Material) (material.Material, error) {
	m.items[id] = v
	m.jobs = append(m.jobs, application.OCRJob{MaterialID: id, Token: "new-lease", Attempts: 1})
	return v, nil
}
