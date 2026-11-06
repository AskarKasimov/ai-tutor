package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audiopreparation"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/subject"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
)

type audioProgressRepo struct {
	revision int64
	assets   []audiopreparation.LearningAsset
	err      error
	calls    int
}

func (r *audioProgressRepo) Snapshot(context.Context, string) (int64, []audiopreparation.LearningAsset, error) {
	r.calls++
	return r.revision, r.assets, r.err
}

type examProgressFake struct {
	recovering bool
	voice      string
	voiceErr   error
	snapshot   audiopreparation.ExamSnapshot
	ok         bool
}

func (r *examProgressFake) Snapshot(string) (audiopreparation.ExamSnapshot, bool) {
	return r.snapshot, r.ok
}

func (r *examProgressFake) VoiceName(context.Context) (string, error) { return r.voice, r.voiceErr }

func (r *examProgressFake) RecoveryComplete() bool { return !r.recovering }

type subjectRepoStub struct {
	exists    bool
	published bool
}

func (subjectRepoStub) Create(context.Context, string, string, int64) (subject.Subject, error) {
	return subject.Subject{}, nil
}
func (subjectRepoStub) List(context.Context) ([]subject.Subject, error) { return nil, nil }
func (r subjectRepoStub) AudioPreparationState(context.Context, string) (bool, bool, error) {
	return r.exists, r.published, nil
}

type subjectIDsStub struct{}

func (subjectIDsStub) New(string) (string, error) { return "id", nil }

func TestAudioPreparationAggregatesCurrentLearningAssets(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Minute), now.Add(-time.Minute)
	status := func(value string) *string { return &value }
	asset := func(task, id, instruction, state string, attempts int, next, lease *time.Time) audiopreparation.LearningAsset {
		return audiopreparation.LearningAsset{TaskID: task, AssetID: &id, Instruction: instruction, Status: status(state), Attempts: attempts, NextAttemptAt: next, LeaseUntil: lease}
	}
	missing := audiopreparation.LearningAsset{TaskID: "task-missing", Instruction: "Speak this"}
	assets := []audiopreparation.LearningAsset{
		asset("t1", "a1", "Ready", "ready", 1, nil, nil),
		asset("t2", "a1", "Ready", "ready", 1, nil, nil), // duplicate asset counts once
		asset("t3", "a2", "Failed", "failed", 5, nil, nil),
		asset("t4", "a3", "Working", "processing", 1, nil, &future),
		asset("t5", "a4", "Expired", "processing", 1, nil, &past),
		asset("t6", "a5", "Retry", "pending", 2, &future, nil),
		asset("t7", "a6", "Queued", "pending", 0, &past, nil),
		asset("t8", "a7", "Cancelled", "cancelled", 5, nil, nil),
		missing,
		{TaskID: "ignored", Instruction: "  "},
	}
	learning := &audioProgressRepo{revision: 42, assets: assets}
	svc := New(subjectRepoStub{exists: true, published: true}, subjectIDsStub{}, func() time.Time { return now })
	svc.SetAudioProgress(&examProgressFake{snapshot: audiopreparation.ExamSnapshot{Version: "graph|voice", Total: 1, Counts: audiopreparation.Counts{Ready: 1}, ObservedAt: now}, ok: true}, learning)
	result, err := svc.AudioPreparation(context.Background(), user.User{Role: user.Admin}, "course-1")
	if err != nil {
		t.Fatal(err)
	}
	if learning.calls != 1 || result.SubjectID != "course-1" || result.Exam.State != audiopreparation.Ready || result.Learn.Revision != 42 {
		t.Fatalf("response=%+v calls=%d", result, learning.calls)
	}
	counts := result.Learn.Counts
	if result.Learn.Total != 8 || counts.Ready != 1 || counts.Failed != 1 || counts.Processing != 1 || counts.Queued != 2 || counts.RetryWait != 1 || counts.Unavailable != 2 || counts.Sum() != result.Learn.Total {
		t.Fatalf("learning total=%d counts=%+v", result.Learn.Total, counts)
	}
}

func TestAudioPreparationChecksAdminBeforeReading(t *testing.T) {
	learning := &audioProgressRepo{}
	svc := New(subjectRepoStub{exists: true}, subjectIDsStub{}, time.Now)
	svc.SetAudioProgress(&examProgressFake{}, learning)
	_, err := svc.AudioPreparation(context.Background(), user.User{Role: user.Student}, "course-1")
	if err == nil || learning.calls != 0 {
		t.Fatalf("error=%v learning reads=%d", err, learning.calls)
	}
}

func TestLearningAudioObservedAtChangesOnlyWithSnapshot(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	learning := &audioProgressRepo{revision: 1}
	svc := New(subjectRepoStub{exists: true}, subjectIDsStub{}, func() time.Time { return now })
	svc.SetAudioProgress(&examProgressFake{}, learning)
	actor := user.User{Role: user.Admin}
	first, err := svc.AudioPreparation(context.Background(), actor, "course")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	second, err := svc.AudioPreparation(context.Background(), actor, "course")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Learn.ObservedAt.Equal(second.Learn.ObservedAt) {
		t.Fatalf("unchanged snapshot changed timestamp: %v -> %v", first.Learn.ObservedAt, second.Learn.ObservedAt)
	}
	learning.revision = 2
	third, err := svc.AudioPreparation(context.Background(), actor, "course")
	if err != nil {
		t.Fatal(err)
	}
	if !third.Learn.ObservedAt.Equal(now) {
		t.Fatalf("changed snapshot timestamp = %v", third.Learn.ObservedAt)
	}
	now = now.Add(time.Minute)
	id, status := "asset", "ready"
	learning.assets = []audiopreparation.LearningAsset{{TaskID: "task", AssetID: &id, Status: &status, Instruction: "Вопрос"}}
	fourth, err := svc.AudioPreparation(context.Background(), actor, "course")
	if err != nil {
		t.Fatal(err)
	}
	if fourth.Learn.Ready != 1 || !fourth.Learn.ObservedAt.Equal(now) {
		t.Fatalf("changed counts snapshot = %+v", fourth.Learn)
	}
}

func TestAudioPreparationDoesNotReportNoGraphDuringRecovery(t *testing.T) {
	svc := New(subjectRepoStub{exists: true}, subjectIDsStub{}, time.Now)
	svc.SetAudioProgress(&examProgressFake{recovering: true}, &audioProgressRepo{})
	_, err := svc.AudioPreparation(context.Background(), user.User{Role: user.Admin}, "course")
	if err == nil {
		t.Fatal("expected an unavailable result while published graphs are being recovered")
	}
}

func TestNoGraphObservedAtChangesOnlyWhenAvailabilityChanges(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	repo := subjectRepoStub{exists: true}
	svc := New(repo, subjectIDsStub{}, func() time.Time { return now })
	svc.SetAudioProgress(&examProgressFake{}, &audioProgressRepo{})
	actor := user.User{Role: user.Admin}
	first, err := svc.AudioPreparation(context.Background(), actor, "course")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	second, err := svc.AudioPreparation(context.Background(), actor, "course")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Exam.ObservedAt.Equal(second.Exam.ObservedAt) {
		t.Fatalf("unchanged no_graph snapshot changed timestamp: %v -> %v", first.Exam.ObservedAt, second.Exam.ObservedAt)
	}
}

func TestAudioPreparationReturnsNotFoundForMissingSubject(t *testing.T) {
	svc := New(subjectRepoStub{}, subjectIDsStub{}, time.Now)
	svc.SetAudioProgress(&examProgressFake{}, &audioProgressRepo{})
	_, err := svc.AudioPreparation(context.Background(), user.User{Role: user.Admin}, "missing")
	if err == nil || err.Error() != "SUBJECT_NOT_FOUND" {
		t.Fatalf("error = %v, want SUBJECT_NOT_FOUND", err)
	}
}

func TestAudioPreparationHidesCachedExamProgressWhenGraphIsUnavailable(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	svc := New(subjectRepoStub{exists: true, published: false}, subjectIDsStub{}, func() time.Time { return now })
	svc.SetAudioProgress(&examProgressFake{snapshot: audiopreparation.ExamSnapshot{
		Version: "old-graph", Total: 3, Counts: audiopreparation.Counts{Ready: 3}, ObservedAt: now,
	}, ok: true}, &audioProgressRepo{})
	result, err := svc.AudioPreparation(context.Background(), user.User{Role: user.Admin}, "course")
	if err != nil {
		t.Fatal(err)
	}
	if result.Exam.Published || result.Exam.State != audiopreparation.NoGraph || result.Exam.Total != 0 {
		t.Fatalf("stale exam progress was exposed: %+v", result.Exam)
	}
}

func TestAudioPreparationRefreshesGraphPublishedAfterStartupRecovery(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	exam := &examProgressFake{}
	svc := New(subjectRepoStub{exists: true, published: true}, subjectIDsStub{}, func() time.Time { return now })
	svc.SetAudioProgress(exam, &audioProgressRepo{})
	refreshes := 0
	svc.SetExamAudioRefresh(func(context.Context, string) error {
		refreshes++
		exam.ok = true
		exam.snapshot = audiopreparation.ExamSnapshot{Version: "restored", Total: 1, Counts: audiopreparation.Counts{Queued: 1}, ObservedAt: now}
		return nil
	})
	result, err := svc.AudioPreparation(context.Background(), user.User{Role: user.Admin}, "course")
	if err != nil {
		t.Fatal(err)
	}
	if refreshes != 1 || result.Exam.State != audiopreparation.Queued || !result.Exam.Published {
		t.Fatalf("refreshes=%d exam=%+v", refreshes, result.Exam)
	}
}

type blockingLearningProgress struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingLearningProgress) Snapshot(ctx context.Context, id string) (int64, []audiopreparation.LearningAsset, error) {
	if id == "slow" {
		close(r.started)
		select {
		case <-r.release:
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		}
	}
	return 1, nil, nil
}

func TestLearningProgressSlowCourseDoesNotBlockOtherCourses(t *testing.T) {
	repo := &blockingLearningProgress{started: make(chan struct{}), release: make(chan struct{})}
	svc := New(subjectRepoStub{exists: true}, subjectIDsStub{}, time.Now)
	svc.SetAudioProgress(&examProgressFake{}, repo)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.AudioPreparation(context.Background(), user.User{Role: user.Admin}, "slow")
	}()
	<-repo.started
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	fast := make(chan error, 1)
	go func() { _, err := svc.AudioPreparation(ctx, user.User{Role: user.Admin}, "fast"); fast <- err }()
	select {
	case err := <-fast:
		if err != nil {
			t.Errorf("fast course: %v", err)
		}
	case <-ctx.Done():
		t.Error("slow course blocked another course")
	}
	close(repo.release)
	<-done
}

type orderedLearningProgress struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (r *orderedLearningProgress) Snapshot(context.Context, string) (int64, []audiopreparation.LearningAsset, error) {
	if r.calls.Add(1) == 1 {
		close(r.started)
		<-r.release
		return 1, nil, nil
	}
	return 2, nil, nil
}
func TestLearningProgressOlderReadDoesNotOverwriteNewerSnapshot(t *testing.T) {
	repo := &orderedLearningProgress{started: make(chan struct{}), release: make(chan struct{})}
	var clock atomic.Int64
	svc := New(subjectRepoStub{exists: true}, subjectIDsStub{}, func() time.Time { return time.Unix(clock.Add(1), 0) })
	svc.SetAudioProgress(&examProgressFake{}, repo)
	actor := user.User{Role: user.Admin}
	done := make(chan struct{})
	go func() { defer close(done); _, _ = svc.AudioPreparation(context.Background(), actor, "course") }()
	<-repo.started
	newer, err := svc.AudioPreparation(context.Background(), actor, "course")
	if err != nil {
		t.Fatal(err)
	}
	close(repo.release)
	<-done
	after, err := svc.AudioPreparation(context.Background(), actor, "course")
	if err != nil {
		t.Fatal(err)
	}
	if newer.Learn.Revision != 2 || !after.Learn.ObservedAt.Equal(newer.Learn.ObservedAt) {
		t.Fatalf("newer=%+v after=%+v", newer.Learn, after.Learn)
	}
}

func TestAudioPreparationUsesGatewayVoiceIdentity(t *testing.T) {
	exam := &examProgressFake{snapshot: audiopreparation.ExamSnapshot{Version: "graph|"}, ok: true, voice: "profile:revision"}
	svc := New(subjectRepoStub{exists: true, published: true}, subjectIDsStub{}, time.Now)
	svc.SetAudioProgress(exam, &audioProgressRepo{})
	response, err := svc.AudioPreparation(context.Background(), user.User{Role: user.Admin}, "course")
	if err != nil || response.Exam.Version != "graph|profile:revision" {
		t.Fatalf("version=%q err=%v", response.Exam.Version, err)
	}
	exam.voiceErr = errors.New("gateway unavailable")
	if _, err := svc.AudioPreparation(context.Background(), user.User{Role: user.Admin}, "course"); err == nil {
		t.Fatal("expected voice identity error")
	}
}
