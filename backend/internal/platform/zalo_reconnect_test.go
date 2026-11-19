package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
)

// sidecar stands for a zalo-bridge that can be restarted (it forgets the session) on its own.
type sidecar struct {
	srv          *httptest.Server
	state        atomic.Value // what GET /status answers
	connects     atomic.Int32
	connectFails atomic.Bool
}

func newSidecar(t *testing.T, state string) *sidecar {
	t.Helper()
	s := &sidecar{}
	s.state.Store(state)
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /status":
			_ = json.NewEncoder(w).Encode(map[string]string{"state": s.state.Load().(string)})
		case "POST /connect":
			s.connects.Add(1)
			if s.connectFails.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "login refused"})
				return
			}
			s.state.Store("connected")
			_ = json.NewEncoder(w).Encode(map[string]string{"state": "connected"})
		case "POST /disconnect":
			s.state.Store("disconnected")
			_ = json.NewEncoder(w).Encode(map[string]string{"state": "disconnected"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func reconnecting(s *sidecar, now *time.Time) *ZaloBridge {
	b := NewZaloBridge(config.Config{ZaloBridgeURL: s.srv.URL, ZaloPersonalCookie: "[]", ZaloPersonalIMEI: "i", ZaloPersonalUserAgent: "u"})
	b.now = func() time.Time { return *now }
	return b
}

func TestABridgeThatRestartedIsConnectedAgain(t *testing.T) {
	s := newSidecar(t, "disconnected")
	now := time.Unix(1000, 0)
	b := reconnecting(s, &now)

	b.EnsureConnected(context.Background())

	if got := s.connects.Load(); got != 1 || s.state.Load() != "connected" {
		t.Fatalf("connects=%d state=%v, want one login and a connected session", got, s.state.Load())
	}
}

func TestAConnectedSessionIsLeftAlone(t *testing.T) {
	s := newSidecar(t, "connected")
	now := time.Unix(1000, 0)
	b := reconnecting(s, &now)

	b.EnsureConnected(context.Background())

	if got := s.connects.Load(); got != 0 {
		t.Fatalf("%d logins for a session that was already connected", got)
	}
}

func TestALoginInProgressIsNotInterrupted(t *testing.T) {
	for _, state := range []string{"connecting", "waiting_qr", "scanned"} {
		s := newSidecar(t, state)
		now := time.Unix(1000, 0)
		b := reconnecting(s, &now)

		b.EnsureConnected(context.Background())

		if got := s.connects.Load(); got != 0 {
			t.Fatalf("state %q: %d logins while somebody is logging in", state, got)
		}
	}
}

func TestADisconnectThePersonAskedForIsKept(t *testing.T) {
	s := newSidecar(t, "connected")
	now := time.Unix(1000, 0)
	b := reconnecting(s, &now)

	b.Disconnect(context.Background())
	b.EnsureConnected(context.Background())

	if got := s.connects.Load(); got != 0 {
		t.Fatalf("%d logins after the person disconnected the account on purpose", got)
	}
}

func TestLoggingInAgainByHandEndsTheDisconnect(t *testing.T) {
	s := newSidecar(t, "connected")
	now := time.Unix(1000, 0)
	b := reconnecting(s, &now)
	ctx := context.Background()

	b.Disconnect(ctx)
	if _, err := b.Connect(ctx, *b.EnvCredentials()); err != nil {
		t.Fatal(err)
	}
	s.state.Store("disconnected") // the bridge restarts later
	b.EnsureConnected(ctx)

	if got := s.connects.Load(); got != 2 {
		t.Fatalf("%d logins, want 2 (the manual one and the automatic one)", got)
	}
}

func TestWithoutCredentialsNothingIsTried(t *testing.T) {
	s := newSidecar(t, "disconnected")
	now := time.Unix(1000, 0)
	b := NewZaloBridge(config.Config{ZaloBridgeURL: s.srv.URL})
	b.now = func() time.Time { return now }

	b.EnsureConnected(context.Background())

	if got := s.connects.Load(); got != 0 {
		t.Fatalf("%d logins without credentials", got)
	}
}

func TestABridgeThatIsDownIsNotCountedAsAFailedLogin(t *testing.T) {
	s := newSidecar(t, "disconnected")
	now := time.Unix(1000, 0)
	b := reconnecting(s, &now)
	s.srv.Close()

	b.EnsureConnected(context.Background()) // nobody to answer: nothing to log in to

	s2 := newSidecar(t, "disconnected")
	b.cfg.ZaloBridgeURL = s2.srv.URL // the bridge is back, on the same footing
	b.EnsureConnected(context.Background())

	if got := s2.connects.Load(); got != 1 {
		t.Fatalf("%d logins once the bridge came back, want 1 right away (no back-off for a bridge that was down)", got)
	}
}

func TestARefusedLoginIsRetriedLessAndLessOften(t *testing.T) {
	s := newSidecar(t, "disconnected")
	s.connectFails.Store(true)
	now := time.Unix(1000, 0)
	b := reconnecting(s, &now)
	ctx := context.Background()

	b.EnsureConnected(ctx)
	b.EnsureConnected(ctx) // same moment: waits
	if got := s.connects.Load(); got != 1 {
		t.Fatalf("%d logins at once, want 1", got)
	}

	now = now.Add(reconnectBackoffMin) // first wait over
	b.EnsureConnected(ctx)
	if got := s.connects.Load(); got != 2 {
		t.Fatalf("%d logins after the first wait, want 2", got)
	}

	now = now.Add(reconnectBackoffMin) // the second wait is longer than the first
	b.EnsureConnected(ctx)
	if got := s.connects.Load(); got != 2 {
		t.Fatalf("%d logins, the second wait must be longer than the first", got)
	}

	for i := 0; i < 10; i++ { // the wait never grows past the ceiling
		now = now.Add(reconnectBackoffMax)
		b.EnsureConnected(ctx)
	}
	if got := s.connects.Load(); got != 12 {
		t.Fatalf("%d logins, want one per ceiling-long wait (12)", got)
	}
}

func TestAfterARefusedLoginTheNextSuccessResetsTheWait(t *testing.T) {
	s := newSidecar(t, "disconnected")
	s.connectFails.Store(true)
	now := time.Unix(1000, 0)
	b := reconnecting(s, &now)
	ctx := context.Background()

	b.EnsureConnected(ctx)
	s.connectFails.Store(false)
	now = now.Add(reconnectBackoffMin)
	b.EnsureConnected(ctx) // works
	s.state.Store("disconnected")
	b.EnsureConnected(ctx) // restart again, straight away

	if got := s.connects.Load(); got != 3 {
		t.Fatalf("%d logins, want 3 (the wait starts over after a success)", got)
	}
}

func TestKeepConnectedLogsInAtOnceAndStopsWithItsContext(t *testing.T) {
	s := newSidecar(t, "disconnected")
	b := NewZaloBridge(config.Config{ZaloBridgeURL: s.srv.URL, ZaloPersonalCookie: "[]", ZaloPersonalIMEI: "i", ZaloPersonalUserAgent: "u"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { b.KeepConnected(ctx, time.Hour); close(done) }()

	deadline := time.After(3 * time.Second)
	for s.connects.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("no login within 3 s of starting")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("KeepConnected did not stop when its context ended")
	}
}
