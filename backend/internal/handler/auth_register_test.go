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

func TestTheSignUpOptionsArePublic(t *testing.T) {
	au := &fakeAuthAPI{settings: service.RegistrationOptions{AllowRegistration: true, CaptchaSiteKey: "0x4AAA-site"}}
	resp, err := registerApp(au).Test(httptest.NewRequest("GET", "/options", nil), 5000)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
}
