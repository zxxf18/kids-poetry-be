package store

import (
	"strings"
	"testing"
)

func TestEscapeLike(t *testing.T) {
	got := escapeLike(`月%_\\`)
	if got != `月\%\_\\\\` {
		t.Fatalf("unexpected escaped value %q", got)
	}
}

func TestUseFullText(t *testing.T) {
	if !useFullText("静夜思") {
		t.Fatal("multi-character Chinese query should use full text")
	}
	if useFullText("春") {
		t.Fatal("single-character query cannot use a two-character ngram index")
	}
}

func TestFullTextPhrase(t *testing.T) {
	if got := fullTextPhrase(`  明月\"几时有  `); got != `"明月 几时有"` {
		t.Fatalf("unexpected phrase %q", got)
	}
}

func TestIsUnfiltered(t *testing.T) {
	if !isUnfiltered(Query{Page: 1, PageSize: 20}) {
		t.Fatal("pagination alone should still use the dataset audit count")
	}
	if isUnfiltered(Query{Dynasty: "唐", Page: 1, PageSize: 20}) {
		t.Fatal("a dynasty filter requires a filtered count")
	}
	if isUnfiltered(Query{HasTranslation: true, Page: 1, PageSize: 20}) {
		t.Fatal("the learning-data filter requires a filtered count")
	}
}

func TestTagOnlyCountQuery(t *testing.T) {
	query, args, ok := tagOnlyCountQuery(Query{Theme: "山水", Page: 1, PageSize: 20})
	if !ok || query == "" || len(args) != 1 || args[0] != "山水" {
		t.Fatalf("unexpected theme count query: ok=%v query=%q args=%v", ok, query, args)
	}
	_, args, ok = tagOnlyCountQuery(Query{Theme: "山水", Collection: "widely-known"})
	if !ok || len(args) != 2 || args[0] != "widely-known" || args[1] != "山水" {
		t.Fatalf("unexpected combined tag count args: ok=%v args=%v", ok, args)
	}
	if _, _, ok = tagOnlyCountQuery(Query{Theme: "山水", Dynasty: "唐"}); ok {
		t.Fatal("poem fields require the joined count query")
	}
}

func TestBuildWhereUsesIndexedSearchPaths(t *testing.T) {
	where, args := buildWhere(Query{Q: "春"})
	if strings.Contains(where, "content_text LIKE") || len(args) != 2 || args[0] != "春%" || args[1] != "春" {
		t.Fatalf("single-character search must use title prefix and exact author: where=%q args=%v", where, args)
	}
	where, args = buildWhere(Query{Title: "春晓"})
	if !strings.Contains(where, "MATCH(") || !strings.Contains(where, "p.title LIKE") || len(args) != 2 {
		t.Fatalf("multi-character title search must narrow through full text: where=%q args=%v", where, args)
	}
}

func TestSinglePrefixOnly(t *testing.T) {
	if !isSinglePrefixOnly(Query{Q: "春", Page: 1, PageSize: 24}) {
		t.Fatal("a standalone one-character query should use the covering prefix path")
	}
	if isSinglePrefixOnly(Query{Q: "春", Dynasty: "唐", Page: 1, PageSize: 24}) {
		t.Fatal("a filtered query still requires the general search path")
	}
	if isSinglePrefixOnly(Query{Q: "李白", Page: 1, PageSize: 24}) {
		t.Fatal("multi-character queries should use full text")
	}
}
