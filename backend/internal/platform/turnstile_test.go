package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeSiteverify answers like Cloudflare's siteverify endpoint and records what it was asked.
func fakeSiteverify(t *testing.T, status int, body string, got *map[string]string) *TurnstileClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		*got = map[string]string{"secret": r.FormValue("secret"), "response": r.FormValue("response"), "remoteip": r.FormValue("remoteip")}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewTurnstileClient("sec-key")
	c.verifyURL = srv.URL
	return c
}

func TestTurnstileAcceptsATokenCloudflareConfirms(t *testing.T) {
	var got map[string]string
	c := fakeSiteverify(t, 200, `{"success":true}`, &got)
	ok, err := c.Verify(context.Background(), "tok", "1.2.3.4")
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if got["secret"] != "sec-key" || got["response"] != "tok" || got["remoteip"] != "1.2.3.4" {
		t.Fatalf("the secret, the token and the visitor IP must be sent: %v", got)
	}
}

func TestTurnstileRefusesARejectedOrMissingToken(t *testing.T) {
	var got map[string]string
	c := fakeSiteverify(t, 200, `{"success":false,"error-codes":["invalid-input-response"]}`, &got)
	if ok, err := c.Verify(context.Background(), "forged", ""); ok || err != nil {
		t.Fatalf("a rejected token is a plain no: %v %v", ok, err)
	}
	if ok, err := c.Verify(context.Background(), "", ""); ok || err != nil {
		t.Fatalf("an empty token is a no without asking Cloudflare: %v %v", ok, err)
	}
}

func TestTurnstileReportsAnOutageAsAnError(t *testing.T) {
	var got map[string]string
	c := fakeSiteverify(t, 503, `oops`, &got)
	if _, err := c.Verify(context.Background(), "tok", ""); err == nil {
		t.Fatal("Cloudflare being down is an error, not a robot")
	}
}
