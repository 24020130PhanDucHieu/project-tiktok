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
		case "POST /friend-status":
			_ = json.NewEncoder(w).Encode(map[string]bool{"isFriend": false, "requestSent": true, "requestReceived": false})
		case "POST /friend-request", "POST /react":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			lastCall = r.URL.Path + " " + in["userId"] + "|" + in["message"] + "|" + in["msgId"] + "|" + in["cliMsgId"] + "|" + in["icon"]
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
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

// lastCall is what the fake sidecar last received on the friend and reaction routes.
var lastCall string

func TestZaloBridgeFriendRequestAndReactionContract(t *testing.T) {
	connected := true
	srv := fakeBridge(t, &connected)
	defer srv.Close()
	b := NewZaloBridge(config.Config{ZaloBridgeURL: srv.URL, ZaloBridgeSecret: "s3"})
	ctx := context.Background()

	st, err := b.FriendStatus(ctx, "42")
	if err != nil || st.IsFriend || !st.RequestSent || st.RequestReceived {
		t.Fatalf("friend status: %v %+v", err, st)
	}
	if err := b.SendFriendRequest(ctx, "42", "Chào bạn"); err != nil || lastCall != "/friend-request 42|Chào bạn|||" {
		t.Fatalf("friend request: %v %q", err, lastCall)
	}
	if err := b.React(ctx, "42", "100", "200", "/-heart"); err != nil || lastCall != "/react 42||100|200|/-heart" {
		t.Fatalf("react: %v %q", err, lastCall)
	}

	connected = false
	if err := b.SendFriendRequest(ctx, "42", "x"); err == nil {
		t.Fatal("no friend request without the Zalo line")
	}
	if err := b.React(ctx, "42", "1", "2", ""); err == nil {
		t.Fatal("no reaction without the Zalo line")
	}
}
