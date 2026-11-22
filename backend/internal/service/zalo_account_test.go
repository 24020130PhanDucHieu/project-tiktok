package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// memZaloAccounts is an in-memory ZaloAccountStore.
type memZaloAccounts struct{ rows []*domain.ZaloAccount }

func (m *memZaloAccounts) Create(_ context.Context, a *domain.ZaloAccount) error {
	a.ID = domain.NewID()
	c := *a
	m.rows = append(m.rows, &c)
	return nil
}
func (m *memZaloAccounts) Update(_ context.Context, a *domain.ZaloAccount) error {
	for i, r := range m.rows {
		if r.ID == a.ID {
			c := *a
			m.rows[i] = &c
		}
	}
	return nil
}
func (m *memZaloAccounts) Find(_ context.Context, id domain.ID) (*domain.ZaloAccount, error) {
	for _, r := range m.rows {
		if r.ID == id {
			c := *r
			return &c, nil
		}
	}
	return nil, nil
}
func (m *memZaloAccounts) FindByUID(_ context.Context, uid string) (*domain.ZaloAccount, error) {
	for _, r := range m.rows {
		if r.ZaloUID == uid {
			c := *r
			return &c, nil
		}
	}
	return nil, nil
}
func (m *memZaloAccounts) List(context.Context) ([]domain.ZaloAccount, error) {
	out := make([]domain.ZaloAccount, 0, len(m.rows))
	for _, r := range m.rows {
		out = append(out, *r)
	}
	return out, nil
}
func (m *memZaloAccounts) Delete(_ context.Context, id domain.ID) error {
	for i, r := range m.rows {
		if r.ID == id {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			return nil
		}
	}
	return nil
}

func newAccountSvc(t *testing.T, cfg config.Config) (*ZaloAccountService, *memZaloAccounts) {
	t.Helper()
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "test-secret-test-secret-test-secret"
	}
	store := &memZaloAccounts{}
	return NewZaloAccountService(cfg, store), store
}

func creds(cookie string) ZaloCredentials {
	return ZaloCredentials{Cookie: json.RawMessage(cookie), IMEI: "imei-1", UserAgent: "UA/1"}
}

func TestSavedCredentialsAreNeverStoredInThePlain(t *testing.T) {
	svc, store := newAccountSvc(t, config.Config{})
	ctx := context.Background()
	acc, err := svc.Create(ctx, "Chính")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveCredentials(ctx, acc.ID, "uid-1", "Tên", creds(`[{"key":"zpw_sek","value":"SECRET-COOKIE"}]`)); err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"SECRET-COOKIE", "imei-1", "UA/1"} {
		if strings.Contains(store.rows[0].Credentials, needle) {
			t.Fatalf("the stored credentials show %q in the plain", needle)
		}
	}
	if !store.rows[0].HasCredentials() {
		t.Fatal("credentials were not stored")
	}
}

func TestCredentialsComeBackAsTheyWereSaved(t *testing.T) {
	svc, _ := newAccountSvc(t, config.Config{})
	ctx := context.Background()
	acc, _ := svc.Create(ctx, "Chính")
	_, _ = svc.SaveCredentials(ctx, acc.ID, "uid-1", "", creds(`[{"key":"k","value":"v"}]`))
	got, err := svc.Credentials(ctx, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Cookie) != `[{"key":"k","value":"v"}]` || got.IMEI != "imei-1" || got.UserAgent != "UA/1" {
		t.Fatalf("got %+v", got)
	}
}

func TestAnAccountWithoutCredentialsNeedsAQRLogin(t *testing.T) {
	svc, _ := newAccountSvc(t, config.Config{})
	ctx := context.Background()
	acc, _ := svc.Create(ctx, "Mới")
	if _, err := svc.Credentials(ctx, acc.ID); err != ErrZaloNoCredentials {
		t.Fatalf("err = %v, want ErrZaloNoCredentials", err)
	}
}

func TestCredentialsSealedWithAnotherKeyAreRejected(t *testing.T) {
	a, storeA := newAccountSvc(t, config.Config{JWTSecret: "secret-A-secret-A-secret-A-secret-A"})
	ctx := context.Background()
	acc, _ := a.Create(ctx, "x")
	_, _ = a.SaveCredentials(ctx, acc.ID, "uid", "", creds(`[]`))
	b := NewZaloAccountService(config.Config{JWTSecret: "secret-B-secret-B-secret-B-secret-B"}, storeA)
	if _, err := b.Credentials(ctx, acc.ID); err == nil {
		t.Fatal("a different key opened the credentials")
	}
}

func TestLoggingInAgainWithTheSameZaloUIDUpdatesTheOldAccount(t *testing.T) {
	svc, store := newAccountSvc(t, config.Config{})
	ctx := context.Background()
	first, _ := svc.Create(ctx, "Chính")
	_, _ = svc.SaveCredentials(ctx, first.ID, "uid-1", "A", creds(`[{"key":"k","value":"old"}]`))
	// somebody adds an account and scans the QR code of the same Zalo account: it must not become a second row
	second, _ := svc.Create(ctx, "Thêm")
	kept, err := svc.SaveCredentials(ctx, second.ID, "uid-1", "A", creds(`[{"key":"k","value":"new"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if kept.ID != first.ID {
		t.Fatalf("kept %s, want the first account %s", kept.ID, first.ID)
	}
	if len(store.rows) != 1 {
		t.Fatalf("%d accounts, want 1", len(store.rows))
	}
	got, _ := svc.Credentials(ctx, first.ID)
	if !strings.Contains(string(got.Cookie), "new") {
		t.Fatalf("the cookie was not replaced: %s", got.Cookie)
	}
}

func TestSavingForAnAccountThatWasRemovedIsRefused(t *testing.T) {
	svc, _ := newAccountSvc(t, config.Config{})
	if _, err := svc.SaveCredentials(context.Background(), domain.NewID(), "uid", "", creds(`[]`)); err == nil {
		t.Fatal("saved credentials for an account that does not exist")
	}
}

func TestTheEnvAccountBecomesTheDefaultAccountOnce(t *testing.T) {
	cfg := config.Config{ZaloPersonalCookie: `[{"key":"k","value":"v"}]`, ZaloPersonalIMEI: "imei-env", ZaloPersonalUserAgent: "UA-env"}
	svc, store := newAccountSvc(t, cfg)
	ctx := context.Background()
	if err := svc.SeedDefault(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.SeedDefault(ctx); err != nil { // a second start must not add a second one
		t.Fatal(err)
	}
	if len(store.rows) != 1 || !store.rows[0].IsDefault {
		t.Fatalf("rows = %+v", store.rows)
	}
	got, err := svc.Credentials(ctx, store.rows[0].ID)
	if err != nil || got.IMEI != "imei-env" {
		t.Fatalf("creds = %+v, err = %v", got, err)
	}
}

func TestNothingIsSeededWithoutEnvCredentials(t *testing.T) {
	svc, store := newAccountSvc(t, config.Config{})
	if err := svc.SeedDefault(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 0 {
		t.Fatal("an account was seeded from nothing")
	}
}

func TestEnvIsNotSeededOverAccountsThatAlreadyExist(t *testing.T) {
	cfg := config.Config{ZaloPersonalCookie: `[]`, ZaloPersonalIMEI: "i", ZaloPersonalUserAgent: "u"}
	svc, store := newAccountSvc(t, cfg)
	ctx := context.Background()
	_, _ = svc.Create(ctx, "Đã có")
	if err := svc.SeedDefault(ctx); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 1 {
		t.Fatalf("%d accounts, want 1", len(store.rows))
	}
}

func TestDefaultIsTheMarkedOneOtherwiseTheFirst(t *testing.T) {
	svc, _ := newAccountSvc(t, config.Config{})
	ctx := context.Background()
	if d, _ := svc.Default(ctx); d != nil {
		t.Fatal("a default with no accounts")
	}
	a, _ := svc.Create(ctx, "A")
	_, _ = svc.Create(ctx, "B")
	if d, _ := svc.Default(ctx); d == nil || d.ID != a.ID {
		t.Fatalf("default = %+v, want the first account", d)
	}
}

func TestCreateNeedsALabel(t *testing.T) {
	svc, _ := newAccountSvc(t, config.Config{})
	if _, err := svc.Create(context.Background(), "   "); err == nil {
		t.Fatal("created an account without a label")
	}
}

// usageIs answers how many conversations an account has.
type usageIs struct {
	n   int64
	err error
}

func (u usageIs) CountByZaloAccount(context.Context, string) (int64, error) { return u.n, u.err }

func TestAnAccountThatStillHasConversationsCannotBeDeleted(t *testing.T) {
	svc, store := newAccountSvc(t, config.Config{})
	svc.WithUsage(usageIs{n: 3})
	ctx := context.Background()
	acc, _ := svc.Create(ctx, "Chính")

	err := svc.Delete(ctx, acc.ID)

	var fault *domain.Fault
	if !errors.As(err, &fault) || fault.Kind != domain.FaultConflict {
		t.Fatalf("err = %v, want a conflict fault", err)
	}
	if !strings.Contains(fault.Message, "3") {
		t.Fatalf("message %q should say how many conversations are left", fault.Message)
	}
	if len(store.rows) != 1 {
		t.Fatal("the account was deleted anyway")
	}
}

func TestAnAccountWithoutConversationsIsDeleted(t *testing.T) {
	svc, store := newAccountSvc(t, config.Config{})
	svc.WithUsage(usageIs{n: 0})
	ctx := context.Background()
	acc, _ := svc.Create(ctx, "Phụ")
	if err := svc.Delete(ctx, acc.ID); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 0 {
		t.Fatal("the account is still there")
	}
}

func TestWhenTheConversationsCannotBeCountedNothingIsDeleted(t *testing.T) {
	svc, store := newAccountSvc(t, config.Config{})
	svc.WithUsage(usageIs{err: errors.New("db down")})
	ctx := context.Background()
	acc, _ := svc.Create(ctx, "Phụ")
	if err := svc.Delete(ctx, acc.ID); err == nil {
		t.Fatal("deleted without knowing whether conversations depend on the account")
	}
	if len(store.rows) != 1 {
		t.Fatal("the account was deleted anyway")
	}
}

func TestCanDeleteAnswersWithoutDeleting(t *testing.T) {
	svc, store := newAccountSvc(t, config.Config{})
	svc.WithUsage(usageIs{n: 0})
	ctx := context.Background()
	acc, _ := svc.Create(ctx, "Phụ")
	if err := svc.CanDelete(ctx, acc.ID); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 1 {
		t.Fatal("CanDelete deleted the account")
	}
}
