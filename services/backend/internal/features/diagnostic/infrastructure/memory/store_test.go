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
