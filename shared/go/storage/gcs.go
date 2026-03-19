package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

var (
	ErrObjectNotFound = errors.New("object not found")
	ErrBucketRequired = errors.New("bucket name is required")
)

// Config holds GCS client configuration.
type Config struct {
	Bucket   string // GCS bucket name
	Endpoint string // custom endpoint for fake-gcs-server in dev (empty = production)
}

// Client wraps the GCS client for snapshot upload/download.
type Client struct {
	bucket *storage.BucketHandle
	log    *zap.Logger
	cfg    Config
}

// NewClient creates a GCS storage client.
// If cfg.Endpoint is set, it connects to that endpoint (for fake-gcs-server in dev).
// Otherwise, it uses default GCP credentials.
func NewClient(ctx context.Context, cfg Config, log *zap.Logger) (*Client, error) {
	if cfg.Bucket == "" {
		return nil, ErrBucketRequired
	}

	var opts []option.ClientOption
	if cfg.Endpoint != "" {
		opts = append(opts,
			option.WithEndpoint(cfg.Endpoint),
			option.WithoutAuthentication(),
		)
	}

	gcs, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gcs client: %w", err)
	}

	return &Client{
		bucket: gcs.Bucket(cfg.Bucket),
		log:    log,
		cfg:    cfg,
	}, nil
}

// Upload writes data to the given object path in the bucket.
func (c *Client) Upload(ctx context.Context, path string, data []byte) error {
	w := c.bucket.Object(path).NewWriter(ctx)
	w.ContentType = "application/json"

	if _, err := w.Write(data); err != nil {
		if closeErr := w.Close(); closeErr != nil {
			c.log.Warn("failed to close gcs writer after write error", zap.String("path", path), zap.Error(closeErr))
		}
		return fmt.Errorf("write object %q: %w", path, err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close writer %q: %w", path, err)
	}

	return nil
}

// Download reads the object at the given path from the bucket.
func (c *Client) Download(ctx context.Context, path string) ([]byte, error) {
	r, err := c.bucket.Object(path).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("open object %q: %w", path, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			c.log.Warn("failed to close gcs reader", zap.String("path", path), zap.Error(err))
		}
	}()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read object %q: %w", path, err)
	}

	return data, nil
}

// SnapshotPath builds the GCS object path for a snapshot.
// Format: {org_uuid}/{repo_uuid}/{commit_sha}/snapshot.json
func SnapshotPath(orgID, repoID, commitSHA string) string {
	return fmt.Sprintf("%s/%s/%s/snapshot.json", orgID, repoID, commitSHA)
}
