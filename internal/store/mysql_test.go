package store

import "testing"

func TestEscapeLike(t *testing.T) {
	got := escapeLike(`月%_\\`)
	if got != `月\%\_\\\\` {
		t.Fatalf("unexpected escaped value %q", got)
	}
}
