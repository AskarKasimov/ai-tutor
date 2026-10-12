package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	learninghttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/learning/transport/http"
)

func decodePlan(t *testing.T, body []byte) learninghttp.PlanResponse {
	t.Helper()
	var plan learninghttp.PlanResponse
	if err := json.Unmarshal(body, &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

// A diagnostic with a mistake builds a plan from every outcome of that
// competency; each outcome is taught with theory and practice in the chat.
func TestLearningPlanTeachesTheCompetencyWithAMistake(t *testing.T) {
	t.Parallel()
	f, access, _, _, d := trainingFixture(t, 0)
	planPath := "/diagnostic-sessions/" + d.ID + "/learning-plan"
	requireCode(t, f.request("GET", planPath, "", access), 404, "LEARNING_PLAN_NOT_FOUND")
	requireCode(t, f.request("POST", planPath, `{"hours":0}`, access), 422, "VALIDATION_ERROR")
	created := f.request("POST", planPath, `{"hours":2}`, access)
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	plan := decodePlan(t, created.Body.Bytes())
	// The map holds three outcomes of the failed competency.
	if len(plan.Items) != 3 || plan.Status != "active" || plan.CurrentIndex == nil || plan.Hours != 2 || plan.ProgressPercent != 0 {
		t.Fatalf("plan: %+v", plan)
	}
	for _, item := range plan.Items {
		if item.CompetencyID != d.Variant.Competencies[0].ID || !item.InPlan || item.TheoryMinutes != 15 || item.PracticeMinutes != 15 {
			t.Fatalf("item: %+v", item)
		}
	}
	current := *plan.CurrentIndex
	if again := decodePlan(t, f.request("POST", planPath, `{"hours":9}`, access).Body.Bytes()); again.ID != plan.ID || again.Hours != 2 {
		t.Fatal("the plan is built once per diagnostic")
	}
	if got := decodePlan(t, f.request("GET", planPath, "", access).Body.Bytes()); got.ID != plan.ID {
		t.Fatal("the plan is found by its diagnostic")
	}

	messagesPath := fmt.Sprintf("/learning-plans/%s/items/%d/messages", plan.ID, current)
	opened := f.request("POST", "/learning-plans/"+plan.ID+"/open", "", access)
	if opened.Code != 200 {
		t.Fatal(opened.Body.String())
	}
	f.request("POST", "/learning-plans/"+plan.ID+"/open", "", access)
	var chat learninghttp.MessagesResponse
	json.Unmarshal(f.request("GET", messagesPath, "", access).Body.Bytes(), &chat)
	if len(chat.Items) != 2 || chat.Items[0].Kind != "theory" || chat.Items[1].Kind != "practice" {
		t.Fatalf("the lesson is written once: %+v", chat)
	}

	answered := upload(f, "/learning-plans/"+plan.ID+"/answers", "audio", "voice.wav", "audio/wav", wavBytes(), access)
	if answered.Code != 200 {
		t.Fatal(answered.Body.String())
	}
	plan = decodePlan(t, answered.Body.Bytes())
	if plan.Items[current].Status != "passed" || plan.CurrentIndex == nil || *plan.CurrentIndex == current || plan.ProgressPercent != 33 || plan.PendingAnswer != nil {
		t.Fatalf("after a correct answer: %+v", plan)
	}
	json.Unmarshal(f.request("GET", messagesPath, "", access).Body.Bytes(), &chat)
	if len(chat.Items) != 4 || chat.Items[2].Role != "user" || chat.Items[2].Kind != "answer" || chat.Items[3].Kind != "feedback" {
		t.Fatalf("answer and feedback join the chat: %+v", chat)
	}
	requireCode(t, f.request("POST", "/learning-plans/"+plan.ID+"/answers/retry", "", access), 409, "LEARNING_NO_PENDING_ANSWER")
	requireCode(t, f.request("GET", "/learning-plans/missing", "", access), 404, "LEARNING_PLAN_NOT_FOUND")
}

func TestLearningPlanWithoutMistakesIsCompleted(t *testing.T) {
	t.Parallel()
	f, access, _, _, d := trainingFixture(t, 2)
	created := f.request("POST", "/diagnostic-sessions/"+d.ID+"/learning-plan", `{"hours":1}`, access)
	plan := decodePlan(t, created.Body.Bytes())
	if created.Code != 200 || len(plan.Items) != 0 || plan.Status != "completed" || plan.CurrentIndex != nil || plan.ProgressPercent != 100 {
		t.Fatalf("%d %+v", created.Code, plan)
	}
}

func TestLearningTheoryIsReadableWhilePracticeIsGenerated(t *testing.T) {
	t.Parallel()
	f, access, _, _, d := trainingFixture(t, 0)
	created := f.request("POST", "/diagnostic-sessions/"+d.ID+"/learning-plan", `{"hours":2}`, access)
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	plan := decodePlan(t, created.Body.Bytes())
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	var requests atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := requests.Add(1)
		content := `{"theory":"Теория доступна сразу"}`
		if requestNumber > 1 {
			var body struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 2 {
				t.Errorf("bad model request: %v", err)
			}
			if len(body.Messages) == 2 {
				var input struct {
					Theory string `json:"theory"`
				}
				if err := json.Unmarshal([]byte(body.Messages[1].Content), &input); err != nil || input.Theory != "Теория доступна сразу" {
					t.Errorf("practice theory: %+v, %v", input, err)
				}
			}
			content = `{"question":"Оцените предложение коллеги и обоснуйте ответ."}`
		}
		if requestNumber == 2 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	defer model.Close()
	defer releaseOnce.Do(func() { close(release) })
	f.app.cfg.APIMode = "real"
	f.app.cfg.TutorBaseURL, f.app.cfg.TutorModel = model.URL, "test-model"
	f.handler = f.app.Handler()
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- f.request("POST", "/learning-plans/"+plan.ID+"/open", "", access) }()
	select {
	case <-started:
	case response := <-finished:
		t.Fatalf("open finished before practice: %d %s", response.Code, response.Body.String())
	case <-time.After(5 * time.Second):
		t.Fatal("practice generation did not start")
	}
	path := fmt.Sprintf("/learning-plans/%s/items/%d/messages", plan.ID, *plan.CurrentIndex)
	response := f.request("GET", path, "", access)
	var chat learninghttp.MessagesResponse
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &chat) != nil || len(chat.Items) != 1 || chat.Items[0].Text != "Теория доступна сразу" || chat.Items[0].Kind != "theory" {
		t.Fatalf("theory not published while open waits: %d %s", response.Code, response.Body.String())
	}
	releaseOnce.Do(func() { close(release) })
	if opened := <-finished; opened.Code != 200 {
		t.Fatal(opened.Body.String())
	}
	json.Unmarshal(f.request("GET", path, "", access).Body.Bytes(), &chat)
	if len(chat.Items) != 2 || chat.Items[1].Kind != "practice" || requests.Load() != 3 {
		t.Fatalf("completed lesson: %+v, requests=%d", chat, requests.Load())
	}
	if opened := f.request("POST", "/learning-plans/"+plan.ID+"/open", "", access); opened.Code != 200 || requests.Load() != 3 {
		t.Fatal("repeated open regenerated the lesson")
	}
}
