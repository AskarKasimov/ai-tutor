package memory

import (
	"context"
	"sync"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

// Store keeps the first saved feedback within each analysis version.
type Store struct {
	mu   sync.RWMutex
	data map[string]diagnostic.OverallFeedback
}

func New() *Store {
	return &Store{data: make(map[string]diagnostic.OverallFeedback)}
}

func key(ownerID, sessionID string) string { return ownerID + "\x00" + sessionID }

func (s *Store) Get(ctx context.Context, ownerID, sessionID string) (diagnostic.OverallFeedback, bool, error) {
	if err := ctx.Err(); err != nil {
		return diagnostic.OverallFeedback{}, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.data[key(ownerID, sessionID)]
	return value, ok, nil
}

func (s *Store) Save(ctx context.Context, ownerID string, feedback diagnostic.OverallFeedback) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.data[key(ownerID, feedback.SessionID)]; !exists || current.AnalysisVersion < feedback.AnalysisVersion {
		s.data[key(ownerID, feedback.SessionID)] = feedback
	}
	return nil
}
