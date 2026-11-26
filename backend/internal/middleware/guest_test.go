package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/service"
)

func guestApp(role string) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(UserKey, &domain.User{ID: domain.NewID(), Role: role})
		return c.Next()
	})
	app.Use(BlockGuestWrites())
	app.All("/*", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	return app
}

func status(t *testing.T, app *fiber.App, method, path string) int {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(method, path, nil))
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

func TestGuestMayOnlyReadAllowedAreas(t *testing.T) {
	app := guestApp(domain.RoleGuest)
	for _, p := range []string{"/api/v1/kocs", "/api/v1/auth/me", "/api/v1/campaigns/"} {
		if got := status(t, app, http.MethodGet, p); got != 200 {
			t.Errorf("guest GET %s = %d, want 200", p, got)
		}
	}
	if got := status(t, app, http.MethodHead, "/api/v1/kocs"); got != 200 {
		t.Errorf("guest HEAD = %d", got)
	}
	for _, m := range []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete} {
		for _, p := range []string{"/api/v1/kocs", "/api/v1/auth/me", "/api/v1/auth/change-password"} {
			if got := status(t, app, m, p); got != 403 {
				t.Errorf("guest %s %s = %d, want 403", m, p, got)
			}
		}
	}
}

func TestGuestCannotReadBlockedPrefixes(t *testing.T) {
	app := guestApp(domain.RoleGuest)
	for _, p := range []string{"/api/v1/admin/users", "/api/v1/inbox/threads", "/api/v1/gmail/x", "/api/v1/zalo/status", "/api/v1/ttshop/status", "/internal/zalo/inbound", "/api/v1/admin"} {
		if got := status(t, app, http.MethodGet, p); got != 403 {
			t.Errorf("guest GET %s = %d, want 403", p, got)
		}
	}
}

func TestMemberIsNotAffectedByGuestRules(t *testing.T) {
	app := guestApp(domain.RoleMember)
	for _, c := range [][2]string{{"POST", "/api/v1/kocs"}, {"PATCH", "/api/v1/auth/me"}, {"DELETE", "/api/v1/kocs/1"}, {"GET", "/api/v1/inbox/x"}} {
		if got := status(t, app, c[0], c[1]); got != 200 {
			t.Errorf("member %s %s = %d, want 200", c[0], c[1], got)
		}
	}
}

func TestRequireAuthNoGuestWritesBlocksGuestToken(t *testing.T) {
	auth := service.NewAuthService(nil, nil, nil, nil, config.Config{JWTSecret: "s", JWTExpiresIn: time.Hour})
	guest := &domain.User{ID: domain.NewID(), Role: domain.RoleGuest, Status: domain.UserActive}
	auth.SeedUser(guest)
	tok, err := auth.IssueTokenForTest(guest)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use(RequireAuthNoGuestWrites(auth))
	app.All("/*", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	do := func(method string) int {
		req := httptest.NewRequest(method, "/api/v1/kocs", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode
	}
	if do(http.MethodGet) != 200 || do(http.MethodPost) != 403 {
		t.Fatal("guest token: GET must pass, POST must be 403")
	}
	if got := status(t, app, http.MethodGet, "/api/v1/kocs"); got != 401 {
		t.Fatalf("no token = %d, want 401", got)
	}
}
