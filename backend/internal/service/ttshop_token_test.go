package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// memTokens is an in-memory TokenStore: the token logic is now testable without MongoDB.
type memTokens struct {
	mu      sync.Mutex
	tok     *domain.TTSToken
	saves   int
	loadErr error
}

func (m *memTokens) Load(context.Context) (*domain.TTSToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.tok == nil {
		return nil, nil
	}
	c := *m.tok
	return &c, nil
}

func (m *memTokens) Save(_ context.Context, t *domain.TTSToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *t
	m.tok = &c
	m.saves++
	return nil
}

func serviceWithStore(t *testing.T, baseURL string, store TokenStore) *TTShopService {
	t.Helper()
	s := newTTShopForTest(baseURL)
	s.tokens = store
	return s
}

func sealed(t *testing.T, s *TTShopService, plain string) string {
	t.Helper()
	v, err := s.seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNoStoredTokenMeansTheShopIsNotConnected(t *testing.T) {
	s := serviceWithStore(t, "", &memTokens{})
	if _, err := s.accessToken(context.Background()); !errors.Is(err, ErrTTShopNotConnected) {
		t.Fatalf("an empty store must read as not connected, got %v", err)
	}
	s.tokens = nil
	if _, err := s.accessToken(context.Background()); !errors.Is(err, ErrTTShopNotConnected) {
		t.Fatalf("no store at all must also read as not connected, got %v", err)
	}
}

func TestAStoreFailureIsNotReportedAsNotConnected(t *testing.T) {
	boom := errors.New("database down")
	s := serviceWithStore(t, "", &memTokens{loadErr: boom})
	if _, err := s.accessToken(context.Background()); !errors.Is(err, boom) || errors.Is(err, ErrTTShopNotConnected) {
		t.Fatalf("a database problem must surface as itself, got %v", err)
	}
}

func TestAValidAccessTokenIsUsedWithoutAskingTikTok(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	defer srv.Close()
	store := &memTokens{}
	s := serviceWithStore(t, srv.URL, store)
	store.tok = &domain.TTSToken{Access: sealed(t, s, "live-access"), Refresh: sealed(t, s, "live-refresh"),
		AccessExpireAt: time.Now().Add(time.Hour), RefreshExpireAt: time.Now().Add(24 * time.Hour)}
	got, err := s.accessToken(context.Background())
	if err != nil || got != "live-access" || hits != 0 || store.saves != 0 {
		t.Fatalf("token %q err %v hits %d saves %d", got, err, hits, store.saves)
	}
}

func TestAnExpiringAccessTokenIsRefreshedAndTheNewOneIsStoredSealed(t *testing.T) {
	var gotRefresh string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/token/refresh" {
			w.Write([]byte(`{"code":1,"message":"unexpected path"}`))
			return
		}
		gotRefresh = r.URL.Query().Get("refresh_token")
		w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"new-access","access_token_expire_in":3600,"refresh_token":"new-refresh","refresh_token_expire_in":86400,"seller_name":"CoolTech"}}`))
	}))
	defer srv.Close()
	store := &memTokens{}
	s := serviceWithStore(t, srv.URL, store)
	store.tok = &domain.TTSToken{Access: sealed(t, s, "old-access"), Refresh: sealed(t, s, "old-refresh"),
		AccessExpireAt: time.Now().Add(time.Minute), RefreshExpireAt: time.Now().Add(24 * time.Hour)} // inside the 5-minute margin

	got, err := s.accessToken(context.Background())
	if err != nil || got != "new-access" {
		t.Fatalf("token %q err %v", got, err)
	}
	if gotRefresh != "old-refresh" {
		t.Fatalf("the refresh token must be sent decrypted, got %q", gotRefresh)
	}
	if store.saves != 1 {
		t.Fatalf("the new tokens must be saved once, got %d", store.saves)
	}
	if strings.Contains(store.tok.Access, "new-access") || strings.Contains(store.tok.Refresh, "new-refresh") {
		t.Fatal("tokens must be stored encrypted, never in plain text")
	}
	if store.tok.SellerName != "CoolTech" {
		t.Fatalf("seller name kept: %+v", store.tok)
	}
}

func TestAnExpiredRefreshTokenMeansTheShopMustAuthorizeAgain(t *testing.T) {
	store := &memTokens{}
	s := serviceWithStore(t, "", store)
	store.tok = &domain.TTSToken{Access: sealed(t, s, "a"), Refresh: sealed(t, s, "r"),
		AccessExpireAt: time.Now().Add(-time.Hour), RefreshExpireAt: time.Now().Add(-time.Minute)}
	if _, err := s.accessToken(context.Background()); !errors.Is(err, ErrTTShopNotConnected) {
		t.Fatalf("got %v", err)
	}
}

func TestAuthorizeStoresTheTokensEncrypted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("auth_code") != "the-code" {
			w.Write([]byte(`{"code":1,"message":"bad code"}`))
			return
		}
		w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"plain-access","access_token_expire_in":3600,"refresh_token":"plain-refresh","refresh_token_expire_in":86400,"seller_name":"CoolTech"}}`))
	}))
	defer srv.Close()
	store := &memTokens{}
	s := serviceWithStore(t, srv.URL, store)
	if err := s.Authorize(context.Background(), "the-code"); err != nil {
		t.Fatal(err)
	}
	if store.tok == nil || strings.Contains(store.tok.Access, "plain-access") || strings.Contains(store.tok.Refresh, "plain-refresh") {
		t.Fatalf("stored token must exist and be encrypted: %+v", store.tok)
	}
	if plain, err := s.open(store.tok.Access); err != nil || plain != "plain-access" {
		t.Fatalf("it must decrypt back, got %q %v", plain, err)
	}
	if err := s.Authorize(context.Background(), "wrong"); err == nil {
		t.Fatal("a wrong code must fail")
	}
}
