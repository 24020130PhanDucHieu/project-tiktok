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

	"github.com/tiktoday/kocflow-api/internal/service"
)

const (
	googleTokenURL = "https://oauth2.googleapis.com/token"
	gmailSendURL   = "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"
)

// GoogleClient implements service.GoogleAPI over HTTP.
type GoogleClient struct{ http *http.Client }

// NewGoogleClient builds a GoogleClient.
func NewGoogleClient() *GoogleClient {
	return &GoogleClient{http: &http.Client{Timeout: 15 * time.Second}}
}

// Token posts a form to Google's token endpoint.
func (c *GoogleClient) Token(ctx context.Context, form url.Values) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		if out["error"] == "invalid_grant" {
			return nil, service.ErrGmailBroken
		}
		return nil, fmt.Errorf("Google từ chối yêu cầu (%d)", resp.StatusCode)
	}
	return out, nil
}

// GetJSON reads a Google API url with a bearer token.
func (c *GoogleClient) GetJSON(ctx context.Context, access, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Gmail trả lỗi %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// SendRaw sends a base64url RFC 5322 message through Gmail and returns the message id.
func (c *GoogleClient) SendRaw(ctx context.Context, access, raw string) (string, error) {
	body, _ := json.Marshal(map[string]string{"raw": raw})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gmailSendURL, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gmail không gửi được thư (%d)", resp.StatusCode)
	}
	return out.ID, nil
}
