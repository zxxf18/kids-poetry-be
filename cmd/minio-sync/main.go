package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type objectFile struct {
	name        string
	contentType string
}

func main() {
	endpoint := flag.String("endpoint", env("POETRY_MINIO_ENDPOINT", "minio:9000"), "MinIO endpoint")
	bucket := flag.String("bucket", env("POETRY_MINIO_BUCKET", "kids-poetry-data"), "private bucket")
	directory := flag.String("directory", env("POETRY_UPLOAD_DIRECTORY", "/upload"), "directory containing release files")
	prefix := flag.String("prefix", env("POETRY_MINIO_PREFIX", "catalog/2026-08-30.v1"), "object prefix")
	useSSL := flag.Bool("ssl", env("POETRY_MINIO_SSL", "false") == "true", "use TLS")
	flag.Parse()

	client, err := minio.New(*endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("POETRY_MINIO_ACCESS_KEY"), os.Getenv("POETRY_MINIO_SECRET_KEY"), ""),
		Secure: *useSSL,
	})
	if err != nil {
		fatal(err)
	}
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, *bucket)
	if err != nil {
		fatal(err)
	}
	if !exists {
		if err = client.MakeBucket(ctx, *bucket, minio.MakeBucketOptions{}); err != nil {
			fatal(err)
		}
	}

	files := []objectFile{
		{"poems.jsonl.gz", "application/gzip"},
		{"manifest.json", "application/json; charset=utf-8"},
		{"ATTRIBUTION.md", "text/markdown; charset=utf-8"},
	}
	for _, file := range files {
		localPath := filepath.Join(*directory, file.name)
		info, statErr := os.Stat(localPath)
		if statErr != nil {
			fatal(statErr)
		}
		objectKey := strings.Trim(strings.TrimSpace(*prefix), "/") + "/" + file.name
		upload, putErr := client.FPutObject(ctx, *bucket, objectKey, localPath, minio.PutObjectOptions{ContentType: file.contentType})
		if putErr != nil {
			fatal(putErr)
		}
		fmt.Printf("uploaded=%s/%s size=%d etag=%s localSize=%d\n", *bucket, objectKey, upload.Size, upload.ETag, info.Size())
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
