package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/application"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type Config struct {
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	PathStyle    bool
	CreateBucket bool
	Timeout      time.Duration
}

type Client struct {
	client       *s3.Client
	bucket       string
	createBucket bool
}

func New(cfg Config) (*Client, error) {
	if cfg.Region == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("S3 region, bucket and credentials are required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Endpoint != "" {
		endpoint, err := url.Parse(cfg.Endpoint)
		if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return nil, errors.New("S3 endpoint must be an absolute HTTP(S) URL without credentials, query or fragment")
		}
	}
	awsConfig := aws.Config{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		HTTPClient:  &http.Client{Timeout: cfg.Timeout},
		Retryer: func() aws.Retryer {
			return retry.NewStandard(func(options *retry.StandardOptions) { options.MaxAttempts = 2 })
		},
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = cfg.PathStyle
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})
	return &Client{client: client, bucket: cfg.Bucket, createBucket: cfg.CreateBucket}, nil
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(c.bucket)})
	if err == nil {
		return nil
	}
	if !isBucketMissing(err) {
		return fmt.Errorf("check S3 bucket: %w", err)
	}
	if !c.createBucket {
		return fmt.Errorf("S3 bucket %q does not exist: %w", c.bucket, err)
	}
	input := &s3.CreateBucketInput{Bucket: aws.String(c.bucket)}
	if c.client.Options().Region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(c.client.Options().Region),
		}
	}
	if _, err := c.client.CreateBucket(ctx, input); err != nil && !isBucketAlreadyOwned(err) {
		return fmt.Errorf("create S3 bucket: %w", err)
	}
	return nil
}

func (c *Client) Stat(ctx context.Context, bucket, key string) (application.ObjectInfo, bool, error) {
	output, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		if isHeadObjectBucketMissing(err) {
			return application.ObjectInfo{}, false, fmt.Errorf("head S3 object: %w", application.ErrBucketNotFound)
		}
		if isObjectMissing(err) {
			return application.ObjectInfo{}, false, nil
		}
		return application.ObjectInfo{}, false, fmt.Errorf("head S3 object: %w", err)
	}
	return objectInfo(output.ContentLength, output.ContentType, output.Metadata), true, nil
}

func (c *Client) BucketExists(ctx context.Context, bucket string) (bool, error) {
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return true, nil
	}
	if isBucketMissing(err) {
		return false, nil
	}
	return false, fmt.Errorf("check S3 bucket %q: %w", bucket, err)
}

func isHeadObjectBucketMissing(err error) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError) && apiError.ErrorCode() == "NoSuchBucket"
}

func (c *Client) Put(ctx context.Context, bucket, key, assetID string, data []byte) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(data),
		ContentType: aws.String("audio/wav"), ContentLength: aws.Int64(int64(len(data))),
		Metadata: map[string]string{"audio-asset-id": assetID},
	})
	if err != nil {
		return fmt.Errorf("put S3 object: %w", err)
	}
	return nil
}

func (c *Client) Open(ctx context.Context, bucket, key string) (io.ReadCloser, application.ObjectInfo, error) {
	output, err := c.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		if isHeadObjectBucketMissing(err) {
			return nil, application.ObjectInfo{}, fmt.Errorf("get S3 object: %w", application.ErrBucketNotFound)
		}
		if isObjectMissing(err) {
			return nil, application.ObjectInfo{}, fmt.Errorf("get S3 object: %w", application.ErrObjectNotFound)
		}
		return nil, application.ObjectInfo{}, fmt.Errorf("get S3 object: %w", err)
	}
	return output.Body, objectInfo(output.ContentLength, output.ContentType, output.Metadata), nil
}

func objectInfo(size *int64, contentType *string, metadata map[string]string) application.ObjectInfo {
	info := application.ObjectInfo{}
	if size != nil {
		info.Size = *size
	}
	if contentType != nil {
		info.ContentType = *contentType
	}
	for key, value := range metadata {
		if strings.EqualFold(key, "audio-asset-id") {
			info.AssetID = value
			break
		}
	}
	return info
}

func isObjectMissing(err error) bool {
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		return apiError.ErrorCode() == "NoSuchKey" || apiError.ErrorCode() == "NotFound"
	}
	return false
}

func isBucketMissing(err error) bool {
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		return apiError.ErrorCode() == "NoSuchBucket" || apiError.ErrorCode() == "NotFound" || apiError.ErrorCode() == "404"
	}
	return false
}

func isBucketAlreadyOwned(err error) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError) && apiError.ErrorCode() == "BucketAlreadyOwnedByYou"
}
