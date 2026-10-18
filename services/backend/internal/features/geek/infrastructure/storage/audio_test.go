package storage

import (
	"context"
	"fmt"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/s3"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAudioStoreRejectsCorruptAndOversizeObjects(t *testing.T) {
	for _, large := range []bool{false, true} {
		t.Run(fmt.Sprint(large), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if large {
					w.Header().Set("Content-Length", fmt.Sprint(audio.MaxStreamBytes+45))
					w.WriteHeader(200)
					return
				}
				w.Write([]byte("broken WAV"))
			}))
			defer server.Close()
			client, err := s3.New(s3.Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "audio", AccessKey: "key", SecretKey: "secret", PathStyle: true})
			if err != nil {
				t.Fatal(err)
			}
			store := NewAudio(client, "audio")
			if _, ok, err := store.Get(context.Background(), "key"); err != nil || ok {
				t.Fatalf("invalid object accepted %v", err)
			}
			if err := store.Put(context.Background(), "key", []byte("partial")); err == nil {
				t.Fatal("invalid WAV published")
			}
		})
	}
}
