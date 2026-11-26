package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

func registerApp(au *fakeAuthAPI) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	h := NewAuthHandler(au, &fakeAudit{})
	app.Post("/register", h.Register)
	app.Get("/options", h.Options)
	return app
}

func TestASignUpIsAcceptedForReviewWithoutASession(t *testing.T) {
	au := &fakeAuthAPI{}
	code, body := postJSON(registerApp(au), "/register",
		`{"name":"Lan Anh","email":"lan@x.vn","password":"Matkhau123","website":"","captchaToken":"tok","role":"admin"}`)
	if code != 202 || body["success"] != true || body["message"] != service.RegistrationNotice {
		t.Fatalf("a sign-up waits for review: %d %v", code, body)
	}
	if _, has := body["data"]; has {
		t.Fatalf("no token or account may be returned before approval: %v", body)
	}
	if len(au.signUps) != 1 || au.signUps[0].CaptchaToken != "tok" || au.signUps[0].Email != "lan@x.vn" {
		t.Fatalf("the form must reach the service as it was sent: %+v", au.signUps)
	}
}

func TestASignUpFormIsValidatedBeforeAnythingElse(t *testing.T) {
	au := &fakeAuthAPI{}
	for _, body := range []string{`{}`, `{"name":"A","email":"x","password":"Matkhau123"}`, `{"name":"Lan","email":"lan@x.vn","password":"short"}`} {
		if code, _ := postJSON(registerApp(au), "/register", body); code != 400 {
			t.Fatalf("%s must be refused, got %d", body, code)
		}
	}
	if len(au.signUps) != 0 {
		t.Fatal("an invalid form never reaches the service")
	}
}

func emailAuthApp(au *fakeAuthAPI, audit *fakeAudit) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/email", NewAuthHandler(au, audit).EmailAuth)
	return app
}

func TestEmailAuthWithTheHoneypotFilledNeverReachesTheService(t *testing.T) {
	au := &fakeAuthAPI{session: &service.Session{Token: "tok"}}
	audit := &fakeAudit{}
	app := emailAuthApp(au, audit)
	for _, hp := range []string{"http://spam.example", "   x  "} {
		code, body := postJSON(app, "/email", `{"email":"bot@x.vn","password":"Matkhau123","website":"`+hp+`"}`)
		if code != 202 || body["success"] != true || body["message"] != service.RegistrationNotice {
			t.Fatalf("a honeypot hit looks like a sign-up waiting for review: %d %v", code, body)
		}
		if _, has := body["data"]; has {
			t.Fatalf("no token or session may be returned: %v", body)
		}
	}
	if len(au.logins) != 0 {
		t.Fatalf("the service must not be called: %v", au.logins)
	}
	if len(audit.actions) != 2 || audit.actions[0] != "auth.email_bot" {
		t.Fatalf("the hit must be audited: %v", audit.actions)
	}
}

func TestEmailAuthWithTheHoneypotEmptyRunsNormally(t *testing.T) {
	au := &fakeAuthAPI{session: &service.Session{Token: "tok", User: service.UserView{ID: "1", Email: "a@x.vn", Role: "member"}}}
	app := emailAuthApp(au, &fakeAudit{})
	for _, body := range []string{
		`{"email":"a@x.vn","password":"Matkhau123"}`,
		`{"email":"a@x.vn","password":"Matkhau123","website":""}`,
		`{"email":"a@x.vn","password":"Matkhau123","website":"   "}`,
	} {
		code, out := postJSON(app, "/email", body)
		d, _ := out["data"].(map[string]any)
		if code != 200 || d["token"] != "tok" {
			t.Fatalf("%s: %d %v", body, code, out)
		}
	}
	if len(au.logins) != 3 {
		t.Fatalf("the service must be called once per request: %v", au.logins)
	}
}

func TestTheSignUpOptionsArePublic(t *testing.T) {
	au := &fakeAuthAPI{settings: service.RegistrationOptions{AllowRegistration: true, CaptchaSiteKey: "0x4AAA-site"}}
	resp, err := registerApp(au).Test(httptest.NewRequest("GET", "/options", nil), 5000)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
}
