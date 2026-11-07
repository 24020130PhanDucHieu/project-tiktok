package service

import (
	"context"
	"encoding/json"

	"github.com/tiktoday/kocflow-api/internal/config"
)

// ZaloStatus describes the personal Zalo session.
type ZaloStatus struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	State      string `json:"state"`
	QRImage    string `json:"qrImage"`
	Error      string `json:"error"`
	// BridgeOnline is false when the zalo-bridge sidecar cannot be reached at all.
	BridgeOnline bool `json:"bridgeOnline"`
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
// (platform.ZaloBridge) talks to the zalo-bridge sidecar that wraps the zca-js library.
type PersonalZalo interface {
	Status(ctx context.Context) ZaloStatus
	Connect(ctx context.Context, creds ZaloCredentials) (ZaloStatus, error)
	StartQR(ctx context.Context) ZaloStatus
	Disconnect(ctx context.Context) ZaloStatus
	IsConnected(ctx context.Context) bool
	ResolveByPhone(ctx context.Context, phone string) (*ZaloUser, error)
	Send(ctx context.Context, userID, text string) error
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
	oa       ZaloOA
}

// ZaloOA sends a text message through Zalo's official-account API.
type ZaloOA interface {
	SendText(ctx context.Context, accessToken, userID, text string) error
}

// NewZaloService builds a ZaloService.
func NewZaloService(cfg config.Config, personal PersonalZalo, oa ZaloOA) *ZaloService {
	return &ZaloService{cfg: cfg, personal: personal, oa: oa}
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
	if err := z.oa.SendText(ctx, z.cfg.ZaloOAAccessToken, userID, text); err != nil {
		return ZaloDelivery{}, err
	}
	return ZaloDelivery{Delivered: true, Provider: "oa"}, nil
}
