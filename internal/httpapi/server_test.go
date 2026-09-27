package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zxxf18/kids-poetry-be/internal/sso"
)

func TestListNeedsAuthMatchesLegacyProtectedQueries(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "plain catalog", path: "/api/v1/poems", want: false},
		{name: "search", path: "/api/v1/poems?q=李白", want: true},
		{name: "whitespace search is ignored", path: "/api/v1/poems?q=%20%20", want: false},
		{name: "collection", path: "/api/v1/poems?collection=widely-known", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if got := listNeedsAuth(r); got != tt.want {
				t.Fatalf("listNeedsAuth(%s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestRequireAuthSwitchDefaultsToAnonymous(t *testing.T) {
	next := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	a := &API{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/poems/1", nil)
	recorder := httptest.NewRecorder()
	a.requireAuth(next)(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("anonymous policy returned %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRequireAuthSwitchProtectsWhenEnabled(t *testing.T) {
	next := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	a := &API{auth: sso.New(sso.Config{SessionSecret: "01234567890123456789012345678901"}), authRequired: true}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/poems/1", nil)
	recorder := httptest.NewRecorder()
	a.requireAuth(next)(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("protected policy returned %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestRandomSampleReturnsDistinctBoundedCandidates(t *testing.T) {
	items := make([]int, 300)
	for index := range items {
		items[index] = index
	}
	sample := randomSample(items, 12)
	if len(sample) != 12 {
		t.Fatalf("expected 12 candidates, got %d", len(sample))
	}
	seen := map[int]bool{}
	for _, item := range sample {
		if item < 0 || item >= 300 || seen[item] {
			t.Fatalf("invalid or duplicate candidate %d", item)
		}
		seen[item] = true
	}
}

func TestRandomSampleKeepsShortCandidateSet(t *testing.T) {
	items := []int{1, 2, 3}
	if sample := randomSample(items, 12); len(sample) != 3 {
		t.Fatalf("expected all short candidates, got %d", len(sample))
	}
}
