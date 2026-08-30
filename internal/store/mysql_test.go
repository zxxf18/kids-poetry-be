package store

import "testing"

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
