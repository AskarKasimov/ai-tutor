package app

import (
	"encoding/json"
	"fmt"
	"testing"

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
	f, access, _, _, d := trainingFixture(t, 2)
	created := f.request("POST", "/diagnostic-sessions/"+d.ID+"/learning-plan", `{"hours":1}`, access)
	plan := decodePlan(t, created.Body.Bytes())
	if created.Code != 200 || len(plan.Items) != 0 || plan.Status != "completed" || plan.CurrentIndex != nil || plan.ProgressPercent != 100 {
		t.Fatalf("%d %+v", created.Code, plan)
	}
}
