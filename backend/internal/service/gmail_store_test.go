package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// memAccounts is an in-memory GmailAccountStore, so the Gmail rules can be tested without MongoDB.
type memAccounts struct {
	mu     sync.Mutex
	byUser map[string]domain.GmailAccount
	broken []string
}

func newMemAccounts() *memAccounts { return &memAccounts{byUser: map[string]domain.GmailAccount{}} }

func (m *memAccounts) Save(_ context.Context, a *domain.GmailAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byUser[a.UserID] = *a
	return nil
}
func (m *memAccounts) Find(_ context.Context, id string) (*domain.GmailAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.byUser[id]; ok {
		return &a, nil
	}
	return nil, nil
}
func (m *memAccounts) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byUser, id)
	return nil
}
func (m *memAccounts) ListHealthy(context.Context) ([]domain.GmailAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.GmailAccount
	for _, a := range m.byUser {
		if !a.Broken {
			out = append(out, a)
		}
	}
	return out, nil
}
func (m *memAccounts) MarkBroken(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broken = append(m.broken, id)
	return nil
}

func TestGmailAccountLifecycleGoesThroughTheStore(t *testing.T) {
	store := newMemAccounts()
	g := NewGmailService(config.Config{}, store, nil)
	ctx := context.Background()
	if a, err := g.Account(ctx, "u1"); a != nil || err != nil {
		t.Fatalf("no mailbox yet: %v %v", a, err)
	}
	_ = store.Save(ctx, &domain.GmailAccount{UserID: "u1", Email: "a@x.vn"})
	_ = store.Save(ctx, &domain.GmailAccount{UserID: "u2", Email: "b@x.vn", Broken: true})
	if a, _ := g.Account(ctx, "u1"); a == nil || a.Email != "a@x.vn" {
		t.Fatalf("%v", a)
	}
	if list, _ := g.Linked(ctx); len(list) != 1 || list[0].UserID != "u1" {
		t.Fatalf("only healthy mailboxes are polled: %v", list)
	}
	if err := g.Disconnect(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if a, _ := g.Account(ctx, "u1"); a != nil {
		t.Fatal("disconnected mailbox must be gone")
	}
}

func TestGmailAccessTokenRefusesMissingAndBrokenMailboxesWithoutCallingGoogle(t *testing.T) {
	store := newMemAccounts()
	g := NewGmailService(config.Config{}, store, nil)
	ctx := context.Background()
	if _, _, err := g.accessToken(ctx, "nobody"); !errors.Is(err, ErrGmailNotConnected) {
		t.Fatalf("got %v", err)
	}
	_ = store.Save(ctx, &domain.GmailAccount{UserID: "u2", Email: "b@x.vn", Broken: true})
	if _, _, err := g.accessToken(ctx, "u2"); !errors.Is(err, ErrGmailBroken) {
		t.Fatalf("got %v", err)
	}
}
