package storage

import (
	"bytes"
	"context"
	"io"

	"github.com/cogent/services/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

// Client wraps minio-go for fetching and uploading full content payloads.
type Client struct {
	client *minio.Client
	bucket string
	logger *zap.Logger
	noop   bool
}

// NewClient creates a Client from config.
// Returns a no-op client if cfg.MinioEndpoint is empty.
func NewClient(cfg config.Config, logger *zap.Logger) (*Client, error) {
	if cfg.MinioEndpoint == "" {
		return &Client{noop: true, logger: logger}, nil
	}
	mc, err := minio.New(cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioSecure,
	})
	if err != nil {
		return nil, err
	}
	return &Client{client: mc, bucket: cfg.MinioBucket, logger: logger}, nil
}

// Fetch downloads and returns full content for a given S3 ref key.
// ref_key format: "{trace_id}/{span_id}/{field_name}"
func (c *Client) Fetch(ctx context.Context, key string) (string, error) {
	if c.noop {
		return "", nil
	}
	obj, err := c.client.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return "", err
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Put uploads content to S3. Used primarily in tests.
func (c *Client) Put(ctx context.Context, key string, content string) error {
	if c.noop {
		return nil
	}
	data := []byte(content)
	_, err := c.client.PutObject(ctx, c.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "text/plain; charset=utf-8"})
	return err
}
