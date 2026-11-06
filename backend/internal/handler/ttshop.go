package handler

import (
	"context"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// TTShopHandler serves the admin routes that connect the app to the TikTok Shop Open Platform.
type TTShopHandler struct {
	tts   TTShopAPI
	audit Auditor
}

// TTShopAPI is the part of *service.TTShopService this handler uses.
type TTShopAPI interface {
	Status(ctx context.Context) service.TTShopStatus
	RecentLogs(n int) []string
	OpenCollaborations(ctx context.Context, keyword, pageToken string, pageSize int) (*service.OpenCollabPage, error)
	OpenCollabSettings(ctx context.Context) (*service.OpenCollabSettings, error)
	Authorize(ctx context.Context, authCode string) error
}

// NewTTShopHandler builds a TTShopHandler.
func NewTTShopHandler(tts TTShopAPI, audit Auditor) *TTShopHandler {
	return &TTShopHandler{tts: tts, audit: audit}
}

// Status handles GET /ttshop/status (read-only: configuration, grant state and authorized shops).
func (h *TTShopHandler) Status(c *fiber.Ctx) error {
	return httpx.OK(c, h.tts.Status(c.UserContext()))
}

// Authorize handles POST /ttshop/authorize with the auth_code TikTok sent to the redirect URL.
func (h *TTShopHandler) Authorize(c *fiber.Ctx) error {
	var in struct {
		AuthCode string `json:"authCode"`
	}
	if err := c.BodyParser(&in); err != nil || in.AuthCode == "" {
		return httpx.Fail(c, fiber.StatusBadRequest, "Thiếu authCode")
	}
	a := middleware.ActorOf(c)
	h.audit.Log(nil, a.ID, a.Email, "ttshop.authorize", "", "", "", a.IP)
	if err := h.tts.Authorize(c.UserContext(), in.AuthCode); err != nil {
		return httpx.Fail(c, fiber.StatusBadGateway, err.Error())
	}
	return httpx.OK(c, h.tts.Status(c.UserContext()))
}

// ttFail maps a TikTok Shop service error to an HTTP failure.
func ttFail(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrTTShopNotConnected):
		return httpx.Fail(c, fiber.StatusConflict, err.Error())
	case errors.Is(err, service.ErrOpenCollabNotFound):
		return httpx.Fail(c, fiber.StatusNotFound, err.Error())
	case service.IsRateLimited(err):
		return httpx.Fail(c, fiber.StatusTooManyRequests, "TikTok đang giới hạn tốc độ. Hãy đợi khoảng một phút rồi thử lại.")
	default:
		return httpx.Fail(c, fiber.StatusBadGateway, err.Error())
	}
}

// OpenCollabs handles GET /shop/open-collaborations?keyword=&pageToken=&pageSize= (read-only).
func (h *TTShopHandler) OpenCollabs(c *fiber.Ctx) error {
	size, _ := strconv.Atoi(c.Query("pageSize", "20"))
	page, err := h.tts.OpenCollaborations(c.UserContext(), c.Query("keyword"), c.Query("pageToken"), size)
	if err != nil {
		return ttFail(c, err)
	}
	return httpx.OK(c, page)
}

// OpenCollabSettings handles GET /shop/open-collaboration-settings (read-only).
func (h *TTShopHandler) OpenCollabSettings(c *fiber.Ctx) error {
	st, err := h.tts.OpenCollabSettings(c.UserContext())
	if err != nil {
		return ttFail(c, err)
	}
	return httpx.OK(c, st)
}

// Logs handles GET /ttshop/logs?lines=100: the newest logged TikTok Shop calls, to check the integration.
func (h *TTShopHandler) Logs(c *fiber.Ctx) error {
	n, _ := strconv.Atoi(c.Query("lines", "100"))
	return httpx.OK(c, fiber.Map{"lines": h.tts.RecentLogs(n)})
}
