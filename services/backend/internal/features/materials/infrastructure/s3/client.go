package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	client *s3.Client
	bucket string
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
		if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
			return nil, errors.New("S3 endpoint must be an absolute HTTP(S) URL")
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

	c := &Client{client: client, bucket: cfg.Bucket}
	if cfg.CreateBucket {
		_ = c.EnsureBucket(context.Background())
	}
	return c, nil
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(c.bucket)})
	if err == nil {
		return nil
	}
	_, err = c.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(c.bucket)})
	return err
}

func (c *Client) Put(ctx context.Context, key, contentType string, data []byte) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(len(data))),
	})
	if err != nil {
		return fmt.Errorf("put S3 object %s: %w", key, err)
	}
	return nil
}

func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	rc, _, _, err := c.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (c *Client) Open(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	output, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", 0, fmt.Errorf("get S3 object %s: %w", key, err)
	}
	contentType := "application/octet-stream"
	if output.ContentType != nil {
		contentType = *output.ContentType
	}
	var size int64
	if output.ContentLength != nil {
		size = *output.ContentLength
	}
	return output.Body, contentType, size, nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiError smithy.APIError
		if errors.As(err, &apiError) && (apiError.ErrorCode() == "NoSuchKey" || apiError.ErrorCode() == "NotFound") {
			return nil
		}
		return fmt.Errorf("delete S3 object %s: %w", key, err)
	}
	return nil
}
