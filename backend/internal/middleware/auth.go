// Package middleware contains Fiber middlewares.
package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// UserKey is the Fiber locals key holding the authenticated *service.Claims.
const UserKey = "user"

// RequireAuth rejects requests without a valid Bearer JWT.
func RequireAuth(auth *service.AuthService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return httpx.Fail(c, fiber.StatusUnauthorized, "Bạn cần đăng nhập")
		}
		claims, err := auth.ParseToken(header[len("Bearer "):])
		if err != nil {
			return httpx.Fail(c, fiber.StatusUnauthorized, "Phiên đăng nhập không còn hợp lệ")
		}
		c.Locals(UserKey, claims)
		return c.Next()
	}
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
