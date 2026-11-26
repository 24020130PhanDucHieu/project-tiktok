package middleware

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

const (
	guestReadOnly = "Chế độ Khách chỉ được xem. Vui lòng đăng nhập để chỉnh sửa."
	guestNoAccess = "Chế độ Khách không có quyền xem mục này. Vui lòng đăng nhập."
)

// guestBlockedPrefixes are the areas a guest may not even read.
var guestBlockedPrefixes = []string{"/admin", "/inbox", "/gmail", "/zalo", "/ttshop", "/internal"}

// guestDenial returns the message to refuse a guest request with, or "" when it is allowed.
func guestDenial(method, path string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		return guestReadOnly
	}
	p := strings.TrimPrefix(path, "/api/v1")
	for _, pre := range guestBlockedPrefixes {
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return guestNoAccess
		}
	}
	return ""
}

// guestGate answers 403 and returns true when the caller is a guest and the request is not allowed.
func guestGate(c *fiber.Ctx) (bool, error) {
	u := CurrentUser(c)
	if u == nil || u.Role != domain.RoleGuest {
		return false, nil
	}
	if msg := guestDenial(c.Method(), c.Path()); msg != "" {
		return true, httpx.Fail(c, fiber.StatusForbidden, msg)
	}
	return false, nil
}

// BlockGuestWrites keeps the shared guest account read-only (use after RequireAuth): a guest may only GET/HEAD/OPTIONS,
// and not even read the admin, inbox, gmail, zalo, ttshop or internal areas.
func BlockGuestWrites() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if done, err := guestGate(c); done {
			return err
		}
		return c.Next()
	}
}

// RequireAuthNoGuestWrites is RequireAuth followed by BlockGuestWrites as one handler, so every route group that takes
// it is protected from guest writes.
func RequireAuthNoGuestWrites(auth *service.AuthService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if ok, err := authenticate(c, auth); !ok {
			return err
		}
		if done, err := guestGate(c); done {
			return err
		}
		return c.Next()
	}
}
