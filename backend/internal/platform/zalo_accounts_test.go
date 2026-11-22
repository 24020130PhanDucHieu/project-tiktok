package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// multiSidecar is a zalo-bridge with one session per account (the X-Zalo-Account header names it).
type multiSidecar struct {
	srv *httptest.Server
	mu  sync.Mutex
	// state of each account's session; an account it does not know is disconnected
	state    map[string]string
	connects map[string]int    // POST /connect per account
	imeis    map[string]string // the imei each account logged in with
	refuse   map[string]bool   // accounts whose login Zalo refuses
	headers  []string          // the account header of every request, in order
	forgets  []string          // the accounts POST /forget was asked to drop
	persists map[string]bool   // accounts whose login asked to be reported back
}

func newMultiSidecar(t *testing.T) *multiSidecar {
	t.Helper()
	m := &multiSidecar{state: map[string]string{}, connects: map[string]int{}, imeis: map[string]string{}, refuse: map[string]bool{}, persists: map[string]bool{}}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acct := r.Header.Get("X-Zalo-Account")
		m.mu.Lock()
		defer m.mu.Unlock()
		m.headers = append(m.headers, acct)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /status":
			st := m.state[acct]
			if st == "" {
				st = "disconnected"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"state": st})
		case "POST /connect":
			m.connects[acct]++
			var body struct {
				IMEI    string `json:"imei"`
				Persist bool   `json:"persist"`
			}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			if m.refuse[acct] {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "login refused"})
				return
			}
			m.imeis[acct] = body.IMEI
			m.persists[acct] = body.Persist
			m.state[acct] = "connected"
			_ = json.NewEncoder(w).Encode(map[string]string{"state": "connected"})
		case "GET /accounts":
			_ = json.NewEncoder(w).Encode(func() map[string]map[string]string {
				out := map[string]map[string]string{}
				for id, st := range m.state {
					out[id] = map[string]string{"state": st}
				}
				return out
			}())
		case "POST /forget":
			m.forgets = append(m.forgets, acct)
			delete(m.state, acct)
			_ = json.NewEncoder(w).Encode(map[string]bool{"forgotten": true})
		case "POST /disconnect":
			m.state[acct] = "disconnected"
			_ = json.NewEncoder(w).Encode(map[string]string{"state": "disconnected"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *multiSidecar) connectsOf(acct string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connects[acct]
}

// memSource is the stored accounts the bridge keeps connected.
type memSource struct {
	mu       sync.Mutex
	accounts []domain.ZaloAccount
	creds    map[domain.ID]service.ZaloCredentials
	disabled map[domain.ID]bool
}

func newMemSource() *memSource {
	return &memSource{creds: map[domain.ID]service.ZaloCredentials{}, disabled: map[domain.ID]bool{}}
}

func (s *memSource) add(label, imei string, withCreds bool) domain.ID {
	a := domain.ZaloAccount{ID: domain.NewID(), Label: label}
	if withCreds {
		a.Credentials = "sealed"
		s.creds[a.ID] = service.ZaloCredentials{Cookie: json.RawMessage(`[]`), IMEI: imei, UserAgent: "ua"}
	}
	s.accounts = append(s.accounts, a)
	return a.ID
}

func (s *memSource) List(context.Context) ([]domain.ZaloAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.ZaloAccount, len(s.accounts))
	copy(out, s.accounts)
	for i := range out {
		out[i].Disabled = s.disabled[out[i].ID]
	}
	return out, nil
}
func (s *memSource) Default(ctx context.Context) (*domain.ZaloAccount, error) {
	all, _ := s.List(ctx)
	if len(all) == 0 {
		return nil, nil
	}
	return &all[0], nil
}
func (s *memSource) Credentials(_ context.Context, id domain.ID) (*service.ZaloCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.creds[id]
	if !ok {
		return nil, service.ErrZaloNoCredentials
	}
	return &c, nil
}
func (s *memSource) SetDisabled(_ context.Context, id domain.ID, disabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disabled[id] = disabled
	return nil
}

func multiBridge(m *multiSidecar, src *memSource, now *time.Time) *ZaloBridge {
	b := NewZaloBridge(config.Config{ZaloBridgeURL: m.srv.URL})
	b.now = func() time.Time { return *now }
	b.SetAccounts(src)
	return b
}

func TestEveryStoredAccountIsLoggedInWithItsOwnCredentials(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	a := src.add("A", "imei-A", true)
	b := src.add("B", "imei-B", true)
	now := time.Unix(1000, 0)

	multiBridge(m, src, &now).EnsureConnected(context.Background())

	if m.connectsOf(a.Hex()) != 1 || m.connectsOf(b.Hex()) != 1 {
		t.Fatalf("connects = %v, want one login each", m.connects)
	}
	if m.imeis[a.Hex()] != "imei-A" || m.imeis[b.Hex()] != "imei-B" {
		t.Fatalf("imeis = %v: an account logged in with the credentials of another", m.imeis)
	}
}

func TestAnAccountThatIsAlreadyConnectedIsLeftAlone(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	a := src.add("A", "i", true)
	b := src.add("B", "i", true)
	m.state[a.Hex()] = "connected"
	now := time.Unix(1000, 0)

	multiBridge(m, src, &now).EnsureConnected(context.Background())

	if m.connectsOf(a.Hex()) != 0 || m.connectsOf(b.Hex()) != 1 {
		t.Fatalf("connects = %v, want only B logged in", m.connects)
	}
}

func TestAnAccountWithoutCredentialsWaitsForItsQRCode(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	src.add("Mới", "", false)
	now := time.Unix(1000, 0)

	multiBridge(m, src, &now).EnsureConnected(context.Background())

	if len(m.connects) != 0 {
		t.Fatalf("connects = %v, nothing to log in with", m.connects)
	}
}

func TestADisabledAccountIsNotLoggedIn(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	a := src.add("A", "i", true)
	src.disabled[a] = true
	now := time.Unix(1000, 0)

	multiBridge(m, src, &now).EnsureConnected(context.Background())

	if len(m.connects) != 0 {
		t.Fatalf("connects = %v, the person disconnected this account", m.connects)
	}
}

func TestARefusedAccountDoesNotHoldBackTheOthers(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	bad := src.add("hết hạn", "i", true)
	good := src.add("tốt", "i", true)
	m.refuse[bad.Hex()] = true
	now := time.Unix(1000, 0)
	br := multiBridge(m, src, &now)
	ctx := context.Background()

	br.EnsureConnected(ctx)
	br.EnsureConnected(ctx) // same moment: the refused account waits, the good one is connected

	if m.connectsOf(bad.Hex()) != 1 {
		t.Fatalf("the refused account was tried %d times at once, want 1 (it backs off)", m.connectsOf(bad.Hex()))
	}
	if m.connectsOf(good.Hex()) != 1 || m.state[good.Hex()] != "connected" {
		t.Fatalf("the good account: connects=%d state=%s", m.connectsOf(good.Hex()), m.state[good.Hex()])
	}
	now = now.Add(reconnectBackoffMin)
	br.EnsureConnected(ctx)
	if m.connectsOf(bad.Hex()) != 2 {
		t.Fatalf("the refused account was tried %d times after its wait, want 2", m.connectsOf(bad.Hex()))
	}
}

func TestDisconnectingOneAccountLeavesTheOthersAndIsRemembered(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	a := src.add("A", "i", true)
	b := src.add("B", "i", true)
	m.state[a.Hex()], m.state[b.Hex()] = "connected", "connected"
	now := time.Unix(1000, 0)
	br := multiBridge(m, src, &now)

	br.Disconnect(service.WithZaloAccount(context.Background(), a.Hex()))

	if m.state[a.Hex()] != "disconnected" || m.state[b.Hex()] != "connected" {
		t.Fatalf("states = %v", m.state)
	}
	if !src.disabled[a] || src.disabled[b] {
		t.Fatalf("disabled = %v: the choice to disconnect A must be stored, B untouched", src.disabled)
	}
	br.EnsureConnected(context.Background()) // and the automatic login respects it
	if m.connectsOf(a.Hex()) != 0 {
		t.Fatal("the automatic login brought back an account the person disconnected")
	}
}

func TestLoggingInByHandEndsTheDisconnectOfThatAccount(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	a := src.add("A", "i", true)
	src.disabled[a] = true
	now := time.Unix(1000, 0)
	br := multiBridge(m, src, &now)

	if _, err := br.Connect(service.WithZaloAccount(context.Background(), a.Hex()), service.ZaloCredentials{Cookie: json.RawMessage(`[]`), IMEI: "i", UserAgent: "u"}); err != nil {
		t.Fatal(err)
	}
	if src.disabled[a] {
		t.Fatal("the account is still marked as disconnected after a manual login")
	}
}

func TestEachCallNamesItsAccountToTheBridge(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	a := src.add("A", "i", true)
	src.add("B", "i", true)
	now := time.Unix(1000, 0)
	br := multiBridge(m, src, &now)

	br.Status(service.WithZaloAccount(context.Background(), a.Hex()))
	if got := m.headers[len(m.headers)-1]; got != a.Hex() {
		t.Fatalf("header = %q, want %q", got, a.Hex())
	}
}

func TestACallWithoutAnAccountIsForTheDefaultAccount(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	first := src.add("A", "i", true)
	src.add("B", "i", true)
	now := time.Unix(1000, 0)

	multiBridge(m, src, &now).Status(context.Background())

	if got := m.headers[len(m.headers)-1]; got != first.Hex() {
		t.Fatalf("header = %q, want the default account %q", got, first.Hex())
	}
}

func TestWithoutStoredAccountsTheBridgeStaysTheOldSingleAccount(t *testing.T) {
	m := newMultiSidecar(t)
	now := time.Unix(1000, 0)
	b := NewZaloBridge(config.Config{ZaloBridgeURL: m.srv.URL})
	b.now = func() time.Time { return now }

	b.Status(context.Background())

	if got := m.headers[len(m.headers)-1]; got != "" {
		t.Fatalf("header = %q, want none", got)
	}
}

func TestTheStatusOfEveryAccountComesFromOneCall(t *testing.T) {
	m := newMultiSidecar(t)
	m.state["acc-A"], m.state["acc-B"] = "connected", "error"
	now := time.Unix(1000, 0)
	b := NewZaloBridge(config.Config{ZaloBridgeURL: m.srv.URL})
	b.now = func() time.Time { return now }

	got, online := b.AccountStatuses(context.Background())

	if !online || len(got) != 2 || !got["acc-A"].Connected || got["acc-B"].Connected || got["acc-B"].State != "error" {
		t.Fatalf("online=%v statuses=%+v", online, got)
	}
	if len(m.headers) != 1 {
		t.Fatalf("%d requests, want one for all the accounts", len(m.headers))
	}
}

func TestAnUnreachableBridgeIsOfflineNotEmpty(t *testing.T) {
	m := newMultiSidecar(t)
	b := NewZaloBridge(config.Config{ZaloBridgeURL: m.srv.URL})
	m.srv.Close()
	if got, online := b.AccountStatuses(context.Background()); online || len(got) != 0 {
		t.Fatalf("online=%v statuses=%v: a bridge that cannot be reached must say so", online, got)
	}
}

func TestForgettingAnAccountNamesItToTheBridge(t *testing.T) {
	m := newMultiSidecar(t)
	m.state["acc-A"] = "connected"
	b := NewZaloBridge(config.Config{ZaloBridgeURL: m.srv.URL})

	if err := b.Forget(service.WithZaloAccount(context.Background(), "acc-A")); err != nil {
		t.Fatal(err)
	}
	if len(m.forgets) != 1 || m.forgets[0] != "acc-A" {
		t.Fatalf("forgets = %v", m.forgets)
	}
}

func TestOnlyALoginAPersonMadeAsksToBeReportedBack(t *testing.T) {
	m := newMultiSidecar(t)
	src := newMemSource()
	auto := src.add("tự động", "i", true)
	now := time.Unix(1000, 0)
	b := multiBridge(m, src, &now)

	b.EnsureConnected(context.Background()) // the automatic login uses what is stored: nothing to report
	if m.persists[auto.Hex()] {
		t.Fatal("the automatic login asked the bridge to report credentials that are already stored")
	}
	manual := service.WithZaloAccount(context.Background(), "acc-M")
	if _, err := b.Connect(manual, service.ZaloCredentials{Cookie: json.RawMessage(`[]`), IMEI: "i", UserAgent: "u", Persist: true}); err != nil {
		t.Fatal(err)
	}
	if !m.persists["acc-M"] {
		t.Fatal("a login typed in by a person did not ask to be stored")
	}
}
