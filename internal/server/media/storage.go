package media

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	sharedcfg "github.com/eisen/teamchat/internal/shared/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage interface {
	Put(ctx context.Context, key, contentType string, body []byte) (string, error)
	Configured() bool
}

type NoopStorage struct{}

func (NoopStorage) Put(context.Context, string, string, []byte) (string, error) {
	return "", fmt.Errorf("media storage not configured")
}
func (NoopStorage) Configured() bool { return false }

type R2Storage struct {
	client     *minio.Client
	bucket     string
	publicBase string
}

func NewStorage(cfg sharedcfg.Server) (Storage, error) {
	if cfg.R2Endpoint == "" || cfg.R2AccessKey == "" || cfg.R2SecretKey == "" || cfg.R2Bucket == "" || cfg.R2PublicBase == "" {
		return NoopStorage{}, nil
	}
	endpoint, err := normalizeEndpoint(cfg.R2Endpoint)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.R2AccessKey, cfg.R2SecretKey, ""),
		Secure: strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.R2Endpoint)), "https://"),
		Region: "auto",
	})
	if err != nil {
		return nil, err
	}
	return &R2Storage{
		client:     client,
		bucket:     cfg.R2Bucket,
		publicBase: strings.TrimRight(cfg.R2PublicBase, "/"),
	}, nil
}

func (r *R2Storage) Configured() bool { return r != nil && r.client != nil }

func (r *R2Storage) Put(ctx context.Context, key, contentType string, body []byte) (string, error) {
	_, err := r.client.PutObject(ctx, r.bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}
	return r.publicBase + "/" + key, nil
}

func normalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty R2 endpoint")
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		return u.Host, nil
	}
	return strings.TrimRight(raw, "/"), nil
}

func ReadAllLimited(src io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(src, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", maxBytes)
	}
	return data, nil
}
