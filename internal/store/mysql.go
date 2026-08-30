package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

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
	if isSinglePrefixOnly(q) {
		return s.listSinglePrefix(ctx, q)
	}
	from, where, scopeArgs := buildScope(q)
	var total int
	if isUnfiltered(q) {
		var err error
		total, err = s.Count(ctx)
		if err != nil {
			return nil, 0, err
		}
	} else if query, countArgs, ok := tagOnlyCountQuery(q); ok {
		if err := s.db.QueryRowContext(ctx, query, countArgs...).Scan(&total); err != nil {
			return nil, 0, err
		}
	} else if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) "+from+" "+where, scopeArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rankSelect := "p.id,p.popular_score"
	innerOrder := "p.popular_score DESC,p.id ASC"
	outerOrder := "ranked.popular_score DESC,ranked.id ASC"
	queryArgs := append([]any(nil), scopeArgs...)
	if useFullText(q.Q) {
		v := "%" + escapeLike(q.Q) + "%"
		rankSelect += `,(p.author=?) AS author_exact,(p.title=?) AS title_exact,
			(p.title LIKE ? ESCAPE '\\') AS title_contains,
			MATCH(p.title,p.author,p.content_text,p.translation) AGAINST (? IN BOOLEAN MODE) AS relevance`
		innerOrder = "author_exact DESC,title_exact DESC,title_contains DESC,p.popular_score DESC,relevance DESC,p.id ASC"
		outerOrder = "ranked.author_exact DESC,ranked.title_exact DESC,ranked.title_contains DESC,ranked.popular_score DESC,ranked.relevance DESC,ranked.id ASC"
		rankArgs := []any{q.Q, q.Q, v, fullTextPhrase(q.Q)}
		queryArgs = append(rankArgs, scopeArgs...)
	}
	query := `SELECT p.id,p.title,p.author,p.dynasty,p.kind,p.form,p.cipai,
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(p.lines_json,'$[0]')),''),p.themes_json,p.collections_json,
		p.age_min,p.age_max,(JSON_LENGTH(p.pinyin_json)>0),(CHAR_LENGTH(p.translation)>0),(JSON_LENGTH(p.annotations_json)>0),p.popular_score
		FROM (` + "SELECT " + rankSelect + " " + from + " " + where + " ORDER BY " + innerOrder + ` LIMIT ? OFFSET ?
		) ranked JOIN poems p ON p.id=ranked.id ORDER BY ` + outerOrder
	queryArgs = append(queryArgs, q.PageSize, (q.Page-1)*q.PageSize)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	return scanPoemListRows(rows, q.PageSize, total)
}

func isSinglePrefixOnly(q Query) bool {
	return q.Q != "" && !useFullText(q.Q) && q.Dynasty == "" && q.Author == "" && q.Title == "" && q.Kind == "" && q.Form == "" && q.Theme == "" && q.Cipai == "" && q.Collection == "" && !q.HasTranslation
}

func (s *MySQL) listSinglePrefix(ctx context.Context, q Query) ([]model.PoemListItem, int, error) {
	prefix := escapeLike(q.Q) + "%"
	candidates := `(SELECT id,popular_score FROM poems FORCE INDEX (idx_poems_title)
		WHERE title LIKE ? ESCAPE '\\'
		UNION
		SELECT id,popular_score FROM poems FORCE INDEX (idx_poems_author)
		WHERE author LIKE ? ESCAPE '\\')`
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+candidates+" candidates", prefix, prefix).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `SELECT p.id,p.title,p.author,p.dynasty,p.kind,p.form,p.cipai,
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(p.lines_json,'$[0]')),''),p.themes_json,p.collections_json,
		p.age_min,p.age_max,(JSON_LENGTH(p.pinyin_json)>0),(CHAR_LENGTH(p.translation)>0),(JSON_LENGTH(p.annotations_json)>0),p.popular_score
		FROM (SELECT candidates.id,candidates.popular_score FROM ` + candidates + ` candidates
			ORDER BY candidates.popular_score DESC,candidates.id ASC LIMIT ? OFFSET ?
		) ranked JOIN poems p ON p.id=ranked.id
		ORDER BY ranked.popular_score DESC,ranked.id ASC`
	rows, err := s.db.QueryContext(ctx, query, prefix, prefix, q.PageSize, (q.Page-1)*q.PageSize)
	if err != nil {
		return nil, 0, err
	}
	return scanPoemListRows(rows, q.PageSize, total)
}

func scanPoemListRows(rows *sql.Rows, capacity, total int) ([]model.PoemListItem, int, error) {
	defer rows.Close()
	items := make([]model.PoemListItem, 0, capacity)
	for rows.Next() {
		var item model.PoemListItem
		var themes, collections []byte
		if err := rows.Scan(&item.ID, &item.Title, &item.Author, &item.Dynasty, &item.Kind, &item.Form, &item.Cipai, &item.Excerpt, &themes, &collections, &item.AgeMin, &item.AgeMax, &item.HasPinyin, &item.HasTranslation, &item.HasAnnotations, &item.PopularScore); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(themes, &item.Themes)
		_ = json.Unmarshal(collections, &item.Collections)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func isUnfiltered(q Query) bool {
	return q.Q == "" && q.Dynasty == "" && q.Author == "" && q.Title == "" && q.Kind == "" && q.Form == "" && q.Theme == "" && q.Cipai == "" && q.Collection == "" && !q.HasTranslation
}

func buildScope(q Query) (string, string, []any) {
	from := "FROM poems p"
	args := make([]any, 0, 18)
	if q.Theme != "" {
		from += " JOIN poem_tags theme_tag ON theme_tag.poem_id=p.id AND theme_tag.dimension='theme' AND theme_tag.value=?"
		args = append(args, q.Theme)
	}
	if q.Collection != "" {
		from += " JOIN poem_tags collection_tag ON collection_tag.poem_id=p.id AND collection_tag.dimension='collection' AND collection_tag.value=?"
		args = append(args, q.Collection)
	}
	where, whereArgs := buildWhere(q)
	return from, where, append(args, whereArgs...)
}

func buildWhere(q Query) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 16)
	if q.Q != "" {
		if useFullText(q.Q) {
			clauses = append(clauses, "MATCH(p.title,p.author,p.content_text,p.translation) AGAINST (? IN BOOLEAN MODE)")
			args = append(args, fullTextPhrase(q.Q))
		} else {
			clauses = append(clauses, "(p.title LIKE ? ESCAPE '\\\\' OR p.author=?)")
			args = append(args, escapeLike(q.Q)+"%", q.Q)
		}
	}
	if q.Author != "" {
		clauses = append(clauses, "p.author=?")
		args = append(args, q.Author)
	}
	if q.Title != "" {
		if useFullText(q.Title) {
			clauses = append(clauses, "MATCH(p.title,p.author,p.content_text,p.translation) AGAINST (? IN BOOLEAN MODE) AND p.title LIKE ? ESCAPE '\\\\'")
			args = append(args, fullTextPhrase(q.Title), "%"+escapeLike(q.Title)+"%")
		} else {
			clauses = append(clauses, "p.title LIKE ? ESCAPE '\\\\'")
			args = append(args, escapeLike(q.Title)+"%")
		}
	}
	for column, value := range map[string]string{"p.dynasty": q.Dynasty, "p.kind": q.Kind, "p.form": q.Form, "p.cipai": q.Cipai} {
		if value != "" {
			clauses = append(clauses, column+"=?")
			args = append(args, value)
		}
	}
	if q.HasTranslation {
		clauses = append(clauses, "CHAR_LENGTH(p.translation)>0")
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func tagOnlyCountQuery(q Query) (string, []any, bool) {
	if q.Q != "" || q.Dynasty != "" || q.Author != "" || q.Title != "" || q.Kind != "" || q.Form != "" || q.Cipai != "" || q.HasTranslation {
		return "", nil, false
	}
	if q.Theme != "" && q.Collection != "" {
		return `SELECT COUNT(*) FROM poem_tags theme_tag
			JOIN poem_tags collection_tag ON collection_tag.poem_id=theme_tag.poem_id
				AND collection_tag.dimension='collection' AND collection_tag.value=?
			WHERE theme_tag.dimension='theme' AND theme_tag.value=?`, []any{q.Collection, q.Theme}, true
	}
	if q.Theme != "" {
		return "SELECT COUNT(*) FROM poem_tags WHERE dimension='theme' AND value=?", []any{q.Theme}, true
	}
	if q.Collection != "" {
		return "SELECT COUNT(*) FROM poem_tags WHERE dimension='collection' AND value=?", []any{q.Collection}, true
	}
	return "", nil, false
}

func useFullText(value string) bool {
	return utf8.RuneCountInString(strings.TrimSpace(value)) >= 2
}

func fullTextPhrase(value string) string {
	cleaned := strings.NewReplacer(`\\`, " ", `\"`, " ").Replace(strings.TrimSpace(value))
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return `"` + cleaned + `"`
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
	if p.Lines == nil {
		p.Lines = []string{}
	}
	if p.Pinyin == nil {
		p.Pinyin = []string{}
	}
	if p.Annotations == nil {
		p.Annotations = []string{}
	}
	if p.Themes == nil {
		p.Themes = []string{}
	}
	if p.Collections == nil {
		p.Collections = []string{}
	}
	return &p, nil
}

func (s *MySQL) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT record_count FROM dataset_imports ORDER BY imported_at DESC LIMIT 1").Scan(&n)
	if err == nil {
		return n, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM poems").Scan(&n)
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
