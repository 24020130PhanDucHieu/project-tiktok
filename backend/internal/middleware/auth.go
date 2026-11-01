// Package middleware contains Fiber middlewares.
package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// UserKey is the Fiber locals key holding the authenticated *domain.User.
const UserKey = "user"

// RequireAuth rejects requests without a valid Bearer JWT of an active account.
func RequireAuth(auth *service.AuthService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return httpx.Fail(c, fiber.StatusUnauthorized, "Bạn cần đăng nhập")
		}
		user, err := auth.Authenticate(c.UserContext(), header[len("Bearer "):])
		if err != nil {
			return httpx.Fail(c, fiber.StatusUnauthorized, "Phiên đăng nhập không còn hợp lệ")
		}
		c.Locals(UserKey, user)
		return c.Next()
	}
}

// RequireRole allows only the listed roles (use after RequireAuth).
func RequireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := CurrentUser(c)
		if u == nil {
			return httpx.Fail(c, fiber.StatusUnauthorized, "Bạn cần đăng nhập")
		}
		for _, r := range roles {
			if u.Role == r {
				return c.Next()
			}
		}
		return httpx.Fail(c, fiber.StatusForbidden, "Bạn không có quyền thực hiện thao tác này")
	}
}

// CurrentUser returns the authenticated user, or nil.
func CurrentUser(c *fiber.Ctx) *domain.User {
	u, _ := c.Locals(UserKey).(*domain.User)
	return u
}

// ActorOf describes the caller for audit logging.
func ActorOf(c *fiber.Ctx) service.Actor {
	a := service.Actor{IP: c.IP()}
	if u := CurrentUser(c); u != nil {
		a.ID, a.Email = u.ID.Hex(), u.Email
	}
	return a
}

// RequireBridgeSecret protects internal endpoints called by the zalo-bridge sidecar.
func RequireBridgeSecret(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if subtle.ConstantTimeCompare([]byte(c.Get("X-Bridge-Secret")), []byte(secret)) != 1 {
			return httpx.Fail(c, fiber.StatusUnauthorized, "Invalid bridge secret")
		}
		return c.Next()
	}
}
