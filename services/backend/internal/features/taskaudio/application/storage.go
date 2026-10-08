package application

import (
	"context"
	"io"
)

type ObjectInfo struct {
	Size        int64
	ContentType string
	AssetID     string
}

type Storage interface {
	EnsureBucket(ctx context.Context) error
	Stat(ctx context.Context, bucket, key string) (ObjectInfo, bool, error)
	Put(ctx context.Context, bucket, key, assetID string, data []byte) error
	Open(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error)
}

type Synthesizer interface {
	Synthesize(context.Context, string) ([]byte, error)
}
