package importdata

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/zxxf18/kids-poetry-be/internal/model"
)

type Result struct {
	Imported int
	SHA256   string
}

type Options struct {
	ExpectedCount  int
	ExpectedSHA256 string
	Prune          bool
}

func Import(ctx context.Context, db *sql.DB, input io.Reader) (Result, error) {
	return ImportWithOptions(ctx, db, input, Options{})
}

func ImportWithOptions(ctx context.Context, db *sql.DB, input io.Reader, options Options) (Result, error) {
	hash := sha256.New()
	gz, err := gzip.NewReader(io.TeeReader(input, hash))
	if err != nil {
		return Result{}, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO poems (id,title,author,dynasty,kind,form,cipai,content_text,lines_json,pinyin_json,translation,annotations_json,appreciation,themes_json,collections_json,age_min,age_max,popular_score,content_hash,source_name,source_url,source_commit,source_id,license_note)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON DUPLICATE KEY UPDATE title=VALUES(title),author=VALUES(author),dynasty=VALUES(dynasty),kind=VALUES(kind),form=VALUES(form),cipai=VALUES(cipai),content_text=VALUES(content_text),lines_json=VALUES(lines_json),pinyin_json=VALUES(pinyin_json),translation=VALUES(translation),annotations_json=VALUES(annotations_json),appreciation=VALUES(appreciation),themes_json=VALUES(themes_json),collections_json=VALUES(collections_json),age_min=VALUES(age_min),age_max=VALUES(age_max),popular_score=VALUES(popular_score),content_hash=VALUES(content_hash),source_name=VALUES(source_name),source_url=VALUES(source_url),source_commit=VALUES(source_commit),source_id=VALUES(source_id),license_note=VALUES(license_note)`)
	if err != nil {
		return Result{}, err
	}
	defer stmt.Close()

	tagStmt, err := tx.PrepareContext(ctx, "INSERT IGNORE INTO poem_tags (poem_id,dimension,value) VALUES (?,?,?)")
	if err != nil {
		return Result{}, err
	}
	defer tagStmt.Close()

	var idStmt *sql.Stmt
	if options.Prune {
		if _, err = tx.ExecContext(ctx, "CREATE TEMPORARY TABLE import_poem_ids (id VARCHAR(64) COLLATE utf8mb4_unicode_ci PRIMARY KEY) ENGINE=MEMORY DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"); err != nil {
			return Result{}, err
		}
		idStmt, err = tx.PrepareContext(ctx, "INSERT IGNORE INTO import_poem_ids(id) VALUES (?)")
		if err != nil {
			return Result{}, err
		}
		defer idStmt.Close()
	}

	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	count := 0
	for scanner.Scan() {
		var p model.PoemPayload
		if err := json.Unmarshal(scanner.Bytes(), &p); err != nil {
			return Result{}, fmt.Errorf("decode record %d: %w", count+1, err)
		}
		if err := validate(p); err != nil {
			return Result{}, fmt.Errorf("record %d: %w", count+1, err)
		}
		if idStmt != nil {
			if _, err = idStmt.ExecContext(ctx, p.ID); err != nil {
				return Result{}, err
			}
		}

		lines, _ := json.Marshal(p.Lines)
		pinyin, _ := json.Marshal(p.Pinyin)
		annotations, _ := json.Marshal(p.Annotations)
		themes, _ := json.Marshal(p.Themes)
		collections, _ := json.Marshal(p.Collections)
		_, err = stmt.ExecContext(ctx, p.ID, p.Title, p.Author, p.Dynasty, p.Kind, p.Form, p.Cipai, strings.Join(p.Lines, "\n"), lines, pinyin, p.Translation, annotations, p.Appreciation, themes, collections, p.AgeMin, p.AgeMax, p.PopularScore, p.ContentHash, p.Source.Name, p.Source.URL, p.Source.Commit, p.Source.SourceID, p.Source.LicenseNote)
		if err != nil {
			return Result{}, fmt.Errorf("upsert %s: %w", p.ID, err)
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM poem_tags WHERE poem_id=?", p.ID); err != nil {
			return Result{}, err
		}
		for _, theme := range p.Themes {
			if _, err = tagStmt.ExecContext(ctx, p.ID, "theme", theme); err != nil {
				return Result{}, err
			}
		}
		for _, collection := range p.Collections {
			if _, err = tagStmt.ExecContext(ctx, p.ID, "collection", collection); err != nil {
				return Result{}, err
			}
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return Result{}, err
	}
	if count == 0 {
		return Result{}, fmt.Errorf("dataset is empty")
	}

	actualSHA256 := hex.EncodeToString(hash.Sum(nil))
	if err := validateExpectations(count, actualSHA256, options); err != nil {
		return Result{}, err
	}
	if options.Prune {
		if _, err = tx.ExecContext(ctx, "DELETE p FROM poems p LEFT JOIN import_poem_ids i ON i.id=p.id WHERE i.id IS NULL"); err != nil {
			return Result{}, fmt.Errorf("prune stale poems: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return Result{Imported: count, SHA256: actualSHA256}, nil
}

func validateExpectations(count int, actualSHA256 string, options Options) error {
	if options.ExpectedCount > 0 && count != options.ExpectedCount {
		return fmt.Errorf("record count mismatch: expected %d, got %d", options.ExpectedCount, count)
	}
	if expected := strings.ToLower(strings.TrimSpace(options.ExpectedSHA256)); expected != "" && actualSHA256 != expected {
		return fmt.Errorf("SHA-256 mismatch: expected %s, got %s", expected, actualSHA256)
	}
	return nil
}

func validate(p model.PoemPayload) error {
	if p.ID == "" || p.Title == "" || p.Author == "" || p.Dynasty == "" {
		return fmt.Errorf("missing identity fields")
	}
	if len(p.Lines) == 0 || len(p.Pinyin) != len(p.Lines) {
		return fmt.Errorf("content and pinyin lines do not align")
	}
	if p.ContentHash == "" {
		return fmt.Errorf("content hash is required")
	}
	return nil
}
