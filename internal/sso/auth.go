package sso

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

type Config struct{ Issuer, ClientID, ClientSecret, RedirectURL, SessionSecret, CookieName, AdminEmails string }
type Service struct {
	cfg               Config
	once              sync.Once
	authURL, tokenURL string
	keys              map[string]*rsa.PublicKey
	err               error
}
type session struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	EmailVerified bool   `json:"email_verified"`
	Role          string `json:"role"`
	ExpiresAt     int64  `json:"exp"`
}
type stateKey struct{}

func New(c Config) *Service {
	if c.CookieName == "" {
		c.CookieName = "sso_session"
	}
	return &Service{cfg: c}
}
func (s *Service) init(ctx context.Context) {
	s.once.Do(func() {
		if s.cfg.Issuer == "" || s.cfg.ClientID == "" || s.cfg.ClientSecret == "" || s.cfg.RedirectURL == "" {
			s.err = errors.New("OIDC is not configured")
			return
		}
		issuer := strings.TrimSuffix(s.cfg.Issuer, "/")
		var doc struct {
			Issuer   string `json:"issuer"`
			AuthURL  string `json:"authorization_endpoint"`
			TokenURL string `json:"token_endpoint"`
			JWKSURL  string `json:"jwks_uri"`
		}
		if err := getJSON(ctx, issuer+"/.well-known/openid-configuration", &doc); err != nil {
			s.err = err
			return
		}
		if doc.Issuer != issuer || doc.AuthURL == "" || doc.TokenURL == "" || doc.JWKSURL == "" {
			s.err = errors.New("invalid OIDC discovery document")
			return
		}
		var jwks struct {
			Keys []struct {
				Kty string `json:"kty"`
				Kid string `json:"kid"`
				N   string `json:"n"`
				E   string `json:"e"`
			} `json:"keys"`
		}
		if err := getJSON(ctx, doc.JWKSURL, &jwks); err != nil {
			s.err = err
			return
		}
		s.keys = make(map[string]*rsa.PublicKey)
		for _, k := range jwks.Keys {
			if k.Kty != "RSA" || k.Kid == "" {
				continue
			}
			n, e1 := base64.RawURLEncoding.DecodeString(k.N)
			e2, err := base64.RawURLEncoding.DecodeString(k.E)
			if err != nil || e1 != nil {
				continue
			}
			ei := 0
			for _, b := range e2 {
				ei = ei*256 + int(b)
			}
			if ei == 0 {
				continue
			}
			s.keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: ei}
		}
		if len(s.keys) == 0 {
			s.err = errors.New("OIDC JWKS has no RSA keys")
			return
		}
		s.authURL, s.tokenURL = doc.AuthURL, doc.TokenURL
	})
}
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	s.init(r.Context())
	if s.err != nil {
		http.Error(w, "SSO is not configured", 503)
		return
	}
	state, _ := randomToken(32)
	nonce, _ := randomToken(32)
	ret := safeReturnTo(r.URL.Query().Get("return_to"))
	setCookie(w, "sso_state", state+"|"+nonce+"|"+ret, 600)
	q := url.Values{"response_type": {"code"}, "client_id": {s.cfg.ClientID}, "redirect_uri": {s.cfg.RedirectURL}, "scope": {"openid profile email"}, "state": {state}, "nonce": {nonce}}
	http.Redirect(w, r, s.authURL+"?"+q.Encode(), 302)
}
func (s *Service) Callback(w http.ResponseWriter, r *http.Request) {
	s.init(r.Context())
	if s.err != nil {
		http.Error(w, "SSO is not configured", 503)
		return
	}
	c, err := r.Cookie("sso_state")
	if err != nil {
		http.Error(w, "invalid SSO state", 400)
		return
	}
	p := strings.SplitN(c.Value, "|", 3)
	if len(p) != 3 || p[0] == "" || p[1] == "" || !hmac.Equal([]byte(p[0]), []byte(r.URL.Query().Get("state"))) {
		http.Error(w, "invalid SSO state", 400)
		return
	}
	raw, err := s.exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "SSO token exchange failed", 401)
		return
	}
	claims := struct {
		jwt.RegisteredClaims
		AuthorizedParty   string `json:"azp"`
		Nonce             string `json:"nonce"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}{}
	tok, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodRS256 {
			return nil, errors.New("unsupported signing algorithm")
		}
		kid, _ := t.Header["kid"].(string)
		key := s.keys[kid]
		if key == nil {
			return nil, errors.New("unknown signing key")
		}
		return key, nil
	})
	// RegisteredClaims accepts both OIDC audience encodings, including Casdoor's array.
	partyValid := claims.AuthorizedParty == s.cfg.ClientID || (claims.AuthorizedParty == "" && len(claims.Audience) == 1)
	if err != nil || tok == nil || !tok.Valid || claims.Issuer != strings.TrimSuffix(s.cfg.Issuer, "/") || !claims.VerifyAudience(s.cfg.ClientID, true) || !partyValid || claims.Nonce != p[1] || claims.ExpiresAt == nil || strings.TrimSpace(claims.Subject) == "" || !claims.EmailVerified || strings.TrimSpace(claims.Email) == "" {
		http.Error(w, "invalid or unverified SSO identity", 403)
		return
	}
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	name := claims.Name
	if name == "" {
		name = username
	}
	role := "user"
	for _, a := range strings.Split(s.cfg.AdminEmails, ",") {
		if strings.EqualFold(strings.TrimSpace(a), claims.Email) {
			role = "admin"
		}
	}
	encoded, err := s.sign(session{Subject: claims.Subject, Email: claims.Email, Username: username, DisplayName: name, EmailVerified: true, Role: role, ExpiresAt: time.Now().Add(24 * time.Hour).Unix()})
	if err != nil {
		http.Error(w, "SSO session unavailable", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cfg.CookieName, Value: encoded, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400})
	setCookie(w, "sso_state", "", -1)
	http.Redirect(w, r, safeReturnTo(p[2]), 302)
}
func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	v, e := s.read(r)
	if e != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, 200, v)
}
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: s.cfg.CookieName, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(204)
}
func (s *Service) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, e := s.read(r)
		if e != nil {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), stateKey{}, v)))
	}
}
func (s *Service) read(r *http.Request) (session, error) {
	c, e := r.Cookie(s.cfg.CookieName)
	if e != nil {
		return session{}, e
	}
	var v session
	if e = s.verify(c.Value, &v); e != nil || v.ExpiresAt <= time.Now().Unix() || !v.EmailVerified {
		return session{}, errors.New("invalid session")
	}
	return v, nil
}
func (s *Service) sign(v session) (string, error) {
	if len(s.cfg.SessionSecret) < 32 {
		return "", errors.New("session secret must be at least 32 bytes")
	}
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	p := base64.RawURLEncoding.EncodeToString(b)
	m := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	m.Write([]byte(p))
	return p + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil)), nil
}
func (s *Service) verify(raw string, v *session) error {
	p := strings.Split(raw, ".")
	if len(p) != 2 || len(s.cfg.SessionSecret) < 32 {
		return errors.New("invalid session")
	}
	m := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	m.Write([]byte(p[0]))
	sig, e := base64.RawURLEncoding.DecodeString(p[1])
	if e != nil || !hmac.Equal(sig, m.Sum(nil)) {
		return errors.New("invalid session")
	}
	b, e := base64.RawURLEncoding.DecodeString(p[0])
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (s *Service) exchange(ctx context.Context, code string) (string, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.cfg.RedirectURL}, "client_id": {s.cfg.ClientID}, "client_secret": {s.cfg.ClientSecret}}
	req, e := http.NewRequestWithContext(ctx, "POST", s.tokenURL, strings.NewReader(form.Encode()))
	if e != nil {
		return "", e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	var out struct {
		IDToken string `json:"id_token"`
	}
	if resp.StatusCode != 200 {
		return "", errors.New("token endpoint rejected code")
	}
	if e = json.NewDecoder(resp.Body).Decode(&out); e != nil || out.IDToken == "" {
		return "", errors.New("missing id_token")
	}
	return out.IDToken, nil
}
func getJSON(ctx context.Context, raw string, out any) error {
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return e
	}
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("OIDC endpoint unavailable")
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	_, e := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func setCookie(w http.ResponseWriter, n, v string, age int) {
	http.SetCookie(w, &http.Cookie{Name: n, Value: v, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func safeReturnTo(raw string) string {
	if raw == "" {
		return "/"
	}
	u, e := url.Parse(raw)
	if e != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "/"
	}
	return u.RequestURI()
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
