package audiostore

import (
	"context"
	"fmt"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/zxxf18/kids-poetry-be/internal/config"
)

type Store struct {
	client *minio.Client
	bucket string
}

func New(c config.Config) (*Store, error) {
	if strings.TrimSpace(c.Audio.Endpoint) == "" {
		return nil, nil
	}
	if c.Audio.AccessKey == "" || c.Audio.SecretKey == "" || c.Audio.Bucket == "" {
		return nil, fmt.Errorf("audio MinIO configuration is incomplete")
	}
	client, err := minio.New(c.Audio.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.Audio.AccessKey, c.Audio.SecretKey, ""),
		Secure: c.Audio.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	return &Store{client: client, bucket: c.Audio.Bucket}, nil
}

func (s *Store) Open(ctx context.Context, objectKey string) (*minio.Object, minio.ObjectInfo, error) {
	object, err := s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, minio.ObjectInfo{}, err
	}
	info, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return nil, minio.ObjectInfo{}, err
	}
	return object, info, nil
}
