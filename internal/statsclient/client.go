package statsclient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client enriches an already recorded yebuluo-stats visit with the
// server-verified Casdoor identity. The browser never receives the HMAC key.
type Client struct {
	endpoint string
	service  string
	secret   string
	http     *http.Client
}

func New(endpoint, service, secret string) *Client {
	return &Client{endpoint: strings.TrimSpace(endpoint), service: strings.TrimSpace(service), secret: secret, http: &http.Client{Timeout: 2 * time.Second}}
}

func (c *Client) Enrich(ctx context.Context, request *http.Request, userID, userName string) {
	if c == nil || c.endpoint == "" || c.service == "" || len(c.secret) < 16 || strings.TrimSpace(userID) == "" {
		return
	}
	cookie, err := request.Cookie("ystats_vid_" + c.service)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	bodyValue := struct {
		VisitorID string `json:"visitorId"`
		UserID    string `json:"userId"`
		UserName  string `json:"userName"`
	}{VisitorID: cookie.Value, UserID: userID, UserName: userName}
	body, err := json.Marshal(bodyValue)
	if err != nil {
		return
	}
	mac := hmac.New(sha256.New, []byte(c.secret))
	_, _ = mac.Write([]byte(strings.Join([]string{c.service, cookie.Value, userID, userName, timestamp}, "\n")))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Stats-Service", c.service)
	req.Header.Set("X-Stats-Timestamp", timestamp)
	req.Header.Set("X-Stats-Signature", hex.EncodeToString(mac.Sum(nil)))
	response, err := c.http.Do(req)
	if err == nil {
		_ = response.Body.Close()
	}
}
