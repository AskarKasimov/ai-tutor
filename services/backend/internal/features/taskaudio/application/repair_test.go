package application

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
)

type repairQueue struct {
	*fakeQueue
	claim    Claim
	claimOK  bool
	claimErr error
}

func (q *repairQueue) ClaimRepair(_ context.Context, assetID, token string, _ time.Duration) (Claim, bool, error) {
	if q.claimErr != nil || !q.claimOK || q.claim.Asset.ID != assetID {
		return Claim{}, false, q.claimErr
	}
	q.claim.Token = token
	q.claim.Asset.Status = audioasset.Processing
	return q.claim, true, nil
}

type repairStorage struct {
	*fakeStorage
	objectInfo   ObjectInfo
	objectFound  bool
	objectData   []byte
	openErr      error
	statBuckets  []string
	bucketExists bool
	bucketErr    error
}

func newRepairStorage(found bool, data []byte) *repairStorage {
	return &repairStorage{fakeStorage: &fakeStorage{}, objectFound: found, objectInfo: ObjectInfo{Size: int64(len(data)), ContentType: "audio/wav", AssetID: "asset-00"}, objectData: data}
}
func (s *repairStorage) Stat(_ context.Context, bucket, _ string) (ObjectInfo, bool, error) {
	s.statBuckets = append(s.statBuckets, bucket)
	if s.statErr != nil {
		return s.objectInfo, false, s.statErr
	}
	return s.objectInfo, s.objectFound, nil
}
func (s *repairStorage) BucketExists(context.Context, string) (bool, error) {
	return s.bucketExists, s.bucketErr
}

func TestRegenerateUsesPersistedBucketWhenExistingObjectIsHealthy(t *testing.T) {
	worker, queue, storage, synth := repairWorker(true, testWAV())
	previousBucket := "legacy-audio"
	queue.claim.Asset.Bucket = &previousBucket
	asset, err := worker.Regenerate(context.Background(), "asset-00")
	if err != nil {
		t.Fatal(err)
	}
	if synth.calls != 0 || storage.putCount != 0 {
		t.Fatalf("healthy object in saved bucket caused synth=%d put=%d", synth.calls, storage.putCount)
	}
	if asset.Bucket == nil || *asset.Bucket != previousBucket {
		t.Fatalf("repair returned bucket %v, want %q", asset.Bucket, previousBucket)
	}
	if got := queue.completeDetails[len(queue.completeDetails)-1].bucket; got != previousBucket {
		t.Fatalf("repair completed in bucket %q, want %q", got, previousBucket)
	}
	if len(storage.statBuckets) != 1 || storage.statBuckets[0] != previousBucket {
		t.Fatalf("HEAD buckets = %v, want [%s]", storage.statBuckets, previousBucket)
	}
}

func TestRegenerateFallsBackToConfiguredBucketWhenSavedBucketWasDeleted(t *testing.T) {
	worker, queue, storage, synth := repairWorker(false, nil)
	previousBucket := "deleted-audio"
	queue.claim.Asset.Bucket = &previousBucket
	storage.statErr = ErrBucketNotFound
	asset, err := worker.Regenerate(context.Background(), "asset-00")
	if err != nil {
		t.Fatal(err)
	}
	if synth.calls != 1 || storage.putCount != 1 {
		t.Fatalf("deleted saved bucket repair synth=%d put=%d", synth.calls, storage.putCount)
	}
	if asset.Bucket == nil || *asset.Bucket != "task-audio" {
		t.Fatalf("repair returned bucket %v, want configured bucket", asset.Bucket)
	}
}
func (s *repairStorage) Open(context.Context, string, string) (io.ReadCloser, ObjectInfo, error) {
	if s.openErr != nil {
		return nil, s.objectInfo, s.openErr
	}
	return io.NopCloser(bytes.NewReader(s.objectData)), s.objectInfo, nil
}

func repairWorker(found bool, object []byte) (*Worker, *repairQueue, *repairStorage, *fakeSynth) {
	base := newFakeQueue(1)
	claim := Claim{Asset: audioasset.Asset{ID: "asset-00", Instruction: "Скажите ответ", ObjectKey: "task-audio/v1/asset-00.wav", Status: audioasset.Ready}}
	queue := &repairQueue{fakeQueue: base, claim: claim, claimOK: true}
	storage := newRepairStorage(found, object)
	synth := &fakeSynth{data: testWAV()}
	worker := NewWorker(queue, storage, synth, WorkerConfig{Concurrency: 1, PollInterval: time.Millisecond, VoiceTimeout: time.Second, S3Timeout: time.Second, Bucket: "task-audio"})
	return worker, queue, storage, synth
}

func TestRegenerateMissingObjectSynthesizesAndStoresOnce(t *testing.T) {
	worker, queue, storage, synth := repairWorker(false, nil)
	asset, err := worker.Regenerate(context.Background(), "asset-00")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Status != audioasset.Ready || synth.calls != 1 || storage.putCount != 1 || len(queue.completed) != 1 {
		t.Fatalf("repair asset=%+v synth=%d put=%d complete=%d", asset, synth.calls, storage.putCount, len(queue.completed))
	}
}

func TestRegenerateHealthyObjectNeverSynthesizes(t *testing.T) {
	worker, queue, storage, synth := repairWorker(true, testWAV())
	if _, err := worker.Regenerate(context.Background(), "asset-00"); err != nil {
		t.Fatal(err)
	}
	if synth.calls != 0 || storage.putCount != 0 || len(queue.completed) != 1 {
		t.Fatalf("healthy repair synth=%d put=%d complete=%d", synth.calls, storage.putCount, len(queue.completed))
	}
}

func TestRegenerateCorruptWAVSynthesizesAndStorageErrorsDoNot(t *testing.T) {
	t.Run("corrupt object", func(t *testing.T) {
		worker, _, storage, synth := repairWorker(true, []byte("broken"))
		if _, err := worker.Regenerate(context.Background(), "asset-00"); err != nil {
			t.Fatal(err)
		}
		if synth.calls != 1 || storage.putCount != 1 {
			t.Fatalf("corrupt repair synth=%d put=%d", synth.calls, storage.putCount)
		}
	})
	t.Run("HEAD forbidden", func(t *testing.T) {
		worker, _, storage, synth := repairWorker(false, nil)
		storage.statErr = errors.New("403 Forbidden")
		if _, err := worker.Regenerate(context.Background(), "asset-00"); err == nil {
			t.Fatal("expected storage error")
		}
		if synth.calls != 0 || storage.putCount != 0 {
			t.Fatalf("storage error caused synth=%d put=%d", synth.calls, storage.putCount)
		}
	})
	t.Run("GET unavailable", func(t *testing.T) {
		worker, _, storage, synth := repairWorker(true, testWAV())
		storage.openErr = errors.New("500 Internal Server Error")
		if _, err := worker.Regenerate(context.Background(), "asset-00"); err == nil {
			t.Fatal("expected storage error")
		}
		if synth.calls != 0 || storage.putCount != 0 {
			t.Fatalf("storage error caused synth=%d put=%d", synth.calls, storage.putCount)
		}
	})
}
