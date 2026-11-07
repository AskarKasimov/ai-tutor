package application

import (
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audiopreparation"
)

func TestExamAudioProgressTracksUniquePublishedKeysAndReplacesVersion(t *testing.T) {
	progress := NewExamAudioProgress("voice-v1")
	key := func(text string) string { return text }
	progress.TrackPublished("course-a", "graph-1", []string{"q1", "q1", "q2"}, key, audiopreparation.StatusUnchecked)
	progress.Transition("q1", audiopreparation.StatusQueued)
	progress.Transition("q1", audiopreparation.StatusProcessing)
	progress.Transition("q1", audiopreparation.StatusReady)
	snapshot, ok := progress.Snapshot("course-a")
	if !ok || snapshot.Total != 2 || snapshot.Ready != 1 || snapshot.Unchecked != 1 || snapshot.Version != "graph-1|voice-v1" {
		t.Fatalf("snapshot = %+v, ok=%v", snapshot, ok)
	}
	if snapshot.Counts.Sum() != snapshot.Total {
		t.Fatalf("counts=%+v total=%d", snapshot.Counts, snapshot.Total)
	}

	progress.TrackPublished("course-a", "graph-2", []string{"q3"}, key, audiopreparation.StatusUnchecked)
	snapshot, _ = progress.Snapshot("course-a")
	if snapshot.Version != "graph-2|voice-v1" || snapshot.Total != 1 || snapshot.Ready != 0 || snapshot.Unchecked != 1 {
		t.Fatalf("replacement snapshot = %+v", snapshot)
	}
	if _, ok := progress.Snapshot("missing"); ok {
		t.Fatal("unexpected snapshot for untracked course")
	}
}

func TestTrackRestoredIsIdempotentForTheSameGraphVersion(t *testing.T) {
	progress := NewExamAudioProgress("voice-v1")
	key := func(text string) string { return text }
	if !progress.TrackRestored("course", "graph", []string{"question"}, key) {
		t.Fatal("initial graph registration was not recorded")
	}
	progress.TransitionSubject("course", "question", audiopreparation.StatusReady)
	before, _ := progress.Snapshot("course")
	if progress.TrackRestored("course", "graph", []string{"question"}, key) {
		t.Fatal("same graph version was registered again")
	}
	after, _ := progress.Snapshot("course")
	if after.Ready != 1 || after.ObservedAt != before.ObservedAt {
		t.Fatalf("duplicate restore changed progress: before=%+v after=%+v", before, after)
	}
}

func TestAudioProgressStatePrecedence(t *testing.T) {
	cases := []struct {
		name      string
		published bool
		total     int
		counts    audiopreparation.Counts
		expected  audiopreparation.State
	}{
		{"no graph", false, 3, audiopreparation.Counts{Queued: 3}, audiopreparation.NoGraph},
		{"empty", true, 0, audiopreparation.Counts{}, audiopreparation.Empty},
		{"checking", true, 2, audiopreparation.Counts{Unchecked: 1, Processing: 1}, audiopreparation.Checking},
		{"processing", true, 3, audiopreparation.Counts{Processing: 1, Queued: 1, Failed: 1}, audiopreparation.Processing},
		{"queued", true, 2, audiopreparation.Counts{Queued: 1, RetryWait: 1}, audiopreparation.Queued},
		{"retry", true, 1, audiopreparation.Counts{RetryWait: 1}, audiopreparation.RetryWait},
		{"partial", true, 1, audiopreparation.Counts{Unavailable: 1}, audiopreparation.Partial},
		{"ready", true, 1, audiopreparation.Counts{Ready: 1}, audiopreparation.Ready},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.counts.State(tc.published, tc.total); got != tc.expected {
				t.Fatalf("state=%s want %s", got, tc.expected)
			}
		})
	}
}
