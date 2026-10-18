package app

import (
	"context"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type countedAudio struct {
	reads, writes int
	data          []byte
}

func (a *countedAudio) Get(context.Context, string) ([]byte, bool, error) {
	a.reads++
	return a.data, a.data != nil, nil
}
func (a *countedAudio) Put(_ context.Context, _ string, data []byte) error {
	a.writes++
	a.data = data
	return nil
}
func TestUnpinnedSpeechBypassesPersistentCache(t *testing.T) {
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wavBytes())
	}))
	defer provider.Close()
	a := &App{cfg: testConfig(), client: provider.Client(), logger: zap.NewNop()}
	a.cfg.TTSURL = provider.URL + "/synthesize"
	store := &countedAudio{data: wavBytes()}
	speech := a.newSpeechCache(store)
	for range 2 {
		if _, err := speech.Part(context.Background(), "Вопрос?"); err != nil {
			t.Fatal(err)
		}
	}
	if store.reads != 0 || store.writes != 0 || calls != 2 {
		t.Fatalf("unpinned persistent cache reads=%d writes=%d synthesis=%d", store.reads, store.writes, calls)
	}
	a.cfg.TTSModelRevision = "fake@revision"
	a.cfg.TTSReferenceSHA256 = strings.Repeat("a", 64)
	store.data = nil
	speech = a.newSpeechCache(store)
	for range 2 {
		if _, err := speech.Part(context.Background(), "Вопрос?"); err != nil {
			t.Fatal(err)
		}
	}
	if store.reads != 2 || store.writes != 1 || calls != 3 {
		t.Fatalf("pinned cache reads=%d writes=%d synthesis=%d", store.reads, store.writes, calls)
	}
}
