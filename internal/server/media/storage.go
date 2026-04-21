package media

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	sharedcfg "github.com/eisen/teamchat/internal/shared/config"
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
	client     *s3.Client
	bucket     string
	publicBase string
}

func NewStorage(cfg sharedcfg.Server) (Storage, error) {
	if cfg.R2Endpoint == "" || cfg.R2AccessKey == "" || cfg.R2SecretKey == "" || cfg.R2Bucket == "" || cfg.R2PublicBase == "" {
		return NoopStorage{}, nil
	}
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.R2AccessKey, cfg.R2SecretKey, "")),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(strings.TrimRight(cfg.R2Endpoint, "/"))
	})
	return &R2Storage{
		client:     client,
		bucket:     cfg.R2Bucket,
		publicBase: strings.TrimRight(cfg.R2PublicBase, "/"),
	}, nil
}

func (r *R2Storage) Configured() bool { return r != nil && r.client != nil }

func (r *R2Storage) Put(ctx context.Context, key, contentType string, body []byte) (string, error) {
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	return r.publicBase + "/" + key, nil
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
