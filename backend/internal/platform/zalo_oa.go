package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const zaloOASendURL = "https://openapi.zalo.me/v3.0/oa/message/cs"

// ZaloOAClient implements service.ZaloOA over HTTP.
type ZaloOAClient struct{ http *http.Client }

// NewZaloOAClient builds a ZaloOAClient.
func NewZaloOAClient() *ZaloOAClient {
	return &ZaloOAClient{http: &http.Client{Timeout: 20 * time.Second}}
}

// SendText sends a customer-service message from the official account.
func (c *ZaloOAClient) SendText(ctx context.Context, accessToken, userID, text string) error {
	payload, _ := json.Marshal(map[string]any{
		"recipient": map[string]string{"user_id": userID},
		"message":   map[string]string{"text": text},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zaloOASendURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("access_token", accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode >= 300 || result.Error != 0 {
		if result.Message == "" {
			result.Message = fmt.Sprintf("Zalo API returned HTTP %d", resp.StatusCode)
		}
		return errors.New(result.Message)
	}
	return nil
}
