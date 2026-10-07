package memory

import (
	"context"
	"sync"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

type Cache struct {
	mu   sync.RWMutex
	data map[string]diagnostic.OverallFeedback
}

func New() *Cache {
	return &Cache{data: make(map[string]diagnostic.OverallFeedback)}
}

func (c *Cache) Get(ctx context.Context, sessionID string) (*diagnostic.OverallFeedback, bool) {
	if err := ctx.Err(); err != nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.data[sessionID]
	if !ok {
		return nil, false
	}
	clone := val
	return &clone, true
}

func (c *Cache) Set(ctx context.Context, sessionID string, feedback diagnostic.OverallFeedback) {
	if err := ctx.Err(); err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[sessionID] = feedback
}
