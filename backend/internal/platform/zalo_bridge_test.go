package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/service"
)

func fakeBridge(t *testing.T, connected *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Bridge-Secret") != "s3" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /status":
			state := "disconnected"
			if *connected {
				state = "connected"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"state": state, "qrImage": "", "error": ""})
		case "POST /connect":
			*connected = true
			_ = json.NewEncoder(w).Encode(map[string]string{"state": "connected"})
		case "POST /find-user":
			_ = json.NewEncoder(w).Encode(map[string]string{"userId": "42", "displayName": "Linh"})
		case "POST /send":
			_ = json.NewEncoder(w).Encode(map[string]bool{"delivered": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestZaloBridgeAgainstSidecarContract(t *testing.T) {
	connected := false
	srv := fakeBridge(t, &connected)
	defer srv.Close()
	cfg := config.Config{ZaloBridgeURL: srv.URL, ZaloBridgeSecret: "s3", ZaloPersonalCookie: "[]", ZaloPersonalIMEI: "i", ZaloPersonalUserAgent: "u"}
	b := NewZaloBridge(cfg)
	ctx := context.Background()

	if st := b.Status(ctx); st.Connected || st.State != "disconnected" || !st.Configured {
		t.Fatalf("initial status: %+v", st)
	}
	if _, err := b.ResolveByPhone(ctx, "0327637213"); err == nil {
		t.Fatal("resolve must fail while disconnected")
	}
	if st, err := b.Connect(ctx, *b.EnvCredentials()); err != nil || !st.Connected {
		t.Fatalf("connect: %v %+v", err, st)
	}
	if u, err := b.ResolveByPhone(ctx, "+84 (327) 637-213"); err != nil || u.UserID != "42" || u.DisplayName != "Linh" {
		t.Fatalf("resolve: %v %+v", err, u)
	}
	if err := b.Send(ctx, "42", "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}

	// A wrong secret is rejected and the API degrades to "disconnected".
	bad := NewZaloBridge(config.Config{ZaloBridgeURL: srv.URL, ZaloBridgeSecret: "nope"})
	if bad.IsConnected(ctx) {
		t.Fatal("wrong secret must not report connected")
	}
}

func TestZaloServiceWithoutAnyProvider(t *testing.T) {
	connected := false
	srv := fakeBridge(t, &connected)
	defer srv.Close()
	cfg := config.Config{ZaloBridgeURL: srv.URL, ZaloBridgeSecret: "s3"}
	z := service.NewZaloService(cfg, NewZaloBridge(cfg), NewZaloOAClient())
	res, err := z.SendText(context.Background(), "42", "hi")
	if err != nil || res.Delivered || res.Reason != "missing_token" {
		t.Fatalf("no provider must report missing_token: %+v %v", res, err)
	}
	connected = true
	if res, err := z.SendText(context.Background(), "42", "hi"); err != nil || !res.Delivered || res.Provider != "personal" {
		t.Fatalf("personal provider: %+v %v", res, err)
	}
}
