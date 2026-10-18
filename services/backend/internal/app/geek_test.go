package app

import (
	"context"
	"encoding/json"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	geekapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	geekgraph "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/infrastructure/graph"
)

const geekFixture = "../../../../api/fixtures/course-graph.json"

func (f *fixture) keyed(method, path, key, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://api.example"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, req)
	return w
}

type geekSession struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	CurrentTurnID *string `json:"current_turn_id"`
	Progress      int     `json:"progress_percent"`
	Turns         []struct {
		ID      string `json:"id"`
		Speaker string `json:"speaker"`
		Kind    string `json:"kind"`
	} `json:"turns"`
}

func decodeSession(t *testing.T, w *httptest.ResponseRecorder) geekSession {
	t.Helper()
	var s geekSession
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	return s
}

// geekFlow runs a whole diagnostic over HTTP on the configured graph fixture
// with the mock examiner and checks every response against the contract.
func geekFlow(t *testing.T, f *fixture, access *http.Cookie, check func(method, path string, w *httptest.ResponseRecorder)) string {
	t.Helper()
	start := `{"subject_id":"subject:test"}`
	// The fixture is served once its rephrases are prepared (decision of
	// 18.10.2026); until then a diagnostic does not start.
	f.app.fixture = geekapp.NewPreparedGraph(geekgraph.Fixture{Path: f.app.cfg.GeekGraphFixture}, f.app.examiner, nil, f.app.warmer, nil)
	notReady := f.keyed("POST", "/diagnostic-sessions", "geek-not-ready", start, access)
	if notReady.Code != http.StatusConflict || !strings.Contains(notReady.Body.String(), "GRAPH_NOT_READY") {
		t.Fatalf("start before the fixture is prepared: %d %s", notReady.Code, notReady.Body.String())
	}
	check("POST", "/diagnostic-sessions", notReady)
	prepareFixture(t, f)
	created := f.keyed("POST", "/diagnostic-sessions", "geek-start", start, access)
	if created.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", created.Code, created.Body.String())
	}
	check("POST", "/diagnostic-sessions", created)
	check("POST", "/diagnostic-sessions", f.keyed("POST", "/diagnostic-sessions", "geek-again", start, access))
	check("POST", "/diagnostic-sessions", f.keyed("POST", "/diagnostic-sessions", "geek-missing", `{"subject_id":"missing"}`, access))
	check("POST", "/diagnostic-sessions", f.keyed("POST", "/diagnostic-sessions", "geek-bad", `{"subject":"x"}`, access))
	check("POST", "/diagnostic-sessions", f.request("POST", "/diagnostic-sessions", start))
	session := decodeSession(t, created)
	path := "/diagnostic-sessions/" + session.ID
	check("GET", "/diagnostic-sessions", f.request("GET", "/diagnostic-sessions?subject_id=subject%3Atest", "", access))
	check("GET", "/diagnostic-sessions", f.request("GET", "/diagnostic-sessions?subject=x", "", access))
	check("GET", "/diagnostic-sessions/{id}", f.request("GET", path, "", access))
	check("GET", "/diagnostic-sessions/{id}", f.request("GET", "/diagnostic-sessions/unknown", "", access))
	check("GET", "/diagnostic-sessions/{id}/result", f.request("GET", path+"/result", "", access))
	audio := f.request("GET", path+"/turns/"+*session.CurrentTurnID+"/audio", "", access)
	if audio.Code != http.StatusOK {
		t.Fatalf("turn audio: %d %s", audio.Code, audio.Body.String())
	}
	check("GET", "/diagnostic-sessions/{id}/turns/{turn_id}/audio", audio)
	streamPath := path + "/turns/" + *session.CurrentTurnID + "/audio/stream"
	streamContract := "/diagnostic-sessions/{id}/turns/{turn_id}/audio/stream"
	disabled := f.request("GET", streamPath, "", access)
	if disabled.Code != 406 {
		t.Fatalf("stream gate %d", disabled.Code)
	}
	check("GET", streamContract, disabled)
	check("GET", streamContract, f.request("GET", streamPath, ""))
	check("GET", streamContract, f.request("GET", path+"/turns/missing/audio/stream", "", access))
	f.app.cfg.TTSStreamingEnabled = true
	check("GET", streamContract, f.request("GET", streamPath, "", access))
	f.app.cfg.TTSStreamingEnabled = false

	check("GET", "/diagnostic-sessions/{id}/turns/{turn_id}/audio", f.request("GET", path+"/turns/unknown/audio", "", access))
	check("POST", "/diagnostic-sessions/{id}/answers", f.keyed("POST", path+"/answers", "geek-stale", `{"turn_id":"unknown","text":"ответ"}`, access))
	check("POST", "/diagnostic-sessions/{id}/answers", f.keyed("POST", path+"/answers", "geek-two", `{"turn_id":"x","text":"ответ","dont_know":true}`, access))
	// Weak answers on everything: every topic becomes a gap, the exam descends
	// through the whole graph and the gaps are the three topics without
	// prerequisites.
	for i := 0; session.Status == "active"; i++ {
		if i > 40 {
			t.Fatal("diagnostic does not end")
		}
		body := `{"turn_id":"` + *session.CurrentTurnID + `","text":"ну это поиск"}`
		if i%2 == 1 {
			body = `{"turn_id":"` + *session.CurrentTurnID + `","dont_know":true}`
		}
		w := f.keyed("POST", path+"/answers", "geek-answer-"+session.Turns[len(session.Turns)-1].ID, body, access)
		check("POST", "/diagnostic-sessions/{id}/answers", w)
		session = decodeSession(t, w)
	}
	if session.Status != "completed" || session.Progress != 100 || session.CurrentTurnID != nil {
		t.Fatalf("completed session = %+v", session)
	}
	result := f.request("GET", path+"/result", "", access)
	check("GET", "/diagnostic-sessions/{id}/result", result)
	var r struct {
		Grade *string `json:"grade"`
		Gaps  []struct {
			NodeID string `json:"node_id"`
		} `json:"gaps"`
		Nodes []struct {
			NodeID   string   `json:"node_id"`
			Requires []string `json:"requires"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &r); err != nil || r.Grade == nil || *r.Grade != "D" || len(r.Gaps) != 3 {
		t.Fatalf("result: %s", result.Body.String())
	}
	// Every node carries its prerequisites, and they reference result nodes.
	ids := map[string]bool{}
	edges := 0
	for _, n := range r.Nodes {
		ids[n.NodeID] = true
	}
	for _, n := range r.Nodes {
		if n.Requires == nil {
			t.Fatalf("node %s has no requires array: %s", n.NodeID, result.Body.String())
		}
		for _, p := range n.Requires {
			if !ids[p] {
				t.Fatalf("node %s requires unknown %s", n.NodeID, p)
			}
			edges++
		}
	}
	if edges == 0 {
		t.Fatalf("result has no prerequisite edges: %s", result.Body.String())
	}
	studentTurn := ""
	for _, turn := range session.Turns {
		if turn.Speaker == "student" {
			studentTurn = turn.ID
		}
	}
	check("GET", "/diagnostic-sessions/{id}/turns/{turn_id}/audio", f.request("GET", path+"/turns/"+studentTurn+"/audio", "", access))
	return session.ID
}

// prepareFixture runs the preparation of the fixture until it is served.
func prepareFixture(t *testing.T, f *fixture) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = f.app.fixture.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		g, ok, err := f.app.fixture.Current(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if left := geekapp.Unprepared(g); len(left) != 0 {
				t.Fatalf("served with unprepared nodes %v", left)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the fixture was not prepared")
		}
	}
}

// Real network hop plus the app's authentication/logging middleware must flush
// PCM while the fake provider is still generating, rather than at handler return.
func TestGeekStreamFlushesThroughAppMiddleware(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/synthesize/stream" {
			t.Errorf("unexpected provider %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		_ = enc.Encode(audio.StartEvent(24000))
		_ = enc.Encode(audio.StreamEvent{Type: "audio", Seq: 0, PCM: []byte{1, 0, 2, 0}})
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			return
		case <-release:
		}
		_ = enc.Encode(audio.StreamEvent{Type: "end", Chunks: 1, Samples: 2})
	}))
	defer provider.Close()
	cfg := testConfig()
	cfg.TTSURL = provider.URL + "/synthesize"
	cfg.GeekGraphFixture = geekFixture
	cfg.TTSStreamingEnabled = true
	cfg.TTSModelRevision = "fake@revision"
	cfg.TTSReferenceSHA256 = strings.Repeat("a", 64)
	f := newFixtureWithConfig(t, cfg)
	f.app.speech = geekapp.NewSpeechCache(f.app.models(), &streamTestAudio{}, f.app.voiceName(), time.Second*5)
	prepareFixture(t, f)
	register := f.request("POST", "/auth/register", `{"email":"stream@example.edu","password":"`+password+`","display_name":"Поток"}`)
	if register.Code != 201 {
		t.Fatal(register.Body.String())
	}
	access := register.Result().Cookies()[0]
	s := decodeSession(t, f.keyed("POST", "/diagnostic-sessions", "stream-start", `{"subject_id":"subject:test"}`, access))
	server := httptest.NewServer(f.app.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/diagnostic-sessions/"+s.ID+"/turns/"+*s.CurrentTurnID+"/audio/stream", nil)
	req.AddCookie(access)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.ContentLength != -1 || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("stream headers %d %v", resp.StatusCode, resp.Header)
	}
	decoder := json.NewDecoder(resp.Body)
	var event audio.StreamEvent
	if err = decoder.Decode(&event); err != nil || event.Type != "start" {
		t.Fatalf("start %v %v", event, err)
	}
	if err = decoder.Decode(&event); err != nil || event.Type != "audio" {
		t.Fatalf("early PCM %v %v", event, err)
	}
	releaseOnce.Do(func() { close(release) })
	if err = decoder.Decode(&event); err != nil || event.Type != "end" {
		t.Fatalf("end %v %v", event, err)
	}
}

type streamTestAudio struct {
	mu   sync.Mutex
	data []byte
}

func (s *streamTestAudio) Get(context.Context, string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data, s.data != nil, nil
}
func (s *streamTestAudio) Put(_ context.Context, _ string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data
	return nil
}
