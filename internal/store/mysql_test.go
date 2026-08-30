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
