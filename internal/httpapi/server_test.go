package httpapi

import "testing"

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
