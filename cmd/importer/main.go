package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/zxxf18/kids-poetry-be/internal/importdata"
	"github.com/zxxf18/kids-poetry-be/internal/store"
)

func main() {
	source := flag.String("source", env("POETRY_IMPORT_SOURCE", "file"), "file or minio")
	filePath := flag.String("file", env("POETRY_DATA_FILE", ""), "local .jsonl.gz file")
	endpoint := flag.String("endpoint", os.Getenv("POETRY_MINIO_ENDPOINT"), "MinIO endpoint")
	bucket := flag.String("bucket", env("POETRY_MINIO_BUCKET", "kids-poetry-data"), "MinIO bucket")
	objectKey := flag.String("object", env("POETRY_MINIO_OBJECT", "catalog/v1/poems.jsonl.gz"), "MinIO object key")
	version := flag.String("version", os.Getenv("POETRY_DATASET_VERSION"), "dataset version")
	sha256 := flag.String("sha256", os.Getenv("POETRY_DATASET_SHA256"), "dataset SHA-256")
	expectedCount := flag.Int("count", envInt("POETRY_DATASET_COUNT"), "expected record count")
	prune := flag.Bool("prune", env("POETRY_IMPORT_PRUNE", "false") == "true", "remove records absent from this full snapshot")
	useSSL := flag.Bool("ssl", env("POETRY_MINIO_SSL", "false") == "true", "use TLS")
	flag.Parse()
	dsn := os.Getenv("POETRY_DB_DSN")
	s, err := store.Open(dsn)
	if err != nil {
		fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	var reader io.ReadCloser
	switch *source {
	case "file":
		if *filePath == "" {
			fatal(fmt.Errorf("-file is required"))
		}
		reader, err = os.Open(*filePath)
	case "minio":
		if strings.TrimSpace(*endpoint) == "" {
			fatal(fmt.Errorf("MinIO endpoint is required"))
		}
		client, e := minio.New(*endpoint, &minio.Options{Creds: credentials.NewStaticV4(os.Getenv("POETRY_MINIO_ACCESS_KEY"), os.Getenv("POETRY_MINIO_SECRET_KEY"), ""), Secure: *useSSL})
		if e != nil {
			fatal(e)
		}
		if _, e = client.StatObject(ctx, *bucket, *objectKey, minio.StatObjectOptions{}); e != nil {
			fatal(fmt.Errorf("stat MinIO object: %w", e))
		}
		reader, err = client.GetObject(ctx, *bucket, *objectKey, minio.GetObjectOptions{})
	default:
		fatal(fmt.Errorf("unsupported source %q", *source))
	}
	if err != nil {
		fatal(err)
	}
	defer reader.Close()
	result, err := importdata.ImportWithOptions(ctx, s.DB(), reader, importdata.Options{ExpectedCount: *expectedCount, ExpectedSHA256: *sha256, Prune: *prune})
	if err != nil {
		fatal(err)
	}
	if err = ensureSearchIndex(ctx, s.DB()); err != nil {
		fatal(fmt.Errorf("ensure search index: %w", err))
	}
	if strings.TrimSpace(*version) != "" {
		_, err = s.DB().ExecContext(ctx, `INSERT INTO dataset_imports(version,object_key,record_count,sha256)
			VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE object_key=VALUES(object_key),record_count=VALUES(record_count),sha256=VALUES(sha256),imported_at=CURRENT_TIMESTAMP`, *version, *objectKey, result.Imported, result.SHA256)
		if err != nil {
			fatal(fmt.Errorf("record dataset import: %w", err))
		}
	}
	fmt.Printf("imported=%d source=%s\n", result.Imported, *source)
}

func ensureSearchIndex(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema=DATABASE() AND table_name='poems' AND index_name='ft_poems_search'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE poems ADD FULLTEXT KEY ft_poems_search
		(title,author,content_text,translation) WITH PARSER ngram`)
	return err
}
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
func envInt(key string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	return n
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
