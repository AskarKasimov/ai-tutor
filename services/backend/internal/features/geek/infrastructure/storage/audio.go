// Package storage caches synthesized Geek turns in S3.
package storage

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/s3"
)

type Audio struct {
	client *s3.Client
	bucket string
}

func NewAudio(client *s3.Client, bucket string) *Audio { return &Audio{client: client, bucket: bucket} }

func (a *Audio) Get(ctx context.Context, key string) ([]byte, bool, error) {
	data, err := a.client.GetObject(ctx, a.bucket, key)
	if errors.Is(err, s3.ErrNotFound) {
		httpx.LoggerFromContext(ctx).Info("geek audio cache miss", zap.String("key", key))
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	httpx.LoggerFromContext(ctx).Info("geek audio cache hit", zap.String("key", key), zap.Int("bytes", len(data)))
	return data, true, nil
}

func (a *Audio) Put(ctx context.Context, key string, data []byte) error {
	if err := a.client.PutObject(ctx, a.bucket, key, "audio/wav", data); err != nil {
		return err
	}
	httpx.LoggerFromContext(ctx).Info("geek audio cached", zap.String("key", key), zap.Int("bytes", len(data)))
	return nil
}
