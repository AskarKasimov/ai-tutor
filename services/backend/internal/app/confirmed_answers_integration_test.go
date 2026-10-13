package app

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	learninghttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/learning/transport/http"
)

func TestConfirmedAnswersRejectForeignAndStalePreviews(t *testing.T) {
	t.Parallel()
	f, access, _, owner, session := trainingFixture(t, 0)
	planResponse := f.request("POST", "/diagnostic-sessions/"+session.ID+"/learning-plan", `{"hours":2}`, access)
	if planResponse.Code != 200 {
		t.Fatal(planResponse.Body.String())
	}
	plan := decodePlan(t, planResponse.Body.Bytes())
	path := "/learning-plans/" + plan.ID
	if opened := f.request("POST", path+"/open", "", access); opened.Code != 200 {
		t.Fatal(opened.Body.String())
	}
	var chat learninghttp.MessagesResponse
	json.Unmarshal(f.request("GET", fmt.Sprintf("%s/items/%d/messages", path, *plan.CurrentIndex), "", access).Body.Bytes(), &chat)
	question := chat.Items[len(chat.Items)-1].Text
	foreignAccess, _, _ := f.register(t, "foreign-preview@example.edu")
	previewID := func(cookieOwner bool) string {
		cookie := access
		if !cookieOwner {
			cookie = foreignAccess
		}
		response := upload(f, "/voice/transcriptions", "audio", "answer.wav", "audio/wav", wavBytes(), cookie)
		var value struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil || response.Code != 200 || value.ID == "" {
			t.Fatal(response.Body.String())
		}
		return value.ID
	}
	owned, foreign := previewID(true), previewID(false)
	body := func(id, q string) string {
		data, _ := json.Marshal(map[string]any{"transcription_id": id, "item_index": *plan.CurrentIndex, "attempts": 0, "question": q})
		return string(data)
	}
	requireCode(t, f.request("POST", path+"/answers/transcription", body(foreign, question), access), 404, "TRANSCRIPTION_NOT_FOUND")
	requireCode(t, f.request("POST", path+"/answers/transcription", body(owned, "previous question"), access), 409, "LEARNING_QUESTION_CHANGED")
	unchanged := decodePlan(t, f.request("GET", path, "", access).Body.Bytes())
	if unchanged.Items[*plan.CurrentIndex].Attempts != 0 || unchanged.PendingAnswer != nil {
		t.Fatal("failed preview changed the learning attempt")
	}
	confirmed := f.request("POST", path+"/answers/transcription", body(owned, question), access)
	if confirmed.Code != 200 {
		t.Fatal(confirmed.Body.String())
	}

	// A separate active diagnostic verifies the same owner boundary in its reader.
	session.ID = "confirmed-diagnostic"
	session.Status = diagnostic.StatusActive
	session.CurrentCompetency, session.CurrentTask = 0, 0
	session.Answers, session.SkippedBasics, session.InFlight = nil, nil, nil
	session.AcceptedRequests = map[string]diagnostic.AcceptedRequest{}
	session.StartRequestDigest = "confirmed-diagnostic"
	if _, _, err := f.app.diagnosticStore.Create(t.Context(), owner, "confirmed-diagnostic", session.StartRequestDigest, session); err != nil {
		t.Fatal(err)
	}
	task := session.Current()
	diagnosticBody := func(id string) string {
		data, _ := json.Marshal(map[string]string{"transcription_id": id, "variant_task_id": task.ID})
		return string(data)
	}
	diagnosticPath := "/diagnostic-sessions/" + session.ID + "/answers/transcription"
	requireCode(t, trainingJSON(f, access, diagnosticPath, "foreign-answer", diagnosticBody(foreign)), 404, "TRANSCRIPTION_NOT_FOUND")
	current, err := f.app.diagnosticStore.Get(t.Context(), owner, session.ID)
	if err != nil || len(current.Answers) != 0 {
		t.Fatal("foreign preview advanced the diagnostic")
	}
	response := trainingJSON(f, access, diagnosticPath, "owned-answer", diagnosticBody(owned))
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	repeated := trainingJSON(f, access, diagnosticPath, "owned-answer", diagnosticBody(owned))
	if repeated.Code != 200 || repeated.Body.String() != response.Body.String() {
		t.Fatal("confirmed answer retry changed its result")
	}
}
