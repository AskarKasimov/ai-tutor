package application

import (
	"context"
	"errors"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type audioReaderStub struct {
	asset audioasset.Asset
	err   error
	calls int
}

func (r *audioReaderStub) Metadata(_ context.Context, id string) (audioasset.Asset, error) {
	r.calls++
	if r.err != nil {
		return audioasset.Asset{}, r.err
	}
	if r.asset.ID != id {
		return audioasset.Asset{}, audioasset.ErrNotFound
	}
	return r.asset, nil
}

func TestCurrentAudioReturnsStoredMetadataWithoutSynthesis(t *testing.T) {
	for _, status := range []audioasset.Status{audioasset.Ready, audioasset.Pending, audioasset.Processing, audioasset.Failed, audioasset.Cancelled} {
		t.Run(string(status), func(t *testing.T) {
			service, voice, _, variants := newTestService()
			assetID, audioURL := "audio-1", "/task-audio/audio-1/file"
			variants.value.Competencies[0].Tasks[0].Task.AudioAssetID = &assetID
			reader := &audioReaderStub{asset: audioasset.Asset{ID: assetID, Status: status}}
			if status == audioasset.Ready {
				reader.asset.AudioURL = &audioURL
			}
			service.WithAudioReader(reader)
			progress := service.start(t)
			for range 10 {
				metadata, err := service.CurrentAudio(context.Background(), "owner-1", progress.SessionID, progress.Current.ID)
				if err != nil {
					t.Fatal(err)
				}
				if metadata.VariantTaskID != progress.Current.ID || metadata.Status != status {
					t.Fatalf("metadata = %#v", metadata)
				}
				if status == audioasset.Ready {
					if metadata.AudioURL == nil || *metadata.AudioURL != audioURL {
						t.Fatalf("ready URL = %v", metadata.AudioURL)
					}
				} else if metadata.AudioURL != nil {
					t.Fatalf("non-ready URL = %v", metadata.AudioURL)
				}
			}
			if voice.syntheses != 0 {
				t.Fatalf("HTTP metadata path synthesized %d times", voice.syntheses)
			}
		})
	}
}

func TestCurrentAudioMissingAssetAndExpectedTaskGuard(t *testing.T) {
	service, _, _, variants := newTestService()
	progress := service.start(t)
	metadata, err := service.CurrentAudio(context.Background(), "owner-1", progress.SessionID, progress.Current.ID)
	if err != nil || metadata.Status != audioasset.Missing || metadata.AudioURL != nil {
		t.Fatalf("missing audio = %#v err=%v", metadata, err)
	}
	_, err = service.CurrentAudio(context.Background(), "owner-1", progress.SessionID, "stale-task")
	var appErr *fault.Error
	if !errors.As(err, &appErr) || appErr.Code != "DIAGNOSTIC_TASK_CHANGED" {
		t.Fatalf("task guard error = %v", err)
	}
	variants.value.Competencies[0].Tasks[0].Task.AudioAssetID = nil
}

func TestCurrentAudioReaderFailureAndSessionOwnership(t *testing.T) {
	service, _, _, variants := newTestService()
	assetID := "audio-1"
	variants.value.Competencies[0].Tasks[0].Task.AudioAssetID = &assetID
	reader := &audioReaderStub{err: errors.New("database offline")}
	service.WithAudioReader(reader)
	progress := service.start(t)
	if _, err := service.CurrentAudio(context.Background(), "owner-2", progress.SessionID, progress.Current.ID); err == nil {
		t.Fatal("foreign session was read")
	}
	if _, err := service.CurrentAudio(context.Background(), "owner-1", progress.SessionID, progress.Current.ID); err == nil {
		t.Fatal("reader failure was hidden")
	}
	if reader.calls != 1 {
		t.Fatalf("reader calls=%d", reader.calls)
	}
}

func TestCurrentAudioPreservesCompletedSessionConflict(t *testing.T) {
	service, _, _, variants := newTestService()
	snapshot, err := snapshotVariant(variants.value)
	if err != nil {
		t.Fatal(err)
	}
	completed := diagnostic.Session{ID: "completed-session", OwnerID: "owner-1", VariantID: variants.value.ID, Status: diagnostic.StatusCompleted, Variant: snapshot}
	if _, _, err = service.store.Create(context.Background(), completed.OwnerID, "completed-start", "digest", completed); err != nil {
		t.Fatal(err)
	}
	_, err = service.CurrentAudio(context.Background(), completed.OwnerID, completed.ID, "task-1")
	var appErr *fault.Error
	if !errors.As(err, &appErr) || appErr.Code != "DIAGNOSTIC_SESSION_COMPLETED" {
		t.Fatalf("completed session audio error = %v", err)
	}
}
