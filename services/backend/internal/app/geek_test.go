package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

// geekFlow runs a whole diagnostic over HTTP on the graph fixture with the mock
// examiner and checks every response against the contract.
func geekFlow(t *testing.T, f *fixture, access *http.Cookie, check func(method, path string, w *httptest.ResponseRecorder)) {
	t.Helper()
	f.app.cfg.GeekGraphFixture = geekFixture
	start := `{"subject_id":"subject:test"}`
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
	check("GET", "/diagnostic-sessions/{id}", f.request("GET", path, "", access))
	check("GET", "/diagnostic-sessions/{id}", f.request("GET", "/diagnostic-sessions/unknown", "", access))
	check("GET", "/diagnostic-sessions/{id}/result", f.request("GET", path+"/result", "", access))
	audio := f.request("GET", path+"/turns/"+*session.CurrentTurnID+"/audio", "", access)
	if audio.Code != http.StatusOK {
		t.Fatalf("turn audio: %d %s", audio.Code, audio.Body.String())
	}
	check("GET", "/diagnostic-sessions/{id}/turns/{turn_id}/audio", audio)
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
	}
	if err := json.Unmarshal(result.Body.Bytes(), &r); err != nil || r.Grade == nil || *r.Grade != "D" || len(r.Gaps) != 3 {
		t.Fatalf("result: %s", result.Body.String())
	}
	studentTurn := ""
	for _, turn := range session.Turns {
		if turn.Speaker == "student" {
			studentTurn = turn.ID
		}
	}
	check("GET", "/diagnostic-sessions/{id}/turns/{turn_id}/audio", f.request("GET", path+"/turns/"+studentTurn+"/audio", "", access))
}
