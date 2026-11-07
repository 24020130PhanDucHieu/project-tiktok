package router_test

import (
	"net/http"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/config"
)

// An event Zalo sends that the handler ignores before it touches the database: it shows the guard let it through.
const ignoredEvent = `{"event_name":"follow","user_id":"u1"}`

func TestTheZaloWebhookDoesNotExistUntilASecretIsConfigured(t *testing.T) {
	app, _, _ := newApp(t) // no ZaloWebhookSecret
	for _, method := range []string{"GET", "POST"} {
		if code, _ := do(t, app, method, "/api/v1/webhooks/zalo", "", ignoredEvent); code != http.StatusNotFound {
			t.Fatalf("%s without a configured secret must not be served, got %d", method, code)
		}
	}
}

func TestTheZaloWebhookRefusesACallerWithoutTheSecret(t *testing.T) {
	app, _, _ := newAppCfg(t, func(c *config.Config) { c.ZaloWebhookSecret = "hook-secret-hook-secret-hook-secret" })

	for _, c := range []struct {
		name  string
		path  string
		extra [][2]string
	}{
		{"no secret", "/api/v1/webhooks/zalo", nil},
		{"wrong header", "/api/v1/webhooks/zalo", [][2]string{{"X-Webhook-Secret", "nope"}}},
		{"wrong query", "/api/v1/webhooks/zalo?secret=nope", nil},
		{"empty query", "/api/v1/webhooks/zalo?secret=", nil},
	} {
		if code, _ := do(t, app, "POST", c.path, "", ignoredEvent, c.extra...); code != http.StatusUnauthorized {
			t.Errorf("%s: a forged event must be refused, got %d", c.name, code)
		}
	}
}

func TestTheZaloWebhookAcceptsTheSecretInTheHeaderOrTheUrl(t *testing.T) {
	const secret = "hook-secret-hook-secret-hook-secret"
	app, _, _ := newAppCfg(t, func(c *config.Config) { c.ZaloWebhookSecret = secret })

	if code, body := do(t, app, "POST", "/api/v1/webhooks/zalo", "", ignoredEvent, [2]string{"X-Webhook-Secret", secret}); code != 200 || body["ignored"] != true {
		t.Fatalf("header secret: %d %v", code, body)
	}
	// Zalo's console only takes a URL, so the secret may also ride in the query string
	if code, body := do(t, app, "POST", "/api/v1/webhooks/zalo?secret="+secret, "", ignoredEvent); code != 200 || body["ignored"] != true {
		t.Fatalf("url secret: %d %v", code, body)
	}
	if code, _ := do(t, app, "GET", "/api/v1/webhooks/zalo?secret="+secret, "", ""); code != 200 {
		t.Fatalf("the verification call with the secret: %d", code)
	}
}
