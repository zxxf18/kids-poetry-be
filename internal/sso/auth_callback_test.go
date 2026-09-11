package sso

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// newCallbackFixture wires a Service directly to a local token endpoint. This
// keeps callback tests independent of a real Casdoor instance or production
// credentials while exercising the complete code exchange and verification.
func newCallbackFixture(t *testing.T, mutate func(map[string]any)) (*Service, *http.Request) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "https://casdoor.test"
	clientID := "poetry-client"
	nonce := "test-nonce"
	state := "test-state"
	claims := map[string]any{
		"iss":                issuer,
		"sub":                "user-1",
		"aud":                clientID,
		"azp":                clientID,
		"exp":                time.Now().Add(10 * time.Minute).Unix(),
		"iat":                time.Now().Add(-1 * time.Minute).Unix(),
		"nonce":              nonce,
		"email":              "user@example.com",
		"email_verified":     true,
		"preferred_username": "user",
		"name":               "Test User",
	}
	if mutate != nil {
		mutate(claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(claims))
	token.Header["kid"] = "test-key"
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id_token":"` + raw + `"}`)), Header: make(http.Header), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })

	s := New(Config{Issuer: issuer, ClientID: clientID, ClientSecret: "client-secret", RedirectURL: "https://poetry.test/auth/callback", SessionSecret: "01234567890123456789012345678901"})
	s.authURL = "https://casdoor.test/auth"
	s.tokenURL = "http://token.test/token"
	s.keys = map[string]*rsa.PublicKey{"test-key": &key.PublicKey}
	// Mark discovery as initialized; the callback should use the fixture URLs.
	s.once.Do(func() {})

	r := httptest.NewRequest(http.MethodGet, "/auth/callback?code=test-code&state="+url.QueryEscape(state), nil)
	r.AddCookie(&http.Cookie{Name: "sso_state", Value: state + "|" + nonce + "|/after"})
	return s, r
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func runCallback(t *testing.T, mutate func(map[string]any), tamper func(*http.Request), badSignature bool) *httptest.ResponseRecorder {
	t.Helper()
	s, r := newCallbackFixture(t, mutate)
	if tamper != nil {
		tamper(r)
	}
	if badSignature {
		// Replace the JWKS public key while leaving the token signed by the
		// fixture key, making signature failures deterministic.
		other, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		s.keys["test-key"] = &other.PublicKey
	}
	w := httptest.NewRecorder()
	s.Callback(w, r)
	return w
}

func TestCallbackAcceptsStringAndArrayAudience(t *testing.T) {
	for name, aud := range map[string]any{"string": "poetry-client", "array": []string{"poetry-client"}} {
		t.Run(name, func(t *testing.T) {
			w := runCallback(t, func(c map[string]any) { c["aud"] = aud }, nil, false)
			if w.Code != http.StatusFound {
				t.Fatalf("callback status = %d, body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Location"); got != "/after" {
				t.Fatalf("callback location = %q", got)
			}
			if !strings.Contains(w.Header().Get("Set-Cookie"), "sso_session=") {
				t.Fatalf("session cookie missing: %q", w.Header().Get("Set-Cookie"))
			}
		})
	}
}

func TestCallbackRequiresAudienceAndAuthorizedParty(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"wrong audience", func(c map[string]any) { c["aud"] = "other-client" }},
		{"missing audience", func(c map[string]any) { delete(c, "aud") }},
		{"multi audience missing azp", func(c map[string]any) { c["aud"] = []string{"poetry-client", "other-client"}; delete(c, "azp") }},
		{"multi audience wrong azp", func(c map[string]any) {
			c["aud"] = []string{"poetry-client", "other-client"}
			c["azp"] = "other-client"
		}},
		{"multi audience valid azp", func(c map[string]any) {
			c["aud"] = []string{"poetry-client", "other-client"}
			c["azp"] = "poetry-client"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := runCallback(t, tc.mutate, nil, false)
			want := http.StatusForbidden
			if tc.name == "multi audience valid azp" {
				want = http.StatusFound
			}
			if w.Code != want {
				t.Fatalf("callback status = %d, want %d, body=%s", w.Code, want, w.Body.String())
			}
		})
	}
}

func TestCallbackRejectsInvalidIdentity(t *testing.T) {
	cases := []struct {
		name         string
		mutate       func(map[string]any)
		tamper       func(*http.Request)
		badSignature bool
	}{
		{"wrong nonce", func(c map[string]any) { c["nonce"] = "other" }, nil, false},
		{"missing subject", func(c map[string]any) { delete(c, "sub") }, nil, false},
		{"missing expiry", func(c map[string]any) { delete(c, "exp") }, nil, false},
		{"expired", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, nil, false},
		{"unverified email", func(c map[string]any) { c["email_verified"] = false }, nil, false},
		{"missing email", func(c map[string]any) { delete(c, "email") }, nil, false},
		{"missing state", func(map[string]any) {}, func(r *http.Request) { r.Header.Del("Cookie") }, false},
		{"bad signature", nil, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := runCallback(t, tc.mutate, tc.tamper, tc.badSignature)
			want := http.StatusForbidden
			if tc.name == "missing state" {
				want = http.StatusBadRequest
			}
			if w.Code != want {
				t.Fatalf("callback status = %d, want %d, body=%s", w.Code, want, w.Body.String())
			}
		})
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	w := runCallback(t, nil, func(r *http.Request) {
		q := r.URL.Query()
		q.Set("state", "wrong-state")
		r.URL.RawQuery = q.Encode()
	}, false)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
