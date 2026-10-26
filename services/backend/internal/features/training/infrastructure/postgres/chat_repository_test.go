package postgres

import (
	"strconv"
	"testing"
	"time"

	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

func TestChatPagesUseLastReturnedSequenceAsExclusiveCursor(t *testing.T) {
	sequences := []int64{5, 4, 3, 2, 1}
	var got []int64
	var before int64
	for {
		rows := make([]db.LearningReadTrainingChatPageRow, 0, len(sequences))
		for _, sequence := range sequences {
			if before == 0 || sequence < before {
				rows = append(rows, db.LearningReadTrainingChatPageRow{Sequence: sequence})
			}
		}
		limit := 2
		cursor := chatPageCursor(rows, limit)
		page := rows
		if len(page) > limit {
			page = page[:limit]
		}
		for _, row := range page {
			got = append(got, row.Sequence)
		}
		if cursor == nil {
			break
		}
		before, _ = strconv.ParseInt(*cursor, 10, 64)
	}

	want := []int64{5, 4, 3, 2, 1}
	if len(got) != len(want) {
		t.Fatalf("read sequences %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("read sequences %v, want %v", got, want)
		}
	}
}

func TestChatRepositoryWithLeaseRoundsUpToWholeSeconds(t *testing.T) {
	repository := NewChatRepository(nil).WithLease(5*time.Second + time.Nanosecond)
	if repository.leaseSeconds != 6 {
		t.Fatalf("lease seconds = %d, want 6", repository.leaseSeconds)
	}
}
