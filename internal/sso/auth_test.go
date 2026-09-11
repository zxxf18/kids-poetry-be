package sso

import "testing"

func TestSessionRoundTripAndExpiry(t *testing.T) {
	s := New(Config{SessionSecret: "01234567890123456789012345678901"})
	raw, err := s.sign(session{Subject: "sub-1", Email: "user@example.com", EmailVerified: true, ExpiresAt: 4102444800})
	if err != nil {
		t.Fatal(err)
	}
	var got session
	if err := s.verify(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Subject != "sub-1" || got.Email != "user@example.com" {
		t.Fatalf("unexpected session: %+v", got)
	}
}

func TestSafeReturnToRejectsExternalURL(t *testing.T) {
	if got := safeReturnTo("https://evil.example/steal"); got != "/" {
		t.Fatalf("external URL was accepted: %q", got)
	}
	if got := safeReturnTo("/poems/1?q=x"); got != "/poems/1?q=x" {
		t.Fatalf("internal URL changed: %q", got)
	}
}
