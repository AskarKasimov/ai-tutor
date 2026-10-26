package s3

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/application"
	shareds3 "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/s3"
)

type Client struct{ storage *shareds3.Client }

func New(storage *shareds3.Client) *Client { return &Client{storage: storage} }

func (c *Client) EnsureBucket(ctx context.Context) error { return c.storage.EnsureBucket(ctx) }

func (c *Client) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return c.storage.BucketExists(ctx, bucket)
}

func (c *Client) Stat(ctx context.Context, bucket, key string) (application.ObjectInfo, bool, error) {
	info, found, err := c.storage.StatObject(ctx, bucket, key)
	return learningObjectInfo(info), found, mapStorageError(err)
}

func (c *Client) Put(ctx context.Context, bucket, key, assetID string, data []byte) error {
	return c.storage.PutObjectWithMetadata(ctx, bucket, key, "audio/wav", data, map[string]string{"audio-asset-id": assetID})
}

func (c *Client) Open(ctx context.Context, bucket, key string) (io.ReadCloser, application.ObjectInfo, error) {
	body, info, err := c.storage.OpenObjectInfo(ctx, bucket, key)
	return body, learningObjectInfo(info), mapStorageError(err)
}

func learningObjectInfo(info shareds3.ObjectInfo) application.ObjectInfo {
	result := application.ObjectInfo{Size: info.Size, ContentType: info.ContentType}
	for key, value := range info.Metadata {
		if strings.EqualFold(key, "audio-asset-id") {
			result.AssetID = value
			break
		}
	}
	return result
}

func mapStorageError(err error) error {
	switch {
	case errors.Is(err, shareds3.ErrNotFound):
		return errors.Join(application.ErrObjectNotFound, err)
	case errors.Is(err, shareds3.ErrBucketNotFound):
		return errors.Join(application.ErrBucketNotFound, err)
	default:
		return err
	}
}
