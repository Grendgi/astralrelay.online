package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/messenger/server/internal/config"
)

// sanitizeEndpoint returns host:port — minio-go expects this format and adds scheme itself.
// Passing "http://minio:9000" causes the lib to build "http://http://minio:9000" and fail validation.
func sanitizeEndpoint(endpoint string) (string, error) {
	// If no scheme, assume it's already host:port
	if !strings.Contains(endpoint, "://") {
		return endpoint, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse S3 endpoint: %w", err)
	}
	host := u.Hostname()
	port := u.Port()
	if port != "" {
		return host + ":" + port, nil
	}
	// Default MinIO port
	if u.Scheme == "https" {
		return host + ":443", nil
	}
	return host + ":9000", nil
}

const maxFileSize = 100 * 1024 * 1024 // 100MB

type Service struct {
	client *minio.Client
	bucket string
}

func New(cfg *config.S3Config) (*Service, error) {
	endpoint, err := sanitizeEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket: %w", err)
		}
	}

	return &Service{client: client, bucket: cfg.Bucket}, nil
}

// objectKey returns S3 key from content URI (blob:sha256:hex -> hex)
func objectKey(contentURI string) (string, error) {
	prefix := "blob:sha256:"
	if !strings.HasPrefix(contentURI, prefix) || len(contentURI) <= len(prefix) {
		return "", fmt.Errorf("invalid content uri")
	}
	hexStr := contentURI[len(prefix):]
	if len(hexStr) != 64 {
		return "", fmt.Errorf("invalid content uri: bad hash length")
	}
	// Validate hex
	if _, err := hex.DecodeString(hexStr); err != nil {
		return "", fmt.Errorf("invalid content uri: %w", err)
	}
	return hexStr, nil
}

func (s *Service) Upload(ctx context.Context, r io.Reader, size int64) (contentURI string, err error) {
	if size > maxFileSize {
		return "", fmt.Errorf("file too large: max %d bytes", maxFileSize)
	}

	data, err := io.ReadAll(io.LimitReader(r, maxFileSize))
	if err != nil {
		return "", err
	}
	if size > 0 && int64(len(data)) != size {
		return "", fmt.Errorf("size mismatch")
	}

	hash := sha256.Sum256(data)
	hexHash := hex.EncodeToString(hash[:])
	contentURI = "blob:sha256:" + hexHash

	_, err = s.client.PutObject(ctx, s.bucket, hexHash, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}

	return contentURI, nil
}

func (s *Service) Download(ctx context.Context, contentURI string) (io.ReadCloser, error) {
	key, err := objectKey(contentURI)
	if err != nil {
		return nil, err
	}

	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}

	return obj, nil
}
