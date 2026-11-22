package router_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// routeAccounts is a tiny account store for the route tests.
type routeAccounts struct{ rows []domain.ZaloAccount }

func (r *routeAccounts) List(context.Context) ([]domain.ZaloAccount, error) { return r.rows, nil }
func (r *routeAccounts) Get(_ context.Context, id domain.ID) (*domain.ZaloAccount, error) {
	for i := range r.rows {
		if r.rows[i].ID == id {
			return &r.rows[i], nil
		}
	}
	return nil, domain.NotFound("Không tìm thấy tài khoản Zalo.")
}
func (r *routeAccounts) Create(_ context.Context, label string) (*domain.ZaloAccount, error) {
	if label == "" {
		return nil, domain.Invalid("Cần đặt tên cho tài khoản Zalo.")
	}
	a := domain.ZaloAccount{ID: domain.NewID(), Label: label}
	r.rows = append(r.rows, a)
	return &a, nil
}
func (r *routeAccounts) Delete(context.Context, domain.ID) error    { return nil }
func (r *routeAccounts) CanDelete(context.Context, domain.ID) error { return nil }
func (r *routeAccounts) Default(context.Context) (*domain.ZaloAccount, error) {
	if len(r.rows) == 0 {
		return nil, nil
	}
	return &r.rows[0], nil
}
func (r *routeAccounts) SaveCredentials(_ context.Context, id domain.ID, _, _ string, _ service.ZaloCredentials) (*domain.ZaloAccount, error) {
	return r.Get(context.Background(), id)
}

type routeBridge struct{}

func (routeBridge) Status(context.Context) service.ZaloStatus     { return service.ZaloStatus{} }
func (routeBridge) StartQR(context.Context) service.ZaloStatus    { return service.ZaloStatus{} }
func (routeBridge) Disconnect(context.Context) service.ZaloStatus { return service.ZaloStatus{} }
func (routeBridge) Connect(context.Context, service.ZaloCredentials) (service.ZaloStatus, error) {
	return service.ZaloStatus{}, nil
}
func (routeBridge) AccountStatuses(context.Context) (map[string]service.ZaloStatus, bool) {
	return nil, true
}
func (routeBridge) Forget(context.Context) error { return nil }

type routeAudit struct{}

func (routeAudit) Log(context.Context, string, string, string, string, string, string, string) {}

func accountsRouteApp(t *testing.T) (*fiber.App, string, string, *routeAccounts) {
	t.Helper()
	store := &routeAccounts{rows: []domain.ZaloAccount{{ID: domain.NewID(), Label: "Chính"}}}
	app, admin, member := newAppFull(t, nil, nil, nil, nil, func(h *router.Handlers) {
		h.ZaloAccounts = handler.NewZaloAccountsHandler(store, routeBridge{}, routeAudit{})
	})
	return app, admin, member, store
}

func TestTheZaloAccountRoutesNeedASignedInUser(t *testing.T) {
	app, _, _, store := accountsRouteApp(t)
	id := store.rows[0].ID.Hex()
	for _, r := range [][2]string{
		{"GET", "/api/v1/zalo/accounts"}, {"POST", "/api/v1/zalo/accounts"}, {"DELETE", "/api/v1/zalo/accounts/" + id},
		{"GET", "/api/v1/zalo/accounts/" + id + "/status"}, {"POST", "/api/v1/zalo/accounts/" + id + "/qr/start"},
		{"POST", "/api/v1/zalo/accounts/" + id + "/connect"}, {"POST", "/api/v1/zalo/accounts/" + id + "/disconnect"},
	} {
		if code, _ := do(t, app, r[0], r[1], "", `{}`); code != http.StatusUnauthorized {
			t.Fatalf("%s %s without a token: %d", r[0], r[1], code)
		}
	}
}

func TestAMemberSeesTheAccountsButOnlyAnAdminAddsOrRemovesThem(t *testing.T) {
	app, admin, member, store := accountsRouteApp(t)
	id := store.rows[0].ID.Hex()

	if code, _ := do(t, app, "GET", "/api/v1/zalo/accounts", member, ""); code != 200 {
		t.Fatalf("a member listing the accounts: %d", code)
	}
	if code, _ := do(t, app, "POST", "/api/v1/zalo/accounts", member, `{"label":"Phụ"}`); code != http.StatusForbidden {
		t.Fatalf("a member adding an account: %d, want 403", code)
	}
	if code, _ := do(t, app, "DELETE", "/api/v1/zalo/accounts/"+id, member, ""); code != http.StatusForbidden {
		t.Fatalf("a member removing an account: %d, want 403", code)
	}
	if code, _ := do(t, app, "POST", "/api/v1/zalo/accounts", admin, `{"label":"Phụ"}`); code != 200 {
		t.Fatalf("an admin adding an account: %d", code)
	}
	if code, _ := do(t, app, "DELETE", "/api/v1/zalo/accounts/"+id, admin, ""); code != 200 {
		t.Fatalf("an admin removing an account: %d", code)
	}
}

func TestAMemberCanLogAnAccountInAndOut(t *testing.T) {
	app, _, member, store := accountsRouteApp(t)
	id := store.rows[0].ID.Hex()
	for _, path := range []string{"/qr/start", "/disconnect"} {
		if code, _ := do(t, app, "POST", "/api/v1/zalo/accounts/"+id+path, member, ""); code != 200 {
			t.Fatalf("POST %s: %d", path, code)
		}
	}
}

func TestTheSessionCallbackOfTheBridgeIsProtectedByItsSecret(t *testing.T) {
	app, _, _, store := accountsRouteApp(t)
	body := `{"accountId":"` + store.rows[0].ID.Hex() + `","zaloUid":"u","cookie":[],"imei":"i","userAgent":"u"}`
	if code, _ := do(t, app, "POST", "/internal/zalo/session", "", body); code != http.StatusUnauthorized {
		t.Fatalf("without the secret: %d", code)
	}
	if code, _ := do(t, app, "POST", "/internal/zalo/session", "", body, [2]string{"X-Bridge-Secret", "wrong"}); code != http.StatusUnauthorized {
		t.Fatalf("with a wrong secret: %d", code)
	}
	if code, _ := do(t, app, "POST", "/internal/zalo/session", "", `{}`, [2]string{"X-Bridge-Secret", "bridge-secret"}); code != http.StatusBadRequest {
		t.Fatalf("valid secret, empty body: %d", code)
	}
	if code, _ := do(t, app, "POST", "/internal/zalo/session", "", body, [2]string{"X-Bridge-Secret", "bridge-secret"}); code != 200 {
		t.Fatalf("valid secret and body: %d", code)
	}
}

func TestWithoutTheHandlerTheZaloAccountRoutesAreNotServed(t *testing.T) {
	app, admin, _ := newApp(t)
	if code, _ := do(t, app, "GET", "/api/v1/zalo/accounts", admin, ""); code != http.StatusNotFound {
		t.Fatalf("code %d, want 404", code)
	}
	if code, _ := do(t, app, "POST", "/internal/zalo/session", "", `{}`, [2]string{"X-Bridge-Secret", "bridge-secret"}); code != http.StatusNotFound {
		t.Fatalf("session callback: %d, want 404", code)
	}
}
