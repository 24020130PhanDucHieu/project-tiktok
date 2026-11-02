package handler

import (
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// GmailHandler serves /gmail routes (per-user OAuth link to a Gmail mailbox).
type GmailHandler struct {
	gmail       *service.GmailService
	frontendURL string
}

// NewGmailHandler builds a GmailHandler.
func NewGmailHandler(gmail *service.GmailService, frontendURL string) *GmailHandler {
	return &GmailHandler{gmail: gmail, frontendURL: frontendURL}
}

// Status handles GET /gmail/status.
func (h *GmailHandler) Status(c *fiber.Ctx) error {
	out := fiber.Map{"configured": h.gmail.Configured(), "connected": false, "broken": false, "email": ""}
	if u := middleware.CurrentUser(c); u != nil {
		acct, err := h.gmail.Account(c.UserContext(), u.ID.Hex())
		if err != nil {
			return err
		}
		if acct != nil {
			out["connected"], out["broken"], out["email"] = !acct.Broken, acct.Broken, acct.Email
		}
	}
	return httpx.OK(c, out)
}

// Connect handles GET /gmail/connect and returns the Google consent URL.
func (h *GmailHandler) Connect(c *fiber.Ctx) error {
	if !h.gmail.Configured() {
		return httpx.NewError(fiber.StatusUnprocessableEntity, "Máy chủ chưa cấu hình GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET")
	}
	u := middleware.CurrentUser(c)
	if u == nil {
		return httpx.NewError(fiber.StatusUnauthorized, "Chưa đăng nhập")
	}
	return httpx.OK(c, fiber.Map{"url": h.gmail.AuthURL(u.ID.Hex())})
}

// Callback handles GET /gmail/callback (unauthenticated browser redirect from Google).
func (h *GmailHandler) Callback(c *fiber.Ctx) error {
	back := func(result string) error {
		return c.Redirect(h.frontendURL + "/?gmail=" + url.QueryEscape(result))
	}
	userID, ok := h.gmail.VerifyState(c.Query("state"))
	if !ok || c.Query("code") == "" || c.Query("error") != "" {
		return back("error")
	}
	if _, err := h.gmail.Connect(c.UserContext(), userID, c.Query("code")); err != nil {
		return back("error")
	}
	return back("connected")
}

// Disconnect handles DELETE /gmail.
func (h *GmailHandler) Disconnect(c *fiber.Ctx) error {
	if u := middleware.CurrentUser(c); u != nil {
		if err := h.gmail.Disconnect(c.UserContext(), u.ID.Hex()); err != nil {
			return err
		}
	}
	return httpx.OKMessage(c, nil, "Đã ngắt kết nối Gmail.")
}
