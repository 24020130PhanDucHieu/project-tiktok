package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// ---- TTShopHandler.Authorize, with a fake TikTok service ----

type fakeTTShopAPI struct {
	authorizeErr error
	codes        []string
}

func (f *fakeTTShopAPI) Status(context.Context) service.TTShopStatus {
	return service.TTShopStatus{Configured: true, Connected: true, SellerName: "CoolTech"}
}
func (f *fakeTTShopAPI) RecentLogs(int) []string       { return nil }
func (f *fakeTTShopAPI) Usage(int) service.UsageReport { return service.UsageReport{} }
func (f *fakeTTShopAPI) OpenCollaborations(context.Context, string, string, int) (*service.OpenCollabPage, error) {
	return &service.OpenCollabPage{}, nil
}
func (f *fakeTTShopAPI) OpenCollabSettings(context.Context) (*service.OpenCollabSettings, error) {
	return &service.OpenCollabSettings{}, nil
}
func (f *fakeTTShopAPI) Authorize(_ context.Context, code string) error {
	f.codes = append(f.codes, code)
	return f.authorizeErr
}

func postJSON(app *fiber.App, path, body string) (int, map[string]any) {
	req := httptest.NewRequest("POST", path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, 10000)
	b, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func TestAuthorizeNeedsACodeAndLogsTheAttempt(t *testing.T) {
	tts, au := &fakeTTShopAPI{}, &fakeAudit{}
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/authorize", NewTTShopHandler(tts, au).Authorize)

	if code, _ := postJSON(app, "/authorize", `{}`); code != 400 || len(tts.codes) != 0 || len(au.actions) != 0 {
		t.Fatalf("a missing code must stop at the handler: %d, tts %v, audit %v", code, tts.codes, au.actions)
	}
	code, body := postJSON(app, "/authorize", `{"authCode":"abc"}`)
	if code != 200 || len(tts.codes) != 1 || tts.codes[0] != "abc" {
		t.Fatalf("%d %v tts %v", code, body, tts.codes)
	}
	if len(au.actions) != 1 || au.actions[0] != "ttshop.authorize" {
		t.Fatalf("the attempt must be audited: %v", au.actions)
	}
	if d, _ := body["data"].(map[string]any); d["connected"] != true || d["sellerName"] != "CoolTech" {
		t.Fatalf("the answer is the fresh status: %v", body)
	}
}

func TestAuthorizeFailureBecomesA502WithTheReason(t *testing.T) {
	tts := &fakeTTShopAPI{authorizeErr: errors.New("auth_code expired")}
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/authorize", NewTTShopHandler(tts, &fakeAudit{}).Authorize)
	code, body := postJSON(app, "/authorize", `{"authCode":"old"}`)
	if code != 502 || body["success"] != false || body["message"] != "auth_code expired" {
		t.Fatalf("%d %v", code, body)
	}
}

// ---- AuthHandler.Login, with a fake auth service ----

type fakeAuthAPI struct {
	session  *service.Session
	err      error
	logins   []string
	signUps  []service.RegistrationRequest
	settings service.RegistrationOptions
}

func (f *fakeAuthAPI) Register(_ context.Context, r service.RegistrationRequest) error {
	f.signUps = append(f.signUps, r)
	return f.err
}
func (f *fakeAuthAPI) RegistrationOptions() service.RegistrationOptions { return f.settings }
func (f *fakeAuthAPI) Login(_ context.Context, email, _, _ string) (*service.Session, error) {
	f.logins = append(f.logins, email)
	return f.session, f.err
}
func (f *fakeAuthAPI) Profile(context.Context, string) (*domain.User, error)  { return nil, nil }
func (f *fakeAuthAPI) UpdateName(context.Context, *domain.User, string) error { return nil }
func (f *fakeAuthAPI) ChangePassword(context.Context, *domain.User, string, string, string) (*service.Session, error) {
	return f.session, f.err
}
func (f *fakeAuthAPI) SendResetCode(context.Context, string, string) {}
func (f *fakeAuthAPI) ResetPassword(context.Context, string, string, string, string) error {
	return nil
}

func TestLoginValidatesTheBodyBeforeCallingTheService(t *testing.T) {
	au := &fakeAuthAPI{session: &service.Session{Token: "tok", User: service.UserView{ID: "1", Email: "a@x.vn", Role: "admin"}}}
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/login", NewAuthHandler(au, &fakeAudit{}).Login)

	for _, bad := range []string{`{}`, `{"email":"not-an-email","password":"longenough1"}`, `{"email":"a@x.vn","password":"short"}`} {
		if code, _ := postJSON(app, "/login", bad); code != 400 || len(au.logins) != 0 {
			t.Fatalf("%s must be refused by the handler: %d, service calls %v", bad, code, au.logins)
		}
	}
	code, body := postJSON(app, "/login", `{"email":"a@x.vn","password":"longenough1"}`)
	d, _ := body["data"].(map[string]any)
	if code != 200 || d["token"] != "tok" || len(au.logins) != 1 {
		t.Fatalf("%d %v", code, body)
	}
	if u, _ := d["user"].(map[string]any); u["role"] != "admin" {
		t.Fatalf("user view expected: %v", d)
	}
}

func TestLoginPassesTheServicesHTTPErrorThrough(t *testing.T) {
	au := &fakeAuthAPI{err: httpx.NewError(401, "Email hoặc mật khẩu không đúng")}
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/login", NewAuthHandler(au, &fakeAudit{}).Login)
	code, body := postJSON(app, "/login", `{"email":"a@x.vn","password":"longenough1"}`)
	if code != 401 || body["success"] != false {
		t.Fatalf("%d %v", code, body)
	}
}
