package statsclient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEnrichSignsVerifiedVisitorIdentity(t *testing.T) {
	const secret = "stats-secret-for-test"
	client := New("http://stats.test/stats/internal/identity", "poetry", secret)
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Stats-Service") != "poetry" || !strings.Contains(string(body), `"userId":"sub-1"`) {
			t.Errorf("unexpected identity request: headers=%v body=%s", r.Header, body)
		}
		canonical := strings.Join([]string{"poetry", "visitor-1", "sub-1", "Reader", r.Header.Get("X-Stats-Timestamp")}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		if got, want := r.Header.Get("X-Stats-Signature"), hex.EncodeToString(mac.Sum(nil)); got != want {
			t.Errorf("signature = %q, want %q", got, want)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "https://poetry.yebuluo.com.cn/", nil)
	request.AddCookie(&http.Cookie{Name: "ystats_vid_poetry", Value: "visitor-1"})
	client.Enrich(context.Background(), request, "sub-1", "Reader")
}
