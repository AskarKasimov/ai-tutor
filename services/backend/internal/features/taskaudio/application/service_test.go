package application

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type serviceReaderStub struct {
	asset      audioasset.Asset
	accessible audioasset.Asset
	err        error
	owner, id  string
}

func (r *serviceReaderStub) Get(context.Context, string) (audioasset.Asset, error) {
	return r.asset, r.err
}
func (r *serviceReaderStub) GetAccessible(_ context.Context, owner, id string) (audioasset.Asset, error) {
	r.owner, r.id = owner, id
	return r.accessible, r.err
}

type serviceStorageStub struct {
	bucket, key string
	body        io.ReadCloser
	info        ObjectInfo
	err         error
}

func (*serviceStorageStub) EnsureBucket(context.Context) error { return nil }
func (*serviceStorageStub) Stat(context.Context, string, string) (ObjectInfo, bool, error) {
	return ObjectInfo{}, false, nil
}
func (*serviceStorageStub) Put(context.Context, string, string, string, []byte) error { return nil }
func (s *serviceStorageStub) Open(_ context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error) {
	s.bucket, s.key = bucket, key
	return s.body, s.info, s.err
}

func TestOpenUsesPersistedBucketAndStreamsOnlyReadyAsset(t *testing.T) {
	bucket, audioURL := "old-bucket", "/task-audio/audio-1/file"
	reader := &serviceReaderStub{accessible: audioasset.Asset{ID: "audio-1", ObjectKey: "task-audio/v1/audio-1.wav", Bucket: &bucket, StorageURI: strptr("s3://old-bucket/task-audio/v1/audio-1.wav"), AudioURL: &audioURL, Status: audioasset.Ready}}
	storage := &serviceStorageStub{body: io.NopCloser(strings.NewReader("WAV bytes")), info: ObjectInfo{Size: 9, ContentType: "audio/wav", AssetID: "audio-1"}}
	service := NewService(reader, storage)
	body, info, err := service.Open(context.Background(), "owner-1", "audio-1")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || string(data) != "WAV bytes" || info.ContentType != "audio/wav" {
		t.Fatalf("body=%q info=%#v err=%v", data, info, err)
	}
	if storage.bucket != "old-bucket" || storage.key != reader.accessible.ObjectKey || reader.owner != "owner-1" || reader.id != "audio-1" {
		t.Fatalf("storage lookup bucket=%q key=%q owner=%q asset=%q", storage.bucket, storage.key, reader.owner, reader.id)
	}
}

func TestOpenRejectsNonReadyAndUnavailableStorage(t *testing.T) {
	for _, tc := range []struct {
		status audioasset.Status
		code   string
		kind   fault.Kind
	}{
		{audioasset.Pending, "AUDIO_NOT_READY", fault.Conflict},
		{audioasset.Processing, "AUDIO_NOT_READY", fault.Conflict},
		{audioasset.Failed, "AUDIO_GENERATION_FAILED", fault.Unavailable},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			reader := &serviceReaderStub{accessible: audioasset.Asset{ID: "asset-1", Status: tc.status}}
			service := NewService(reader, &serviceStorageStub{})
			_, _, err := service.Open(context.Background(), "owner-1", "asset-1")
			var appErr *fault.Error
			if !errors.As(err, &appErr) || appErr.Code != tc.code || appErr.Kind != tc.kind {
				t.Fatalf("open error=%v", err)
			}
		})
	}
	bucket := "stored"
	reader := &serviceReaderStub{accessible: audioasset.Asset{ID: "asset-1", Status: audioasset.Ready, Bucket: &bucket, ObjectKey: "key"}}
	service := NewService(reader, &serviceStorageStub{err: errors.New("S3 unavailable")})
	_, _, err := service.Open(context.Background(), "owner-1", "asset-1")
	var appErr *fault.Error
	if !errors.As(err, &appErr) || appErr.Code != "AUDIO_STORAGE_UNAVAILABLE" {
		t.Fatalf("storage outage error=%v", err)
	}
}

func TestMetadataReadsWithoutStorageAndReturnsNotFound(t *testing.T) {
	asset := audioasset.Asset{ID: "asset-1", Status: audioasset.Pending}
	service := NewService(&serviceReaderStub{asset: asset}, &serviceStorageStub{})
	got, err := service.Metadata(context.Background(), "asset-1")
	if err != nil || got.ID != asset.ID || got.Status != asset.Status {
		t.Fatalf("metadata=%#v err=%v", got, err)
	}
	reader := &serviceReaderStub{err: audioasset.ErrNotFound}
	service = NewService(reader, &serviceStorageStub{})
	_, _, err = service.Open(context.Background(), "owner-1", "unknown")
	var appErr *fault.Error
	if !errors.As(err, &appErr) || appErr.Kind != fault.NotFound {
		t.Fatalf("unknown asset error=%v", err)
	}
}

func strptr(value string) *string { return &value }
