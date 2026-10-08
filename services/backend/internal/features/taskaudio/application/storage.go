package application

import (
	"context"
	"errors"
	"io"
)

var ErrObjectNotFound = errors.New("stored audio object not found")
var ErrBucketNotFound = errors.New("stored audio bucket not found")

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

type BucketChecker interface {
	BucketExists(ctx context.Context, bucket string) (bool, error)
}

type Synthesizer interface {
	Synthesize(context.Context, string) ([]byte, error)
}
