package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
)

const zaloOASendURL = "https://openapi.zalo.me/v3.0/oa/message/cs"

// ZaloStatus describes the personal Zalo session.
type ZaloStatus struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	State      string `json:"state"`
	QRImage    string `json:"qrImage"`
	Error      string `json:"error"`
}

// ZaloCredentials are the personal-account credentials exported from Zalo Web.
type ZaloCredentials struct {
	Cookie    json.RawMessage `json:"cookie"`
	IMEI      string          `json:"imei"`
	UserAgent string          `json:"userAgent"`
}

// ZaloUser is a resolved Zalo account.
type ZaloUser struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
}

// PersonalZalo is the gateway to a personal Zalo account.
//
// Zalo personal messaging has no official Go SDK; the production implementation
// (BridgeClient) talks to the zalo-bridge sidecar that wraps the zca-js library.
type PersonalZalo interface {
	Status(ctx context.Context) ZaloStatus
	Connect(ctx context.Context, creds ZaloCredentials) (ZaloStatus, error)
	StartQR(ctx context.Context) ZaloStatus
	Disconnect(ctx context.Context) ZaloStatus
	IsConnected(ctx context.Context) bool
	ResolveByPhone(ctx context.Context, phone string) (*ZaloUser, error)
	Send(ctx context.Context, userID, text string) error
}

// BridgeClient implements PersonalZalo over the sidecar's HTTP API.
type BridgeClient struct {
	cfg  config.Config
	http *http.Client
}

// NewBridgeClient builds a BridgeClient.
func NewBridgeClient(cfg config.Config) *BridgeClient {
	return &BridgeClient{cfg: cfg, http: &http.Client{Timeout: 45 * time.Second}}
}

// EnvCredentials returns credentials from the environment, or nil when incomplete.
func (b *BridgeClient) EnvCredentials() *ZaloCredentials {
	if b.cfg.ZaloPersonalCookie == "" || b.cfg.ZaloPersonalIMEI == "" || b.cfg.ZaloPersonalUserAgent == "" {
		return nil
	}
	return &ZaloCredentials{Cookie: json.RawMessage(b.cfg.ZaloPersonalCookie), IMEI: b.cfg.ZaloPersonalIMEI, UserAgent: b.cfg.ZaloPersonalUserAgent}
}

type bridgeState struct {
	State   string `json:"state"`
	QRImage string `json:"qrImage"`
	Error   string `json:"error"`
}

func (b *BridgeClient) status(s bridgeState) ZaloStatus {
	return ZaloStatus{
		Configured: b.EnvCredentials() != nil,
		Connected:  s.State == "connected",
		State:      s.State, QRImage: s.QRImage, Error: s.Error,
	}
}

func (b *BridgeClient) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(b.cfg.ZaloBridgeURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bridge-Secret", b.cfg.ZaloBridgeSecret)
	resp, err := b.http.Do(req)
	if err != nil {
		return fmt.Errorf("zalo-bridge không phản hồi: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = fmt.Sprintf("zalo-bridge HTTP %d", resp.StatusCode)
		}
		return errors.New(e.Message)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (b *BridgeClient) fetchState(ctx context.Context, method, path string, in any) (bridgeState, error) {
	var s bridgeState
	err := b.call(ctx, method, path, in, &s)
	return s, err
}

// Status returns the current session state.
func (b *BridgeClient) Status(ctx context.Context) ZaloStatus {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	s, err := b.fetchState(ctx, http.MethodGet, "/status", nil)
	if err != nil {
		return b.status(bridgeState{State: "disconnected", Error: err.Error()})
	}
	return b.status(s)
}

// Connect logs in with cookie/IMEI/user-agent credentials.
func (b *BridgeClient) Connect(ctx context.Context, creds ZaloCredentials) (ZaloStatus, error) {
	s, err := b.fetchState(ctx, http.MethodPost, "/connect", creds)
	if err != nil {
		return b.status(bridgeState{State: "error", Error: err.Error()}), err
	}
	return b.status(s), nil
}

// StartQR begins a QR-code login (poll Status for the image and progress).
func (b *BridgeClient) StartQR(ctx context.Context) ZaloStatus {
	s, err := b.fetchState(ctx, http.MethodPost, "/qr/start", map[string]string{"userAgent": b.cfg.ZaloPersonalUserAgent})
	if err != nil {
		return b.status(bridgeState{State: "error", Error: err.Error()})
	}
	return b.status(s)
}

// Disconnect closes the session.
func (b *BridgeClient) Disconnect(ctx context.Context) ZaloStatus {
	s, err := b.fetchState(ctx, http.MethodPost, "/disconnect", nil)
	if err != nil {
		return b.status(bridgeState{State: "disconnected", Error: err.Error()})
	}
	return b.status(s)
}

// IsConnected reports whether the personal account is logged in.
func (b *BridgeClient) IsConnected(ctx context.Context) bool { return b.Status(ctx).Connected }

var phoneStripRe = regexp.MustCompile(`[\s()-]`)

// ResolveByPhone finds the Zalo account registered with a phone number.
func (b *BridgeClient) ResolveByPhone(ctx context.Context, phone string) (*ZaloUser, error) {
	if !b.IsConnected(ctx) {
		return nil, errors.New("Zalo cá nhân chưa kết nối.")
	}
	normalized := strings.TrimPrefix(phoneStripRe.ReplaceAllString(phone, ""), "+")
	if normalized == "" {
		return nil, errors.New("Thiếu số điện thoại Zalo.")
	}
	var user ZaloUser
	if err := b.call(ctx, http.MethodPost, "/find-user", map[string]string{"phone": normalized}, &user); err != nil {
		return nil, err
	}
	if user.UserID == "" {
		return nil, errors.New("Không tìm thấy tài khoản Zalo với số điện thoại này.")
	}
	return &user, nil
}

// Send delivers a text message to a Zalo user.
func (b *BridgeClient) Send(ctx context.Context, userID, text string) error {
	if !b.IsConnected(ctx) {
		return errors.New("Zalo cá nhân chưa kết nối.")
	}
	return b.call(ctx, http.MethodPost, "/send", map[string]string{"userId": userID, "text": text}, nil)
}

// InitializeFromEnv connects the personal account at startup when credentials are configured.
func (b *BridgeClient) InitializeFromEnv(ctx context.Context) {
	creds := b.EnvCredentials()
	if creds == nil {
		return
	}
	if _, err := b.Connect(ctx, *creds); err != nil {
		log.Printf("Unable to connect personal Zalo: %v", err)
	}
}

// ZaloDelivery is the outcome of SendText.
type ZaloDelivery struct {
	Delivered bool
	Provider  string
	Reason    string
}

// ZaloService sends messages through the personal account or the official OA API.
type ZaloService struct {
	cfg      config.Config
	personal PersonalZalo
	http     *http.Client
}

// NewZaloService builds a ZaloService.
func NewZaloService(cfg config.Config, personal PersonalZalo) *ZaloService {
	return &ZaloService{cfg: cfg, personal: personal, http: &http.Client{Timeout: 20 * time.Second}}
}

// Personal exposes the personal-account gateway.
func (z *ZaloService) Personal() PersonalZalo { return z.personal }

// IsConfigured reports whether any delivery path is available.
func (z *ZaloService) IsConfigured(ctx context.Context) bool {
	return z.personal.IsConnected(ctx) || z.cfg.ZaloOAAccessToken != ""
}

// SendText delivers a message to a Zalo user id.
func (z *ZaloService) SendText(ctx context.Context, userID, text string) (ZaloDelivery, error) {
	if z.personal.IsConnected(ctx) {
		if err := z.personal.Send(ctx, userID, text); err != nil {
			return ZaloDelivery{}, err
		}
		return ZaloDelivery{Delivered: true, Provider: "personal"}, nil
	}
	if z.cfg.ZaloOAAccessToken == "" {
		return ZaloDelivery{Delivered: false, Reason: "missing_token"}, nil
	}
	payload, _ := json.Marshal(map[string]any{
		"recipient": map[string]string{"user_id": userID},
		"message":   map[string]string{"text": text},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zaloOASendURL, bytes.NewReader(payload))
	if err != nil {
		return ZaloDelivery{}, err
	}
	req.Header.Set("access_token", z.cfg.ZaloOAAccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := z.http.Do(req)
	if err != nil {
		return ZaloDelivery{}, err
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
		return ZaloDelivery{}, errors.New(result.Message)
	}
	return ZaloDelivery{Delivered: true, Provider: "oa"}, nil
}
