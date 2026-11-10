package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// TurnstileClient checks Cloudflare Turnstile tokens (service.CaptchaVerifier).
type TurnstileClient struct {
	secret    string
	http      *http.Client
	verifyURL string // a field so that tests can point it at a fake
}

// NewTurnstileClient builds a TurnstileClient for the given secret key.
func NewTurnstileClient(secret string) *TurnstileClient {
	return &TurnstileClient{secret: secret, http: &http.Client{Timeout: 8 * time.Second}, verifyURL: turnstileVerifyURL}
}

// Verify asks Cloudflare whether the token was issued to a human for this site. A rejected or empty token is
// (false, nil); an unreachable or failing Cloudflare is an error, so the caller does not mistake an outage for a bot.
func (c *TurnstileClient) Verify(ctx context.Context, token, ip string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, nil
	}
	form := url.Values{"secret": {c.secret}, "response": {token}}
	if ip != "" {
		form.Set("remoteip", ip)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("turnstile: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, fmt.Errorf("turnstile: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return false, fmt.Errorf("turnstile: unreadable answer: %w", err)
	}
	return out.Success, nil
}
