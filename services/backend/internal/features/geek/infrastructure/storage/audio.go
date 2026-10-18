// Package storage keeps synthesized Geek turns and prepared graph texts in S3.
package storage

import (
	"context"
	"errors"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"io"

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
	rc, _, size, err := a.client.OpenObject(ctx, a.bucket, key)
	if errors.Is(err, s3.ErrNotFound) {
		httpx.LoggerFromContext(ctx).Info("geek audio cache miss")
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer rc.Close()
	if size > audio.MaxStreamBytes+44 {
		httpx.LoggerFromContext(ctx).Warn("oversize geek audio cache object")
		return nil, false, nil
	}
	data, err := io.ReadAll(io.LimitReader(rc, audio.MaxStreamBytes+45))
	if err != nil {
		return nil, false, err
	}
	if len(data) > audio.MaxStreamBytes+44 {
		return nil, false, nil
	}
	if _, _, err = audio.PCMFromWAV(data); err != nil {
		httpx.LoggerFromContext(ctx).Warn("invalid geek audio cache object")
		return nil, false, nil
	}

	httpx.LoggerFromContext(ctx).Info("geek audio cache hit", zap.String("key", key), zap.Int("bytes", len(data)))
	return data, true, nil
}

func (a *Audio) Put(ctx context.Context, key string, data []byte) error {
	if _, _, err := audio.PCMFromWAV(data); err != nil {
		return err
	}
	if err := a.client.PutObject(ctx, a.bucket, key, "audio/wav", data); err != nil {
		return err
	}
	httpx.LoggerFromContext(ctx).Info("geek audio cached", zap.String("key", key), zap.Int("bytes", len(data)))
	return nil
}

// Prepared keeps the texts prepared for a graph version in S3.
type Prepared struct {
	client *s3.Client
	bucket string
}

func NewPrepared(client *s3.Client, bucket string) *Prepared {
	return &Prepared{client: client, bucket: bucket}
}

func (p *Prepared) Get(ctx context.Context, key string) ([]byte, bool, error) {
	data, err := p.client.GetObject(ctx, p.bucket, key)
	if errors.Is(err, s3.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (p *Prepared) Put(ctx context.Context, key string, data []byte) error {
	return p.client.PutObject(ctx, p.bucket, key, "application/json", data)
}
