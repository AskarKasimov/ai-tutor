package s3

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestS3GenericObjectOperations(t *testing.T) {
	data := []byte("# Markdown content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("Content-Type") != "text/markdown" {
				t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/markdown")
			w.Header().Set("Content-Length", "18")
			_, _ = w.Write(data)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client, err := New(Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "test-bucket", AccessKey: "key", SecretKey: "secret", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.PutObject(ctx, "test-bucket", "notes.md", "text/markdown", data); err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}
	got, err := client.GetObject(ctx, "test-bucket", "notes.md")
	if err != nil || string(got) != string(data) {
		t.Fatalf("GetObject failed: got=%s err=%v", string(got), err)
	}
	rc, ct, size, err := client.OpenObject(ctx, "test-bucket", "notes.md")
	if err != nil || ct != "text/markdown" || size != 18 {
		t.Fatalf("OpenObject failed: ct=%s size=%d err=%v", ct, size, err)
	}
	_ = rc.Close()
	if err := client.DeleteObject(ctx, "test-bucket", "notes.md"); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}
}
