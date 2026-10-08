package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/application"
)

func TestS3PutStatAndOpenUseBucketKeyAndAssetMetadata(t *testing.T) {
	const wav = "RIFF\x04\x00\x00\x00WAVE"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/task-audio/task-audio/v1/asset-1.wav" {
			t.Errorf("path = %q", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("Content-Type") != "audio/wav" || r.Header.Get("X-Amz-Meta-Audio-Asset-Id") != "asset-1" {
				t.Errorf("PUT headers = %#v", r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != wav {
				t.Errorf("PUT body = %q", body)
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("Content-Length", "12")
			w.Header().Set("X-Amz-Meta-Audio-Asset-Id", "asset-1")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("Content-Length", "12")
			w.Header().Set("X-Amz-Meta-Audio-Asset-Id", "asset-1")
			_, _ = w.Write([]byte(wav))
		default:
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	storage, err := New(Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "task-audio", AccessKey: "key", SecretKey: "secret", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := storage.Put(ctx, "task-audio", "task-audio/v1/asset-1.wav", "asset-1", []byte(wav)); err != nil {
		t.Fatal(err)
	}
	info, found, err := storage.Stat(ctx, "task-audio", "task-audio/v1/asset-1.wav")
	if err != nil || !found || info.ContentType != "audio/wav" || info.Size != 12 || info.AssetID != "asset-1" {
		t.Fatalf("stat = %#v found=%v err=%v", info, found, err)
	}
	body, info, err := storage.Open(ctx, "task-audio", "task-audio/v1/asset-1.wav")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil || !bytes.Equal(got, []byte(wav)) || info.ContentType != "audio/wav" {
		t.Fatalf("open = %q info=%#v err=%v", got, info, err)
	}
}

func TestS3StatDistinguishesMissingFromStorageErrors(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			storage, err := New(Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "task-audio", AccessKey: "key", SecretKey: "secret", PathStyle: true})
			if err != nil {
				t.Fatal(err)
			}
			_, found, err := storage.Stat(context.Background(), "task-audio", "missing.wav")
			if status == http.StatusNotFound {
				if err != nil || found {
					t.Fatalf("404 stat = found %v err %v", found, err)
				}
			} else if err == nil || found {
				t.Fatalf("status %d treated as missing: found %v err %v", status, found, err)
			}
		})
	}
}

func TestS3StatIdentifiesDeletedBucketSeparatelyFromMissingObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `<Error><Code>NoSuchBucket</Code><Message>missing</Message></Error>`)
	}))
	defer server.Close()
	storage, err := New(Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "task-audio", AccessKey: "key", SecretKey: "secret", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := storage.Stat(context.Background(), "legacy-audio", "missing.wav")
	if found || err != nil {
		t.Fatalf("missing object stat = found %v err %v", found, err)
	}
	exists, err := storage.BucketExists(context.Background(), "legacy-audio")
	if exists || err != nil {
		t.Fatalf("deleted bucket check = exists %v err %v", exists, err)
	}
}

func TestS3OpenDistinguishesMissingObjectFromStorageFailure(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			storage, err := New(Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "task-audio", AccessKey: "key", SecretKey: "secret", PathStyle: true})
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = storage.Open(context.Background(), "task-audio", "missing.wav")
			if status == http.StatusNotFound {
				if !errors.Is(err, application.ErrObjectNotFound) {
					t.Fatalf("404 Open error = %v", err)
				}
			} else if err == nil || errors.Is(err, application.ErrObjectNotFound) {
				t.Fatalf("status %d treated as missing: %v", status, err)
			}
		})
	}
}

func TestS3CanceledStatReturnsContextError(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	storage, err := New(Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "task-audio", AccessKey: "key", SecretKey: "secret", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = storage.Stat(ctx, "task-audio", "missing.wav")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "context canceled") {
		t.Fatalf("canceled stat error = %v", err)
	}
}

func TestS3EnsureBucketCreatesMissingBucketWhenEnabled(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()
	storage, err := New(Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "task-audio", AccessKey: "key", SecretKey: "secret", PathStyle: true, CreateBucket: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.EnsureBucket(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(requests, ",") != "HEAD /task-audio,PUT /task-audio" {
		t.Fatalf("bucket requests = %v", requests)
	}
}
