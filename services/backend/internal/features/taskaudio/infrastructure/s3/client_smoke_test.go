package s3

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsS3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// TestS3ServiceSmoke is an opt-in live check. It creates a unique bucket and
// object, then removes only the object this test created.
func TestS3ServiceSmoke(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT is not set")
	}
	accessKey, secretKey := os.Getenv("TEST_S3_ACCESS_KEY"), os.Getenv("TEST_S3_SECRET_KEY")
	if accessKey == "" || secretKey == "" {
		t.Fatal("TEST_S3_ACCESS_KEY and TEST_S3_SECRET_KEY are required with TEST_S3_ENDPOINT")
	}
	bucket := "codex-smoke-" + strings.ToLower(strings.ReplaceAll(time.Now().UTC().Format("20060102t150405.000000000"), ".", ""))
	storage, err := New(Config{Endpoint: endpoint, Region: "us-east-1", Bucket: bucket, AccessKey: accessKey, SecretKey: secretKey, PathStyle: true, CreateBucket: true, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := storage.EnsureBucket(ctx); err != nil {
		t.Fatalf("create smoke bucket: %v", err)
	}
	key, assetID := "probe/task-audio.wav", "codex-smoke-asset"
	defer func() {
		_, _ = storage.client.DeleteObject(context.Background(), &awsS3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		_, _ = storage.client.DeleteBucket(context.Background(), &awsS3.DeleteBucketInput{Bucket: aws.String(bucket)})
	}()
	wav := []byte("RIFF\x04\x00\x00\x00WAVE")
	if err := storage.Put(ctx, bucket, key, assetID, wav); err != nil {
		t.Fatalf("PUT smoke object: %v", err)
	}
	info, found, err := storage.Stat(ctx, bucket, key)
	if err != nil || !found || info.Size != int64(len(wav)) || info.ContentType != "audio/wav" || info.AssetID != assetID {
		t.Fatalf("HEAD smoke object: found=%v info=%+v err=%v", found, info, err)
	}
	body, info, err := storage.Open(ctx, bucket, key)
	if err != nil {
		t.Fatalf("GET smoke object: %v", err)
	}
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil || !bytes.Equal(got, wav) || info.AssetID != assetID {
		t.Fatalf("GET smoke bytes/metadata: %q info=%+v err=%v", got, info, err)
	}
}
