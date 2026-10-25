package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

// Store keeps the overall feedback of a diagnostic in PostgreSQL. The first
// saved feedback of a session wins within an analysis version. A newer version
// may replace obsolete conclusions; repeated reads then retain its first text.
type Store struct {
	q *db.Queries
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{q: db.New(pool)} }

func (s *Store) Get(ctx context.Context, ownerID, sessionID string) (diagnostic.OverallFeedback, bool, error) {
	raw, err := s.q.LearningReadDiagnosticFeedback(ctx, db.LearningReadDiagnosticFeedbackParams{OwnerID: ownerID, SessionID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return diagnostic.OverallFeedback{}, false, nil
	}
	if err != nil {
		return diagnostic.OverallFeedback{}, false, err
	}
	var feedback diagnostic.OverallFeedback
	if err := json.Unmarshal(raw, &feedback); err != nil {
		return diagnostic.OverallFeedback{}, false, err
	}
	return feedback, true, nil
}

func (s *Store) Save(ctx context.Context, ownerID string, feedback diagnostic.OverallFeedback) error {
	data, err := json.Marshal(feedback)
	if err != nil {
		return err
	}
	return s.q.LearningSaveDiagnosticFeedback(ctx, db.LearningSaveDiagnosticFeedbackParams{SessionID: feedback.SessionID, OwnerID: ownerID, FeedbackData: data})
}
