package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zxxf18/kids-poetry-be/internal/model"
)

type MySQL struct{ db *sql.DB }

type Query struct {
	Q, Dynasty, Author, Title, Kind, Form, Theme, Cipai, Collection string
	HasTranslation                                                  bool
	Page, PageSize                                                  int
}

type FacetValue struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func Open(dsn string) (*MySQL, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("database DSN is required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	return &MySQL{db: db}, nil
}

func (s *MySQL) Close() error                   { return s.db.Close() }
func (s *MySQL) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *MySQL) List(ctx context.Context, q Query) ([]model.PoemListItem, int, error) {
	where, args := buildWhere(q)
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM poems p "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "p.popular_score DESC, p.id ASC"
	query := `SELECT p.id,p.title,p.author,p.dynasty,p.kind,p.form,p.cipai,
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(p.lines_json,'$[0]')),''),p.themes_json,p.collections_json,
		p.age_min,p.age_max,(CHAR_LENGTH(p.translation)>0),p.popular_score
		FROM poems p ` + where + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]model.PoemListItem, 0, q.PageSize)
	for rows.Next() {
		var item model.PoemListItem
		var themes, collections []byte
		if err := rows.Scan(&item.ID, &item.Title, &item.Author, &item.Dynasty, &item.Kind, &item.Form, &item.Cipai, &item.Excerpt, &themes, &collections, &item.AgeMin, &item.AgeMax, &item.HasTranslation, &item.PopularScore); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(themes, &item.Themes)
		_ = json.Unmarshal(collections, &item.Collections)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func buildWhere(q Query) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 16)
	like := func(column, value string) {
		if value != "" {
			clauses = append(clauses, column+" LIKE ? ESCAPE '\\\\'")
			args = append(args, "%"+escapeLike(value)+"%")
		}
	}
	if q.Q != "" {
		v := "%" + escapeLike(q.Q) + "%"
		clauses = append(clauses, "(p.title LIKE ? ESCAPE '\\\\' OR p.author LIKE ? ESCAPE '\\\\' OR p.content_text LIKE ? ESCAPE '\\\\' OR p.translation LIKE ? ESCAPE '\\\\')")
		args = append(args, v, v, v, v)
	}
	like("p.author", q.Author)
	like("p.title", q.Title)
	for column, value := range map[string]string{"p.dynasty": q.Dynasty, "p.kind": q.Kind, "p.form": q.Form, "p.cipai": q.Cipai} {
		if value != "" {
			clauses = append(clauses, column+"=?")
			args = append(args, value)
		}
	}
	if q.HasTranslation {
		clauses = append(clauses, "CHAR_LENGTH(p.translation)>0")
	}
	if q.Theme != "" {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM poem_tags t WHERE t.poem_id=p.id AND t.dimension='theme' AND t.value=?)")
		args = append(args, q.Theme)
	}
	if q.Collection != "" {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM poem_tags t WHERE t.poem_id=p.id AND t.dimension='collection' AND t.value=?)")
		args = append(args, q.Collection)
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func escapeLike(v string) string {
	r := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return r.Replace(strings.TrimSpace(v))
}

func (s *MySQL) Get(ctx context.Context, id string) (*model.PoemPayload, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,title,author,dynasty,kind,form,cipai,lines_json,pinyin_json,translation,annotations_json,appreciation,themes_json,collections_json,age_min,age_max,popular_score,content_hash,source_name,source_url,source_commit,source_id,license_note FROM poems WHERE id=?`, id)
	var p model.PoemPayload
	var lines, pinyin, annotations, themes, collections []byte
	if err := row.Scan(&p.ID, &p.Title, &p.Author, &p.Dynasty, &p.Kind, &p.Form, &p.Cipai, &lines, &pinyin, &p.Translation, &annotations, &p.Appreciation, &themes, &collections, &p.AgeMin, &p.AgeMax, &p.PopularScore, &p.ContentHash, &p.Source.Name, &p.Source.URL, &p.Source.Commit, &p.Source.SourceID, &p.Source.LicenseNote); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(lines, &p.Lines)
	_ = json.Unmarshal(pinyin, &p.Pinyin)
	_ = json.Unmarshal(annotations, &p.Annotations)
	_ = json.Unmarshal(themes, &p.Themes)
	_ = json.Unmarshal(collections, &p.Collections)
	return &p, nil
}

func (s *MySQL) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM poems").Scan(&n)
	return n, err
}

func (s *MySQL) Facets(ctx context.Context) (map[string][]FacetValue, error) {
	result := map[string][]FacetValue{}
	for key, column := range map[string]string{"dynasties": "dynasty", "forms": "form", "cipais": "cipai", "authors": "author"} {
		rows, err := s.db.QueryContext(ctx, "SELECT "+column+",COUNT(*) n FROM poems WHERE "+column+"<>'' GROUP BY "+column+" ORDER BY n DESC,"+column+" ASC LIMIT 80")
		if err != nil {
			return nil, err
		}
		values := []FacetValue{}
		for rows.Next() {
			var v FacetValue
			if err := rows.Scan(&v.Value, &v.Count); err != nil {
				rows.Close()
				return nil, err
			}
			values = append(values, v)
		}
		rows.Close()
		result[key] = values
	}
	for _, dimension := range []string{"theme", "collection"} {
		rows, err := s.db.QueryContext(ctx, "SELECT value,COUNT(*) n FROM poem_tags WHERE dimension=? GROUP BY value ORDER BY n DESC,value ASC", dimension)
		if err != nil {
			return nil, err
		}
		values := []FacetValue{}
		for rows.Next() {
			var v FacetValue
			if err := rows.Scan(&v.Value, &v.Count); err != nil {
				rows.Close()
				return nil, err
			}
			values = append(values, v)
		}
		rows.Close()
		result[dimension+"s"] = values
	}
	return result, nil
}

func (s *MySQL) DB() *sql.DB { return s.db }
