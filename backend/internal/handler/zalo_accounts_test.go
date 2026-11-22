package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// memAdmin is the account store behind the handler.
type memAdmin struct {
	blocked error // what CanDelete answers
	rows    []domain.ZaloAccount
	saved   []savedCreds
	deleted []domain.ID
}

type savedCreds struct {
	id         domain.ID
	uid        string
	creds      service.ZaloCredentials
	returnedAs domain.ID
}

func (m *memAdmin) List(context.Context) ([]domain.ZaloAccount, error) { return m.rows, nil }
func (m *memAdmin) Get(_ context.Context, id domain.ID) (*domain.ZaloAccount, error) {
	for i := range m.rows {
		if m.rows[i].ID == id {
			return &m.rows[i], nil
		}
	}
	return nil, domain.NotFound("Không tìm thấy tài khoản Zalo.")
}
func (m *memAdmin) Create(_ context.Context, label string) (*domain.ZaloAccount, error) {
	if strings.TrimSpace(label) == "" {
		return nil, domain.Invalid("Cần đặt tên cho tài khoản Zalo.")
	}
	a := domain.ZaloAccount{ID: domain.NewID(), Label: label}
	m.rows = append(m.rows, a)
	return &a, nil
}
func (m *memAdmin) Delete(_ context.Context, id domain.ID) error {
	m.deleted = append(m.deleted, id)
	return nil
}
func (m *memAdmin) CanDelete(context.Context, domain.ID) error { return m.blocked }
func (m *memAdmin) Default(context.Context) (*domain.ZaloAccount, error) {
	if len(m.rows) == 0 {
		return nil, nil
	}
	return &m.rows[0], nil
}
func (m *memAdmin) SaveCredentials(_ context.Context, id domain.ID, uid, _ string, c service.ZaloCredentials) (*domain.ZaloAccount, error) {
	a, err := m.Get(context.Background(), id)
	if err != nil {
		return nil, err
	}
	m.saved = append(m.saved, savedCreds{id: id, uid: uid, creds: c, returnedAs: a.ID})
	a.Credentials, a.ZaloUID = "sealed", uid
	return a, nil
}

// bridgeNotes is a bridge that notes the account of every call.
type bridgeNotes struct {
	statuses map[string]service.ZaloStatus
	calls    []string // "op:account"
	creds    []service.ZaloCredentials
	connErr  error
}

func (b *bridgeNotes) note(op string, ctx context.Context) {
	b.calls = append(b.calls, op+":"+domain.ZaloAccountFrom(ctx))
}
func (b *bridgeNotes) Status(ctx context.Context) service.ZaloStatus {
	b.note("status", ctx)
	return b.statuses[domain.ZaloAccountFrom(ctx)]
}
func (b *bridgeNotes) StartQR(ctx context.Context) service.ZaloStatus {
	b.note("qr", ctx)
	return service.ZaloStatus{State: "waiting_qr", BridgeOnline: true}
}
func (b *bridgeNotes) Disconnect(ctx context.Context) service.ZaloStatus {
	b.note("disconnect", ctx)
	return service.ZaloStatus{State: "disconnected", BridgeOnline: true}
}
func (b *bridgeNotes) Connect(ctx context.Context, c service.ZaloCredentials) (service.ZaloStatus, error) {
	b.note("connect", ctx)
	b.creds = append(b.creds, c)
	return service.ZaloStatus{State: "connected", Connected: true, BridgeOnline: true}, b.connErr
}
func (b *bridgeNotes) AccountStatuses(context.Context) (map[string]service.ZaloStatus, bool) {
	return b.statuses, true
}
func (b *bridgeNotes) Forget(ctx context.Context) error {
	b.note("forget", ctx)
	return nil
}

func accountsApp(store *memAdmin, br *bridgeNotes, audit *fakeAudit) *fiber.App {
	h := NewZaloAccountsHandler(store, br, audit)
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Get("/accounts", h.List)
	app.Post("/accounts", h.Create)
	app.Delete("/accounts/:id", h.Delete)
	app.Get("/accounts/:id/status", h.Status)
	app.Post("/accounts/:id/qr/start", h.StartQR)
	app.Post("/accounts/:id/connect", h.Connect)
	app.Post("/accounts/:id/disconnect", h.Disconnect)
	app.Post("/session", h.ReceiveSession)
	return app
}

func call(app *fiber.App, method, path, body string) (int, string) {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, 10000)
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func twoAccounts() (*memAdmin, domain.ID, domain.ID) {
	a := domain.ZaloAccount{ID: domain.NewID(), Label: "Chính", IsDefault: true, Credentials: "sealed-cookie-blob"}
	b := domain.ZaloAccount{ID: domain.NewID(), Label: "Phụ"}
	return &memAdmin{rows: []domain.ZaloAccount{a, b}}, a.ID, b.ID
}

func TestTheListJoinsTheStoredAccountsWithTheirLiveStatus(t *testing.T) {
	store, a, b := twoAccounts()
	br := &bridgeNotes{statuses: map[string]service.ZaloStatus{a.Hex(): {State: "connected", Connected: true, BridgeOnline: true}}}
	code, raw := call(accountsApp(store, br, &fakeAudit{}), "GET", "/accounts", "")
	if code != 200 {
		t.Fatalf("%d %s", code, raw)
	}
	var out struct {
		Data []struct {
			ID             string `json:"_id"`
			Label          string `json:"label"`
			State          string `json:"state"`
			Connected      bool   `json:"connected"`
			HasCredentials bool   `json:"hasCredentials"`
			NeedsQR        bool   `json:"needsQr"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil || len(out.Data) != 2 {
		t.Fatalf("%v %s", err, raw)
	}
	if got := out.Data[0]; got.ID != a.Hex() || !got.Connected || got.State != "connected" || !got.HasCredentials || got.NeedsQR {
		t.Fatalf("first = %+v", got)
	}
	if got := out.Data[1]; got.ID != b.Hex() || got.Connected || got.State != "disconnected" || got.HasCredentials || !got.NeedsQR {
		t.Fatalf("an account the bridge does not hold is disconnected and needs its QR code: %+v", got)
	}
}

func TestTheListNeverCarriesCredentials(t *testing.T) {
	store, _, _ := twoAccounts()
	_, raw := call(accountsApp(store, &bridgeNotes{}, &fakeAudit{}), "GET", "/accounts", "")
	if strings.Contains(raw, "sealed-cookie-blob") {
		t.Fatalf("the list shows the stored credentials: %s", raw)
	}
	var out struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil || len(out.Data) == 0 {
		t.Fatalf("%v %s", err, raw)
	}
	for _, item := range out.Data {
		for _, key := range []string{"credentials", "cookie", "imei", "userAgent"} {
			if _, has := item[key]; has {
				t.Fatalf("an account carries %q: %v", key, item)
			}
		}
	}
}

func TestAnAccountNeedsALabel(t *testing.T) {
	store := &memAdmin{}
	app := accountsApp(store, &bridgeNotes{}, &fakeAudit{})
	if code, _ := call(app, "POST", "/accounts", `{"label":"  "}`); code != 400 || len(store.rows) != 0 {
		t.Fatalf("code %d, %d rows", code, len(store.rows))
	}
	audit := &fakeAudit{}
	app = accountsApp(store, &bridgeNotes{}, audit)
	if code, _ := call(app, "POST", "/accounts", `{"label":"Phụ"}`); code != 200 || len(store.rows) != 1 {
		t.Fatalf("code %d, %d rows", code, len(store.rows))
	}
	if len(audit.actions) != 1 || audit.actions[0] != "zalo.account.create" {
		t.Fatalf("audit = %v", audit.actions)
	}
}

func TestAnAccountThatDoesNotExistIsRefusedBeforeTheBridgeIsTouched(t *testing.T) {
	store, _, _ := twoAccounts()
	br := &bridgeNotes{}
	app := accountsApp(store, br, &fakeAudit{})
	for _, path := range []string{"/accounts/" + domain.NewID().Hex() + "/qr/start", "/accounts/not-an-id/qr/start"} {
		if code, _ := call(app, "POST", path, ""); code != 404 && code != 400 {
			t.Fatalf("%s answered %d", path, code)
		}
	}
	if len(br.calls) != 0 {
		t.Fatalf("the bridge was called for an unknown account: %v", br.calls)
	}
}

func TestEveryActionRunsForTheAccountInTheURL(t *testing.T) {
	store, _, b := twoAccounts()
	br := &bridgeNotes{statuses: map[string]service.ZaloStatus{}}
	app := accountsApp(store, br, &fakeAudit{})
	for _, req := range [][2]string{{"POST", "/qr/start"}, {"POST", "/disconnect"}, {"GET", "/status"}} {
		if code, raw := call(app, req[0], "/accounts/"+b.Hex()+req[1], ""); code != 200 {
			t.Fatalf("%s %s: %d %s", req[0], req[1], code, raw)
		}
	}
	for _, c := range br.calls {
		if !strings.HasSuffix(c, ":"+b.Hex()) {
			t.Fatalf("a call went to %q, want account %s: %v", c, b.Hex(), br.calls)
		}
	}
	if len(br.calls) != 3 {
		t.Fatalf("calls = %v", br.calls)
	}
}

func TestAManualLoginHasToBeKeptAndNeedsAllThreeParts(t *testing.T) {
	store, _, b := twoAccounts()
	br := &bridgeNotes{}
	app := accountsApp(store, br, &fakeAudit{})
	if code, _ := call(app, "POST", "/accounts/"+b.Hex()+"/connect", `{"imei":"i"}`); code != 400 || len(br.creds) != 0 {
		t.Fatalf("an incomplete login: %d", code)
	}
	body := `{"cookie":[{"key":"k","value":"v"}],"imei":"i","userAgent":"u"}`
	if code, raw := call(app, "POST", "/accounts/"+b.Hex()+"/connect", body); code != 200 {
		t.Fatalf("%d %s", code, raw)
	}
	if len(br.creds) != 1 || !br.creds[0].Persist {
		t.Fatalf("creds = %+v: a login typed in by a person must be stored", br.creds)
	}
	if br.calls[0] != "connect:"+b.Hex() {
		t.Fatalf("calls = %v", br.calls)
	}
}

func TestAFailedLoginIsAnErrorForThePerson(t *testing.T) {
	store, _, b := twoAccounts()
	br := &bridgeNotes{connErr: errors.New("cookie hết hạn")}
	code, _ := call(accountsApp(store, br, &fakeAudit{}), "POST", "/accounts/"+b.Hex()+"/connect", `{"cookie":[],"imei":"i","userAgent":"u"}`)
	if code < 400 {
		t.Fatalf("code %d, want an error", code)
	}
}

func TestDeletingAnAccountMakesTheBridgeForgetItFirst(t *testing.T) {
	store, _, b := twoAccounts()
	br := &bridgeNotes{}
	audit := &fakeAudit{}
	if code, raw := call(accountsApp(store, br, audit), "DELETE", "/accounts/"+b.Hex(), ""); code != 200 {
		t.Fatalf("%d %s", code, raw)
	}
	if len(br.calls) != 1 || br.calls[0] != "forget:"+b.Hex() {
		t.Fatalf("bridge calls = %v", br.calls)
	}
	if len(store.deleted) != 1 || store.deleted[0] != b {
		t.Fatalf("deleted = %v", store.deleted)
	}
	if len(audit.actions) != 1 || audit.actions[0] != "zalo.account.delete" {
		t.Fatalf("audit = %v", audit.actions)
	}
}

// --- the bridge reports the credentials of a login

func TestTheBridgeCredentialsAreStoredUnderTheAccountThatLoggedIn(t *testing.T) {
	store, _, b := twoAccounts()
	body := `{"accountId":"` + b.Hex() + `","zaloUid":"uid-9","cookie":[{"key":"k","value":"v"}],"imei":"i","userAgent":"u"}`
	if code, raw := call(accountsApp(store, &bridgeNotes{}, &fakeAudit{}), "POST", "/session", body); code != 200 {
		t.Fatalf("%d %s", code, raw)
	}
	if len(store.saved) != 1 || store.saved[0].id != b || store.saved[0].uid != "uid-9" || store.saved[0].creds.IMEI != "i" {
		t.Fatalf("saved = %+v", store.saved)
	}
}

func TestTheOldBridgeKeyIsTheDefaultAccount(t *testing.T) {
	store, a, _ := twoAccounts()
	for _, key := range []string{`"default"`, `""`} {
		store.saved = nil
		body := `{"accountId":` + key + `,"zaloUid":"u","cookie":[],"imei":"i","userAgent":"u"}`
		if code, raw := call(accountsApp(store, &bridgeNotes{}, &fakeAudit{}), "POST", "/session", body); code != 200 {
			t.Fatalf("%d %s", code, raw)
		}
		if len(store.saved) != 1 || store.saved[0].id != a {
			t.Fatalf("key %s: saved = %+v, want the default account %s", key, store.saved, a)
		}
	}
}

func TestALoginOnABridgeWithNoStoredAccountCreatesTheFirstOne(t *testing.T) {
	store := &memAdmin{}
	body := `{"accountId":"default","zaloUid":"u","cookie":[],"imei":"i","userAgent":"u"}`
	if code, raw := call(accountsApp(store, &bridgeNotes{}, &fakeAudit{}), "POST", "/session", body); code != 200 {
		t.Fatalf("%d %s", code, raw)
	}
	if len(store.rows) != 1 || len(store.saved) != 1 {
		t.Fatalf("rows %d saved %d: a QR login must not be lost for want of an account", len(store.rows), len(store.saved))
	}
}

func TestACredentialReportWithoutAnythingToLogInWithIsRefused(t *testing.T) {
	store, _, b := twoAccounts()
	for _, body := range []string{
		`{"accountId":"` + b.Hex() + `"}`,
		`{"accountId":"` + b.Hex() + `","cookie":[],"imei":"i"}`,
		`{"accountId":"not-an-id","cookie":[],"imei":"i","userAgent":"u"}`,
	} {
		if code, _ := call(accountsApp(store, &bridgeNotes{}, &fakeAudit{}), "POST", "/session", body); code != 400 && code != 404 {
			t.Fatalf("%s answered %d", body, code)
		}
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved = %+v", store.saved)
	}
}

func TestAnAccountInUseIsRefusedBeforeTheBridgeForgetsIt(t *testing.T) {
	store, _, b := twoAccounts()
	store.blocked = domain.Conflict("Tài khoản này còn 4 cuộc trò chuyện nên chưa xóa được")
	br := &bridgeNotes{}
	audit := &fakeAudit{}
	code, raw := call(accountsApp(store, br, audit), "DELETE", "/accounts/"+b.Hex(), "")
	if code != 409 {
		t.Fatalf("%d %s, want 409", code, raw)
	}
	if !strings.Contains(raw, "4 cuộc trò chuyện") {
		t.Fatalf("the person is not told why: %s", raw)
	}
	if len(br.calls) != 0 {
		t.Fatalf("the bridge dropped the session of an account that was not deleted: %v", br.calls)
	}
	if len(store.deleted) != 0 || len(audit.actions) != 0 {
		t.Fatalf("deleted %v, audit %v", store.deleted, audit.actions)
	}
}
