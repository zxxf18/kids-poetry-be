package importdata

import (
	"testing"

	"github.com/zxxf18/kids-poetry-be/internal/model"
)

func TestValidateRejectsMismatchedPinyin(t *testing.T) {
	p := model.PoemPayload{ID: "1", Title: "静夜思", Author: "李白", Dynasty: "唐", Lines: []string{"床前明月光"}, ContentHash: "x"}
	if validate(p) == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateExpectations(t *testing.T) {
	if err := validateExpectations(10, "abcd", Options{ExpectedCount: 9}); err == nil {
		t.Fatal("expected count mismatch")
	}
	if err := validateExpectations(10, "abcd", Options{ExpectedSHA256: "ABCD"}); err != nil {
		t.Fatalf("hash comparison should be case insensitive: %v", err)
	}
	if err := validateExpectations(10, "abcd", Options{ExpectedSHA256: "ffff"}); err == nil {
		t.Fatal("expected hash mismatch")
	}
}
