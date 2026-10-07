package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

func TestStoreHandlesTwoThousandConcurrentUsers(t *testing.T) {
	const users = 2000
	store := New()
	var wait sync.WaitGroup
	errs := make(chan error, users)
	for i := 0; i < users; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			ownerID := fmt.Sprintf("user-%d", index)
			sessionID := fmt.Sprintf("session-%d", index)
			session := diagnostic.Session{
				ID: sessionID, OwnerID: ownerID, Status: diagnostic.StatusActive,
				Variant: diagnostic.VariantSnapshot{
					IncludedCompetencyCount: 1,
					Competencies: []diagnostic.Competency{{Tasks: []diagnostic.TaskSnapshot{
						{ID: "task-main", Role: "main"}, {ID: "task-basic-1", Role: "basic"}, {ID: "task-basic-2", Role: "basic"},
					}}},
				},
				AcceptedRequests: make(map[string]diagnostic.AcceptedRequest),
			}
			if _, reused, err := store.Create(context.Background(), ownerID, "start", "digest", session); err != nil || reused {
				errs <- fmt.Errorf("create for %s: reused=%v, err=%w", ownerID, reused, err)
				return
			}
			if _, err := store.Get(context.Background(), ownerID, sessionID); err != nil {
				errs <- fmt.Errorf("read for %s: %w", ownerID, err)
				return
			}
			accepted, reservation, err := store.Reserve(context.Background(), ownerID, sessionID, "answer", "fingerprint", "task-main", "token")
			if err != nil || accepted != nil || reservation == nil || reservation.Token != "token" {
				errs <- fmt.Errorf("reserve for %s: accepted=%v reservation=%v err=%w", ownerID, accepted != nil, reservation, err)
			}
		}(i)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestStoreOwnsAcceptedAnswersAndReplayResponses(t *testing.T) {
	ctx := context.Background()
	store := New()
	session := diagnostic.Session{
		ID: "session", OwnerID: "owner", Status: diagnostic.StatusActive,
		Variant: diagnostic.VariantSnapshot{
			IncludedCompetencyCount: 1,
			Competencies: []diagnostic.Competency{{Tasks: []diagnostic.TaskSnapshot{
				{ID: "main", Role: "main", Options: []string{"main option"}},
				{ID: "basic-1", Role: "basic", Options: []string{"basic option"}},
				{ID: "basic-2", Role: "basic"},
			}}},
		},
		AcceptedRequests: make(map[string]diagnostic.AcceptedRequest),
	}
	if _, _, err := store.Create(ctx, "owner", "start", "start-digest", session); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Reserve(ctx, "owner", "session", "answer", "fingerprint", "main", "token"); err != nil {
		t.Fatal(err)
	}
	answer := diagnostic.Answer{
		VariantTaskID: "main", Task: session.Variant.Competencies[0].Tasks[0], Score: 1,
		GraderScore: 1, GraderMaxScore: 2, Verdict: "partial",
		CriterionResults: []diagnostic.CriterionResult{{Key: "accuracy", Explanation: "Explanation"}},
		Feedback:         []string{"Feedback"},
	}
	response, err := store.Accept(ctx, "owner", "session", "token", "answer", "fingerprint", answer,
		diagnostic.Transition{CurrentTask: 1, Status: diagnostic.StatusActive})
	if err != nil {
		t.Fatal(err)
	}
	answer.Task.Options[0], answer.CriterionResults[0].Key, answer.Feedback[0] = "changed", "changed", "changed"
	mutate := func(value diagnostic.Progress) {
		value.Current.Options[0] = "changed"
		value.CriterionResults[0].Key = "changed"
		value.Feedback[0] = "changed"
		*value.Score, *value.GraderScore, *value.GraderMaxScore = 0, 0, 0
	}
	mutate(response)
	for range 2 {
		replay, _, err := store.Reserve(ctx, "owner", "session", "answer", "fingerprint", "main", "retry-token")
		if err != nil || replay == nil {
			t.Fatalf("replay: %v / %v", replay, err)
		}
		value := replay.Response
		if value.Current.Options[0] != "basic option" || value.CriterionResults[0].Key != "accuracy" || value.Feedback[0] != "Feedback" || *value.Score != 1 || *value.GraderScore != 1 || *value.GraderMaxScore != 2 {
			t.Fatalf("caller mutation changed an idempotent response: %+v", value)
		}
		mutate(value)
	}
	stored, err := store.Get(ctx, "owner", "session")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Current().Options[0] != "basic option" || stored.Answers[0].Task.Options[0] != "main option" || stored.Answers[0].CriterionResults[0].Key != "accuracy" || stored.Answers[0].Feedback[0] != "Feedback" {
		t.Fatalf("caller mutation changed stored session data: %+v", stored)
	}
}
