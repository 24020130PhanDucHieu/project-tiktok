package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/metrics"
	"github.com/tiktoday/kocflow-api/internal/platform"
	"github.com/tiktoday/kocflow-api/internal/repository"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// newApp wires the real app on a lazy (never connected) Mongo client, so only DB-free paths are exercised.
// It returns an admin and a member token; both accounts are pre-seeded in the auth cache.
func newApp(t *testing.T) (app *fiber.App, adminToken, memberToken string) {
	t.Helper()
	return newAppCfg(t, nil)
}

// newAppCfg is newApp with a chance to change the configuration first (nil keeps the defaults of the tests).
func newAppCfg(t *testing.T, tweak func(*config.Config)) (app *fiber.App, adminToken, memberToken string) {
	t.Helper()
	cfg := config.Config{
		JWTSecret: "test-secret", JWTExpiresIn: time.Hour, FrontendURL: "http://localhost:5173",
		ZaloBridgeURL: "http://127.0.0.1:1", ZaloBridgeSecret: "bridge-secret",
	}
	if tweak != nil {
		tweak(&cfg)
	}
	client, err := mongo.Connect(nil, options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database("test")
	kocRepo, convRepo, msgRepo := repository.NewKocRepository(db), repository.NewConversationRepository(db), repository.NewMessageRepository(db)
	mailer := service.NewSMTPMailer(cfg)
	audit := service.NewAuditService(nil)
	authSvc := service.NewAuthService(repository.NewUserRepository(db), repository.NewResetRepository(db), audit, mailer, cfg)
	bridge := platform.NewZaloBridge(cfg)
	outreach := service.NewOutreachService(kocRepo, convRepo, msgRepo, mailer,
		platform.NewJTClient(cfg), platform.NewTikTokWeb(cfg), service.NewZaloService(cfg, bridge, platform.NewZaloOAClient()))
	statsSvc := service.NewStatsService(kocRepo)
	gmailSvc := service.NewGmailService(cfg, repository.NewGmailAccountRepository(db), platform.NewGoogleClient())
	collector := metrics.New()
	adminSvc := service.NewAdminService(repository.NewUserRepository(db), authSvc, audit, repository.NewDatabaseInspector(db), bridge, mailer, collector, metrics.NewJobs(), "test")
	app = router.New(cfg, authSvc, router.Handlers{
		Auth:   handler.NewAuthHandler(authSvc, audit),
		Koc:    handler.NewKocHandler(service.NewKocService(kocRepo, convRepo, outreach), outreach, statsSvc, audit),
		Inbox:  handler.NewInboxHandler(kocRepo, convRepo, msgRepo, outreach, gmailSvc),
		Gmail:  handler.NewGmailHandler(gmailSvc, cfg.FrontendURL),
		Collab: handler.NewCollabHandler(kocRepo, repository.NewCollabRepository(db), convRepo, msgRepo),
		Zalo:   handler.NewZaloHandler(bridge, convRepo, msgRepo, outreach, audit),
		Admin:  handler.NewAdminHandler(adminSvc, audit, statsSvc),
		Target: handler.NewTargetCollabHandler(noTTS{}, noStore{}, audit, 10),
	}, collector)
	mk := func(email, role string) string {
		u := &domain.User{ID: domain.NewID(), Name: "T", Email: email, Role: role, Status: domain.UserActive}
		authSvc.SeedUser(u)
		tok, err := authSvc.IssueTokenForTest(u)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	return app, mk("admin@t.t", domain.RoleAdmin), mk("member@t.t", domain.RoleMember)
}

func do(t *testing.T, app *fiber.App, method, path, token, body string, extra ...[2]string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, h := range extra {
		req.Header.Set(h[0], h[1])
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestHealthAndNotFound(t *testing.T) {
	app, _, _ := newApp(t)
	if code, body := do(t, app, "GET", "/health", "", ""); code != 200 || body["service"] != "kocflow-api" {
		t.Fatalf("health: %d %v", code, body)
	}
	code, body := do(t, app, "GET", "/api/v1/nope", "", "")
	if code != 404 || body["success"] != false || !strings.HasPrefix(body["message"].(string), "Route not found: GET /api/v1/nope") {
		t.Fatalf("404 shape: %d %v", code, body)
	}
}

func TestAuthGuardsAndValidation(t *testing.T) {
	app, token, _ := newApp(t)
	for _, p := range []string{"/api/v1/kocs", "/api/v1/inbox/conversations", "/api/v1/zalo/status", "/api/v1/auth/me"} {
		if code, body := do(t, app, "GET", p, "", ""); code != 401 || body["message"] != "Bạn cần đăng nhập" {
			t.Errorf("%s without token: %d %v", p, code, body)
		}
		if code, body := do(t, app, "GET", p, "garbage", ""); code != 401 {
			t.Errorf("%s bad header: %d %v", p, code, body)
		}
	}
	if code, body := do(t, app, "GET", "/api/v1/kocs", "bad.token.value", ""); code != 401 || body["message"] != "Phiên đăng nhập không còn hợp lệ" {
		t.Errorf("invalid jwt: %d %v", code, body)
	}

	code, body := do(t, app, "GET", "/api/v1/auth/me", token, "")
	if code != 200 || body["data"].(map[string]any)["email"] != "admin@t.t" {
		t.Fatalf("me: %d %v", code, body)
	}

	// Zod-equivalent validation -> 400 "Dữ liệu không hợp lệ".
	for _, c := range []struct{ path, body string }{
		{"/api/v1/auth/forgot-password", `{"email":"bad"}`},
		{"/api/v1/auth/reset-password", `{"email":"a@b.vn","code":"12ab","password":"short"}`},
		{"/api/v1/auth/login", `{}`},
	} {
		if code, body := do(t, app, "POST", c.path, "", c.body); code != 400 || body["message"] != "Dữ liệu không hợp lệ" {
			t.Errorf("%s: %d %v", c.path, code, body)
		}
	}
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/v1/kocs", `{"tiktokId":"x"}`},
		{"POST", "/api/v1/kocs/bulk", `{"items":[]}`},
		{"POST", "/api/v1/kocs/outreach/run", `{"mode":"nope"}`},
		{"PATCH", "/api/v1/kocs/abc/stage", `{"action":"explode"}`},
		{"POST", "/api/v1/inbox/conversations/abc/messages", `{"text":""}`},
	} {
		if code, body := do(t, app, c.method, c.path, token, c.body); code != 400 || body["message"] != "Dữ liệu không hợp lệ" {
			t.Errorf("%s %s: %d %v", c.method, c.path, code, body)
		}
	}
}

func TestDBFreeEndpoints(t *testing.T) {
	app, token, _ := newApp(t)

	// Chatbot intent preview (also used by the frontend for unsaved conversations).
	code, body := do(t, app, "POST", "/api/v1/inbox/chatbot/parse", token,
		`{"text":"Ok shop, mình đồng ý nhận mẫu nhé","koc":{"name":"Linh"}}`)
	data, _ := body["data"].(map[string]any)
	if code != 200 || data["parsed"].(map[string]any)["intent"] != "AGREE_NO_ADDRESS" || !strings.Contains(data["botReply"].(string), "Linh") {
		t.Fatalf("parse: %d %v", code, body)
	}
	// DNC KOC: chatbot stays silent.
	_, body = do(t, app, "POST", "/api/v1/inbox/chatbot/parse", token, `{"text":"ok shop đồng ý","koc":{"dnc":true}}`)
	if body["data"].(map[string]any)["botReply"] != "" {
		t.Fatalf("dnc must be silent: %v", body)
	}

	// Zalo status degrades gracefully when the bridge is down.
	code, body = do(t, app, "GET", "/api/v1/zalo/status", token, "")
	st := body["data"].(map[string]any)
	if code != 200 || st["connected"] != false || st["state"] != "disconnected" {
		t.Fatalf("zalo status: %d %v", code, body)
	}

	// Zalo OA webhook (served only with a secret, see webhook_test.go): verification + ignored events + malformed payload.
	hookSecret := [2]string{"X-Webhook-Secret", "hook-secret-hook-secret-hook-secret"}
	hookApp, _, _ := newAppCfg(t, func(c *config.Config) { c.ZaloWebhookSecret = hookSecret[1] })
	if code, body := do(t, hookApp, "GET", "/api/v1/webhooks/zalo", "", "", hookSecret); code != 200 || body["service"] != "zalo-webhook" {
		t.Fatalf("webhook verify: %d %v", code, body)
	}
	if code, body := do(t, hookApp, "POST", "/api/v1/webhooks/zalo", "", `{"event_name":"follow"}`, hookSecret); code != 200 || body["ignored"] != true {
		t.Fatalf("webhook ignore: %d %v", code, body)
	}
	if code, _ := do(t, hookApp, "POST", "/api/v1/webhooks/zalo", "", `{"event_name":"user_send_text"}`, hookSecret); code != 400 {
		t.Fatalf("webhook missing fields: %d", code)
	}

	// Input checks that fail before touching the database.
	if code, body := do(t, app, "POST", "/api/v1/kocs/track-shipment/lookup", token, `{}`); code != 400 || body["message"] != "Vui lòng nhập mã vận đơn J&T Express" {
		t.Fatalf("jt lookup: %d %v", code, body)
	}
	// New inbox / collaboration / gmail routes are auth-protected, except the Google redirect.
	for _, route := range [][2]string{{"GET", "/api/v1/inbox/labels"}, {"GET", "/api/v1/inbox/templates"}, {"GET", "/api/v1/gmail/status"}, {"GET", "/api/v1/kocs/abc/collabs"}, {"POST", "/api/v1/collabs/abc/close"}, {"GET", "/api/v1/kocs/abc/contacts"}} {
		if code, _ := do(t, app, route[0], route[1], "", ""); code != 401 {
			t.Fatalf("%s %s without token: %d", route[0], route[1], code)
		}
	}
	if code, _ := do(t, app, "GET", "/api/v1/gmail/callback?state=bad", "", ""); code != 302 {
		t.Fatalf("gmail callback must redirect, got %d", code)
	}
	for _, payload := range []string{`{"trackingCode":"841000098765"}`, `{"trackingCode":"841000098765","phoneLast4":"12"}`, `{"trackingCode":"841000098765","phoneLast4":"12ab"}`} {
		if code, body := do(t, app, "POST", "/api/v1/kocs/track-shipment/lookup", token, payload); code != 400 || body["message"] != "Cần đúng 4 số cuối SĐT người nhận để tra cứu J&T" {
			t.Fatalf("jt lookup without valid phoneLast4 %s: %d %v", payload, code, body)
		}
	}
	if code, body := do(t, app, "POST", "/api/v1/kocs/video/lookup", token, `{}`); code != 400 || body["message"] != "Vui lòng nhập đường dẫn video TikTok" {
		t.Fatalf("video lookup: %d %v", code, body)
	}
}

func TestInternalBridgeCallbackIsProtected(t *testing.T) {
	app, _, _ := newApp(t)
	if code, _ := do(t, app, "POST", "/internal/zalo/inbound", "", `{"externalContactId":"1","text":"hi"}`); code != http.StatusUnauthorized {
		t.Fatalf("missing secret: %d", code)
	}
	if code, _ := do(t, app, "POST", "/internal/zalo/inbound", "", `{}`, [2]string{"X-Bridge-Secret", "bridge-secret"}); code != http.StatusBadRequest {
		t.Fatalf("valid secret, empty body: %d", code)
	}
}

func TestRoleBasedAccess(t *testing.T) {
	app, admin, member := newApp(t)

	// members are blocked from every admin route
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/admin/users"}, {"POST", "/api/v1/admin/users"}, {"GET", "/api/v1/admin/audit"},
		{"GET", "/api/v1/admin/system"}, {"DELETE", "/api/v1/admin/users/abc"},
	} {
		if code, body := do(t, app, c.method, c.path, member, `{}`); code != 403 || body["success"] != false {
			t.Errorf("member %s %s: %d %v", c.method, c.path, code, body)
		}
		if code, _ := do(t, app, c.method, c.path, "", `{}`); code != 401 {
			t.Errorf("anonymous %s %s: %d", c.method, c.path, code)
		}
	}
	// admin passes the guard and reaches validation
	if code, body := do(t, app, "POST", "/api/v1/admin/users", admin, `{"name":"x","email":"bad","role":"root"}`); code != 400 || body["message"] != "Dữ liệu không hợp lệ" {
		t.Errorf("admin create validation: %d %v", code, body)
	}
	if code, _ := do(t, app, "PATCH", "/api/v1/admin/users/abc", admin, `{"role":"root"}`); code != 400 {
		t.Errorf("admin patch validation: %d", code)
	}
	// members keep access to the normal workspace validation path
	if code, body := do(t, app, "POST", "/api/v1/kocs", member, `{"tiktokId":"x"}`); code != 400 {
		t.Errorf("member kocs create: %d %v", code, body)
	}
}

func TestPublicRegistrationClosedAndForgotIsGeneric(t *testing.T) {
	app, _, _ := newApp(t)
	code, body := do(t, app, "POST", "/api/v1/auth/register", "", `{"name":"New User","email":"n@u.vn","password":"Passw0rd1"}`)
	if code != 403 || body["success"] != false {
		t.Fatalf("registration must be closed by default: %d %v", code, body)
	}
	code, body = do(t, app, "POST", "/api/v1/auth/forgot-password", "", `{"email":"nobody@nowhere.vn"}`)
	if code != 200 || body["message"] == nil {
		t.Fatalf("forgot-password must answer generically: %d %v", code, body)
	}
}
