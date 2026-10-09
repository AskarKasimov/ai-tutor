package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
	trainingapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/application"
	trainingpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/infrastructure/postgres"
	trainingsnapshot "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/infrastructure/snapshot"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTrainingPersistsCyclesAndReplaysAnswers(t *testing.T) {
	f := newFixture(t)
	f.app.cfg.APIMode = "mock"
	f.handler = f.app.Handler()
	access, _, owner := f.register(t, "training@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/subjects/subject:intro-to-ml/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	post := func(path, key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		r.AddCookie(access)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		return w
	}
	v := post("/variants", "variant", `{"subject_id":"subject:intro-to-ml"}`)
	var header struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(v.Body.Bytes(), &header); err != nil {
		t.Fatal(err)
	}
	start := post("/diagnostic-sessions", "start", `{"variant_id":"`+header.ID+`"}`)
	var progress struct {
		ID string `json:"session_id"`
	}
	json.Unmarshal(start.Body.Bytes(), &progress)
	session, err := f.app.diagnosticStore.Get(t.Context(), owner, progress.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireCode(t, post("/diagnostic-sessions/"+session.ID+"/training", "active-start", `{}`), 409, "DIAGNOSTIC_SESSION_ACTIVE")
	task := session.Variant.Competencies[0].Tasks[0]
	if _, _, err = f.app.diagnosticStore.Reserve(t.Context(), owner, session.ID, "diag-answer", "digest", task.ID, "token"); err != nil {
		t.Fatal(err)
	}
	_, err = f.app.diagnosticStore.Accept(t.Context(), owner, session.ID, "token", "diag-answer", "digest", diagnostic.Answer{VariantTaskID: task.ID, CompetencyID: task.CompetencyID, OutcomeID: task.OutcomeID, Role: "main", Score: 1, Task: task}, diagnostic.Transition{Status: diagnostic.StatusCompleted, CurrentCompetency: 1})
	if err != nil {
		t.Fatal(err)
	}
	preview := f.request("GET", "/diagnostic-sessions/"+session.ID+"/training/preview", "", access)
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), "partial_competencies") {
		t.Fatalf("preview %d %s", preview.Code, preview.Body.String())
	}
	created := post("/diagnostic-sessions/"+session.ID+"/training", "training-start", `{}`)
	if created.Code != 201 {
		t.Fatalf("training create %d %s", created.Code, created.Body.String())
	}
	same := post("/diagnostic-sessions/"+session.ID+"/training", "another-start-key", `{}`)
	if same.Code != 200 || same.Body.String() != created.Body.String() {
		t.Fatalf("one training violated: %d %s", same.Code, same.Body.String())
	}
	requireCode(t, post("/diagnostic-sessions/missing/training", "training-start", `{}`), 409, "IDEMPOTENCY_KEY_REUSED")
	var training struct {
		ID      string `json:"session_id"`
		Round   int64  `json:"round"`
		Current struct {
			ID string `json:"exercise_id"`
		} `json:"current"`
	}
	if err = json.Unmarshal(created.Body.Bytes(), &training); err != nil {
		t.Fatal(err)
	}
	original := training.Current.ID
	data, media := diagnosticAnswerBody(t, original)
	body := strings.ReplaceAll(data.String(), "variant_task_id", "exercise_id")
	answer := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/training-sessions/"+training.ID+"/answers", strings.NewReader(body))
		r.Header.Set("Content-Type", media)
		r.Header.Set("Idempotency-Key", "answer-1")
		r.AddCookie(access)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		return w
	}
	accepted := answer()
	if accepted.Code != http.StatusOK {
		t.Fatalf("answer %d %s", accepted.Code, accepted.Body.String())
	}
	if err = json.Unmarshal(accepted.Body.Bytes(), &training); err != nil {
		t.Fatal(err)
	}
	if training.Round != 2 || training.Current.ID == original {
		t.Fatalf("no cycle: %s", accepted.Body.String())
	}
	f.handler = f.app.Handler()
	replay := answer()
	if replay.Code != 200 || replay.Body.String() != accepted.Body.String() {
		t.Fatalf("replay differs %d %s", replay.Code, replay.Body.String())
	}
	history := f.request("GET", "/training-sessions/"+training.ID+"/history?limit=1", "", access)
	if history.Code != 200 || !strings.Contains(history.Body.String(), "exercise_id") {
		t.Fatalf("history %d %s", history.Code, history.Body.String())
	}
	if strings.Contains(created.Body.String(), "reference_answer") || strings.Contains(created.Body.String(), "criteria") {
		t.Fatal("private snapshot leaked")
	}
	other, _, _ := f.register(t, "training-other@example.edu")
	if w := f.request("GET", "/training-sessions/"+training.ID, "", other); w.Code != 404 {
		t.Fatalf("foreign %d", w.Code)
	}
}

type blockedPractice struct {
	*trainingpg.Repository
	entered chan struct{}
	release chan struct{}
}

func (r *blockedPractice) Practice(ctx context.Context, subjectID string) (int64, []training.Task, error) {
	close(r.entered)
	select {
	case <-r.release:
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	}
	return r.Repository.Practice(ctx, subjectID)
}
func TestConcurrentStartReplaysExistingTrainingBeforeStalePreview(t *testing.T) {
	f, _, admin, owner, d := trainingFixture(t, 2)
	repo := trainingpg.New(f.pool)
	revision, _, err := repo.Practice(t.Context(), d.Variant.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	blocked := &blockedPractice{Repository: repo, entered: make(chan struct{}), release: make(chan struct{})}
	first := trainingapp.New(blocked, f.app.diagnosticStore, nil, nil, trainingsnapshot.Provider{}, security.IDGenerator{}, f.app.now)
	second := trainingapp.New(repo, f.app.diagnosticStore, nil, nil, trainingsnapshot.Provider{}, security.IDGenerator{}, f.app.now)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	type result struct {
		p   training.Progress
		err error
	}
	done := make(chan result, 1)
	go func() { p, _, err := first.Start(ctx, owner, d.ID, "same-key", &revision); done <- result{p, err} }()
	select {
	case <-blocked.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	accepted, _, err := second.Start(ctx, owner, d.ID, "same-key", &revision)
	if err != nil {
		close(blocked.release)
		t.Fatal(err)
	}
	if w := upload(f, "/admin/subjects/subject:intro-to-ml/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != 200 {
		close(blocked.release)
		t.Fatal(w.Body.String())
	}
	close(blocked.release)
	select {
	case actual := <-done:
		if actual.err != nil || actual.p.ID != accepted.ID {
			t.Fatalf("concurrent replay: %+v %v", actual.p, actual.err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestFreePracticeRotatesAllSavedTasksOfOneOutcome(t *testing.T) {
	f, _, admin, owner, d := trainingFixture(t, 2)
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO tasks(id,outcome_id,question,voice_instruction,reference_answer,created_at) SELECT 'alternate-task',id,'Другой вопрос','Ответьте','Ответ',1 FROM outcomes WHERE name='Знает параметры данных'`); err != nil {
		t.Fatal(err)
	}
	repo := trainingpg.New(f.pool)
	service := trainingapp.New(repo, f.app.diagnosticStore, &trainingTestVoice{}, &trainingTestGrader{}, trainingsnapshot.Provider{}, security.IDGenerator{}, f.app.now)
	preview, err := service.Preview(t.Context(), owner, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	progress, _, err := service.Start(t.Context(), owner, d.ID, "start", &preview.PlanRevision)
	if err != nil {
		t.Fatal(err)
	}
	if w := upload(f, "/admin/subjects/subject:intro-to-ml/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for i := 0; i < len(progress.Targets); i++ {
		progress, err = service.Answer(t.Context(), owner, progress.ID, progress.Current.ID, fmt.Sprintf("answer-%d", i), wavBytes(), "audio/wav")
		if err != nil {
			t.Fatal(err)
		}
	}
	if progress.Round != 2 || progress.Current.Question != "Другой вопрос" {
		t.Fatalf("rotation %+v", progress)
	}
}

type trainingTestVoice struct {
	calls int
	fail  bool
}

func (v *trainingTestVoice) Transcribe(_ context.Context, owner string, _ []byte, _ string) (transcription.Transcription, error) {
	v.calls++
	if v.fail {
		return transcription.Transcription{}, fmt.Errorf("STT failed")
	}
	return transcription.Transcription{ID: "test-transcript", OwnerID: owner, Text: "Ответ"}, nil
}

type trainingTestGrader struct{ fail bool }

func (g *trainingTestGrader) EvaluateTraining(_ context.Context, _, _ string, _ training.Exercise) (assessment.Evaluation, error) {
	if g.fail {
		return assessment.Evaluation{}, fmt.Errorf("grader failed")
	}
	return assessment.Evaluation{Score: 1, MaxScore: 2, Verdict: "partial", Feedback: []string{"Итог", "Причина", "Совет"}, CriterionResults: []assessment.CriterionResult{}}, nil
}
func TestTrainingModelFailureDoesNotAdvanceAndRetryReusesTranscript(t *testing.T) {
	f, _, _, owner, d := trainingFixture(t, 1)
	repo := trainingpg.New(f.pool)
	voice := &trainingTestVoice{fail: true}
	grader := &trainingTestGrader{fail: true}
	service := trainingapp.New(repo, f.app.diagnosticStore, voice, grader, trainingsnapshot.Provider{}, security.IDGenerator{}, f.app.now)
	p, _, err := service.Start(t.Context(), owner, d.ID, "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Answer(t.Context(), owner, p.ID, p.Current.ID, "answer", wavBytes(), "audio/wav"); err == nil {
		t.Fatal("STT failure accepted")
	}
	current, err := service.Get(t.Context(), owner, p.ID)
	if err != nil || current.AnswerCount != 0 || current.Current.ID != p.Current.ID {
		t.Fatalf("STT moved %+v %v", current, err)
	}
	voice.fail = false
	if _, err = service.Answer(t.Context(), owner, p.ID, p.Current.ID, "answer", wavBytes(), "audio/wav"); err == nil {
		t.Fatal("grader failure accepted")
	}
	calls := voice.calls
	grader.fail = false
	result, err := service.Answer(t.Context(), owner, p.ID, p.Current.ID, "answer", wavBytes(), "audio/wav")
	if err != nil || result.AnswerCount != 1 || voice.calls != calls {
		t.Fatalf("retry %+v calls %d/%d err %v", result, calls, voice.calls, err)
	}
	original, err := f.app.diagnosticStore.Get(t.Context(), owner, d.ID)
	if err != nil || len(original.Answers) != 1 || original.Answers[0].Score != 1 {
		t.Fatalf("diagnostic changed %+v %v", original, err)
	}
}

func TestTrainingHistoryPaginationAndSnapshotAudioAfterReimport(t *testing.T) {
	f, access, admin, owner, d := trainingFixture(t, 1)
	created := trainingJSON(f, access, "/diagnostic-sessions/"+d.ID+"/training", "start", `{}`)
	var p training.Progress
	json.Unmarshal(created.Body.Bytes(), &p)
	state, err := trainingpg.New(f.pool).Get(t.Context(), owner, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	asset := *state.Current.Task.AudioAssetID
	if _, err = f.pool.Exec(t.Context(), `UPDATE audio_assets SET status='ready',bucket='task-audio-test',storage_uri='s3://task-audio-test/'||object_key,audio_url='/task-audio/'||id||'/file' WHERE id=$1`, asset); err != nil {
		t.Fatal(err)
	}
	if w := upload(f, "/admin/subjects/subject:intro-to-ml/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	before := f.request("GET", "/training-sessions/"+p.ID, "", access)
	if !strings.Contains(before.Body.String(), p.Current.ID) {
		t.Fatal("exercise changed on GET/reimport")
	}
	metadata := f.request("GET", "/training-sessions/"+p.ID+"/current/audio?exercise_id="+p.Current.ID, "", access)
	if metadata.Code != 200 || !strings.Contains(metadata.Body.String(), `"status":"ready"`) {
		t.Fatalf("metadata %d %s", metadata.Code, metadata.Body.String())
	}
	f.s3Mu.Lock()
	f.s3AssetID = asset
	f.s3Mu.Unlock()
	file := f.request("GET", "/task-audio/"+asset+"/file", "", access)
	if file.Code != 200 {
		t.Fatalf("historical audio %d %s", file.Code, file.Body.String())
	}
	other, _, _ := f.register(t, "audio-other@example.edu")
	if w := f.request("GET", "/task-audio/"+asset+"/file", "", other); w.Code != 404 {
		t.Fatalf("foreign historical audio %d", w.Code)
	}
	for i := 0; i < 3; i++ {
		data, media := diagnosticAnswerBody(t, p.Current.ID)
		r := httptest.NewRequest("POST", "/training-sessions/"+p.ID+"/answers", strings.NewReader(strings.ReplaceAll(data.String(), "variant_task_id", "exercise_id")))
		r.Header.Set("Content-Type", media)
		r.Header.Set("Idempotency-Key", fmt.Sprintf("answer-%d", i))
		r.AddCookie(access)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("answer %d %s", w.Code, w.Body.String())
		}
		json.Unmarshal(w.Body.Bytes(), &p)
	}
	seen := map[int64]bool{}
	cursor := ""
	for i := 0; i < 3; i++ {
		path := "/training-sessions/" + p.ID + "/history?limit=1"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		w := f.request("GET", path, "", access)
		var page struct {
			Items []training.Attempt `json:"items"`
			Next  string             `json:"next_cursor"`
		}
		if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 || len(page.Items) != 1 || seen[page.Items[0].Sequence] {
			t.Fatalf("page %d %s %v", w.Code, w.Body.String(), err)
		}
		seen[page.Items[0].Sequence] = true
		cursor = page.Next
	}
	if cursor != "" || len(seen) != 3 {
		t.Fatalf("history cursor %q seen %v", cursor, seen)
	}
}

func trainingFixture(t *testing.T, score int) (*fixture, *http.Cookie, *http.Cookie, string, diagnostic.Session) {
	t.Helper()
	f := newFixture(t)
	f.app.cfg.APIMode = "mock"
	f.handler = f.app.Handler()
	access, _, owner := f.register(t, "source@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/subjects/subject:intro-to-ml/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	post := func(path, key, body string) *httptest.ResponseRecorder {
		return trainingJSON(f, access, path, key, body)
	}
	v := post("/variants", "source-variant", `{"subject_id":"subject:intro-to-ml"}`)
	var vh struct {
		ID string `json:"id"`
	}
	json.Unmarshal(v.Body.Bytes(), &vh)
	d := post("/diagnostic-sessions", "source-start", `{"variant_id":"`+vh.ID+`"}`)
	var dh struct {
		ID string `json:"session_id"`
	}
	json.Unmarshal(d.Body.Bytes(), &dh)
	s, err := f.app.diagnosticStore.Get(t.Context(), owner, dh.ID)
	if err != nil {
		t.Fatal(err)
	}
	task := s.Variant.Competencies[0].Tasks[0]
	if _, _, err = f.app.diagnosticStore.Reserve(t.Context(), owner, s.ID, "answer", "digest", task.ID, "token"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.app.diagnosticStore.Accept(t.Context(), owner, s.ID, "token", "answer", "digest", diagnostic.Answer{VariantTaskID: task.ID, CompetencyID: task.CompetencyID, OutcomeID: task.OutcomeID, Role: "main", Score: score, Task: task}, diagnostic.Transition{Status: diagnostic.StatusCompleted, CurrentCompetency: 1}); err != nil {
		t.Fatal(err)
	}
	s, err = f.app.diagnosticStore.Get(t.Context(), owner, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	return f, access, admin, owner, s
}
func trainingJSON(f *fixture, access *http.Cookie, path, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.AddCookie(access)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func TestFreePracticeIncludesFalseAndStalePreviewDoesNotWrite(t *testing.T) {
	f, access, admin, _, d := trainingFixture(t, 2)
	if _, err := f.pool.Exec(t.Context(), `UPDATE outcomes SET include_in_test=false WHERE name='Знает параметры данных'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO tasks(id,outcome_id,question,voice_instruction,reference_answer,created_at) SELECT 'alternate-task',id,'Другой вопрос','Ответьте','Ответ',1 FROM outcomes WHERE name='Знает параметры данных'`); err != nil {
		t.Fatal(err)
	}
	w := f.request("GET", "/diagnostic-sessions/"+d.ID+"/training/preview", "", access)
	var p training.Preview
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || w.Code != 200 {
		t.Fatalf("preview %d %s %v", w.Code, w.Body.String(), err)
	}
	if p.Mode != "free_practice" || len(p.Topics) != 3 {
		t.Fatalf("FALSE excluded: %+v", p)
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM training_sessions").Scan(&count); err != nil || count != 0 {
		t.Fatalf("preview wrote: %d %v", count, err)
	}
	old := p.PlanRevision
	if w := upload(f, "/admin/subjects/subject:intro-to-ml/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	rejected := trainingJSON(f, access, "/diagnostic-sessions/"+d.ID+"/training", "free-start", fmt.Sprintf(`{"plan_revision":%d}`, old))
	requireCode(t, rejected, 409, "TRAINING_PREVIEW_STALE")
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM training_sessions").Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale wrote: %d %v", count, err)
	}
	w = f.request("GET", "/diagnostic-sessions/"+d.ID+"/training/preview", "", access)
	json.Unmarshal(w.Body.Bytes(), &p)
	created := trainingJSON(f, access, "/diagnostic-sessions/"+d.ID+"/training", "free-start", fmt.Sprintf(`{"plan_revision":%d}`, p.PlanRevision))
	if created.Code != 201 {
		t.Fatalf("free start %d %s", created.Code, created.Body.String())
	}
	if w := upload(f, "/admin/subjects/subject:intro-to-ml/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	replay := trainingJSON(f, access, "/diagnostic-sessions/"+d.ID+"/training", "free-start", fmt.Sprintf(`{"plan_revision":%d}`, old))
	if replay.Code != 200 || replay.Body.String() != created.Body.String() {
		t.Fatalf("start replay %d %s", replay.Code, replay.Body.String())
	}
}

func TestTrainingReservationLeaseConcurrencyAndRetryTranscription(t *testing.T) {
	f, access, _, owner, d := trainingFixture(t, 1)
	created := trainingJSON(f, access, "/diagnostic-sessions/"+d.ID+"/training", "start", `{}`)
	var p training.Progress
	json.Unmarshal(created.Body.Bytes(), &p)
	repo := trainingpg.New(f.pool)
	var wait sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, r, _, err := repo.Reserve(context.Background(), owner, p.ID, "answer", "digest", p.Current.ID, fmt.Sprintf("token-%d", i))
			mu.Lock()
			defer mu.Unlock()
			if err == nil && r != nil {
				success++
			}
		}(i)
	}
	wait.Wait()
	if success != 1 {
		t.Fatalf("reservations %d", success)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE training_sessions SET lease_until=now()-interval '1 second' WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	_, reserved, state, err := trainingpg.New(f.pool).Reserve(t.Context(), owner, p.ID, "answer", "digest", p.Current.ID, "new-token")
	if err != nil {
		t.Fatal(err)
	}
	reserved.TranscriptionID = "saved-transcription"
	reserved.Text = "stored text"
	if err = repo.Fail(t.Context(), owner, p.ID, *reserved); err != nil {
		t.Fatal(err)
	}
	_, retried, _, err := repo.Reserve(t.Context(), owner, p.ID, "answer", "digest", p.Current.ID, "retry-token")
	if err != nil || retried.TranscriptionID != "saved-transcription" {
		t.Fatalf("lost transcript %+v %v", retried, err)
	}
	_, _, _, err = repo.Reserve(t.Context(), owner, p.ID, "answer", "different", p.Current.ID, "mismatch")
	var failure *fault.Error
	if !errors.As(err, &failure) || failure.Kind != fault.Conflict {
		t.Fatalf("mismatch %v", err)
	}
	attempt := training.Attempt{Sequence: 1, ExerciseID: p.Current.ID}
	state.AnswerCount = 1
	state.Round = 2
	state.Current.ID = "next-exercise"
	state.Current.Round = 2
	if _, err = repo.Accept(t.Context(), owner, *reserved, state, attempt); err == nil {
		t.Fatal("stale token accepted")
	}
	if _, err = repo.Accept(t.Context(), owner, *retried, state, attempt); err != nil {
		t.Fatal(err)
	}
	replay, _, _, err := trainingpg.New(f.pool).Reserve(t.Context(), owner, p.ID, "answer", "digest", p.Current.ID, "replay")
	if err != nil || replay == nil || replay.AnswerCount != 1 {
		t.Fatalf("replay %+v %v", replay, err)
	}
}
