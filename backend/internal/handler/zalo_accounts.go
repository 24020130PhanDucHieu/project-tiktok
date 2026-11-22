package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// ZaloAccountAdmin is the part of *service.ZaloAccountService this handler uses.
type ZaloAccountAdmin interface {
	List(ctx context.Context) ([]domain.ZaloAccount, error)
	Get(ctx context.Context, id domain.ID) (*domain.ZaloAccount, error)
	Create(ctx context.Context, label string) (*domain.ZaloAccount, error)
	Delete(ctx context.Context, id domain.ID) error
	// CanDelete says whether the account may go now (no conversation may still talk through it).
	CanDelete(ctx context.Context, id domain.ID) error
	Default(ctx context.Context) (*domain.ZaloAccount, error)
	SaveCredentials(ctx context.Context, id domain.ID, zaloUID, displayName string, c service.ZaloCredentials) (*domain.ZaloAccount, error)
}

// ZaloAccountsBridge is the part of *platform.ZaloBridge this handler uses. Every call is for the account of its
// context (domain.WithZaloAccount).
type ZaloAccountsBridge interface {
	Status(ctx context.Context) service.ZaloStatus
	StartQR(ctx context.Context) service.ZaloStatus
	Disconnect(ctx context.Context) service.ZaloStatus
	Connect(ctx context.Context, creds service.ZaloCredentials) (service.ZaloStatus, error)
	// AccountStatuses is the live status of every session the bridge holds, by account id; false when the bridge cannot be reached.
	AccountStatuses(ctx context.Context) (map[string]service.ZaloStatus, bool)
	// Forget makes the bridge drop the session of the account.
	Forget(ctx context.Context) error
}

// ZaloAccountsHandler serves the personal Zalo accounts: the list, adding and removing an account, logging each in
// (QR code or cookie) and out, and the callback with which the bridge hands over the credentials of a login.
type ZaloAccountsHandler struct {
	accounts ZaloAccountAdmin
	bridge   ZaloAccountsBridge
	audit    Auditor
}

// NewZaloAccountsHandler builds a ZaloAccountsHandler.
func NewZaloAccountsHandler(accounts ZaloAccountAdmin, bridge ZaloAccountsBridge, audit Auditor) *ZaloAccountsHandler {
	return &ZaloAccountsHandler{accounts: accounts, bridge: bridge, audit: audit}
}

// zaloAccountView is an account as the screen sees it: who it is and whether it is logged in. The credentials are
// never part of it (ZaloAccount keeps them out of its JSON, and nothing here adds them back).
type zaloAccountView struct {
	domain.ZaloAccount
	State        string `json:"state"`
	Connected    bool   `json:"connected"`
	QRImage      string `json:"qrImage,omitempty"`
	Error        string `json:"error,omitempty"`
	BridgeOnline bool   `json:"bridgeOnline"`
	HasLogin     bool   `json:"hasCredentials"` // it can log in without a QR code
	NeedsQR      bool   `json:"needsQr"`
}

func viewOf(a domain.ZaloAccount, st service.ZaloStatus) zaloAccountView {
	if st.State == "" {
		st.State = "disconnected"
	}
	return zaloAccountView{ZaloAccount: a, State: st.State, Connected: st.Connected, QRImage: st.QRImage, Error: st.Error,
		BridgeOnline: st.BridgeOnline, HasLogin: a.HasCredentials(), NeedsQR: !a.HasCredentials()}
}

// List handles GET /zalo/accounts: every account with its live status, from one call to the bridge.
func (h *ZaloAccountsHandler) List(c *fiber.Ctx) error {
	ctx := c.UserContext()
	all, err := h.accounts.List(ctx)
	if err != nil {
		return err
	}
	live, online := h.bridge.AccountStatuses(ctx)
	out := make([]zaloAccountView, 0, len(all))
	for _, a := range all {
		st := live[a.ID.Hex()] // an account the bridge holds no session for is disconnected
		st.BridgeOnline = online
		if !online {
			st.Error = "Không kết nối được dịch vụ Zalo (zalo-bridge)."
		}
		out = append(out, viewOf(a, st))
	}
	return httpx.OK(c, out)
}

// accountOf reads the account in the URL and returns it with a context made for it.
func (h *ZaloAccountsHandler) accountOf(c *fiber.Ctx) (*domain.ZaloAccount, context.Context, error) {
	id, err := domain.ParseID(c.Params("id"))
	if err != nil {
		return nil, nil, domain.Invalid("Mã tài khoản Zalo không hợp lệ.")
	}
	acc, err := h.accounts.Get(c.UserContext(), id)
	if err != nil {
		return nil, nil, err
	}
	return acc, domain.WithZaloAccount(c.UserContext(), acc.ID.Hex()), nil
}

// Create handles POST /zalo/accounts: adds an account with no login yet (its QR code comes next).
func (h *ZaloAccountsHandler) Create(c *fiber.Ctx) error {
	var in struct {
		Label string `json:"label"`
	}
	if err := c.BodyParser(&in); err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Dữ liệu không hợp lệ")
	}
	acc, err := h.accounts.Create(c.UserContext(), in.Label)
	if err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	h.audit.Log(nil, a.ID, a.Email, "zalo.account.create", "zalo_account", acc.ID.Hex(), acc.Label, a.IP)
	return httpx.OK(c, viewOf(*acc, service.ZaloStatus{State: "disconnected", BridgeOnline: true}))
}

// Delete handles DELETE /zalo/accounts/:id. An account that conversations still talk through is refused (409) before
// anything is touched; otherwise the bridge drops the session, then the account and its credentials go.
func (h *ZaloAccountsHandler) Delete(c *fiber.Ctx) error {
	acc, ctx, err := h.accountOf(c)
	if err != nil {
		return err
	}
	if err := h.accounts.CanDelete(c.UserContext(), acc.ID); err != nil {
		return err
	}
	if err := h.bridge.Forget(ctx); err != nil {
		fmt.Printf("zalo account %s: the bridge did not forget the session: %v\n", acc.ID.Hex(), err)
	}
	if err := h.accounts.Delete(c.UserContext(), acc.ID); err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	h.audit.Log(nil, a.ID, a.Email, "zalo.account.delete", "zalo_account", acc.ID.Hex(), acc.Label, a.IP)
	return httpx.OK(c, fiber.Map{"deleted": true})
}

// Status handles GET /zalo/accounts/:id/status (the screen polls it for the QR code and the login progress).
func (h *ZaloAccountsHandler) Status(c *fiber.Ctx) error {
	acc, ctx, err := h.accountOf(c)
	if err != nil {
		return err
	}
	return httpx.OK(c, viewOf(*acc, h.bridge.Status(ctx)))
}

// StartQR handles POST /zalo/accounts/:id/qr/start.
func (h *ZaloAccountsHandler) StartQR(c *fiber.Ctx) error {
	acc, ctx, err := h.accountOf(c)
	if err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	h.audit.Log(nil, a.ID, a.Email, "zalo.account.qr", "zalo_account", acc.ID.Hex(), acc.Label, a.IP)
	return httpx.OK(c, viewOf(*acc, h.bridge.StartQR(ctx)))
}

// Disconnect handles POST /zalo/accounts/:id/disconnect: the automatic login leaves the account alone from now on.
func (h *ZaloAccountsHandler) Disconnect(c *fiber.Ctx) error {
	acc, ctx, err := h.accountOf(c)
	if err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	h.audit.Log(nil, a.ID, a.Email, "zalo.account.disconnect", "zalo_account", acc.ID.Hex(), acc.Label, a.IP)
	return httpx.OK(c, viewOf(*acc, h.bridge.Disconnect(ctx)))
}

// Connect handles POST /zalo/accounts/:id/connect with cookie, imei and userAgent exported from Zalo Web.
func (h *ZaloAccountsHandler) Connect(c *fiber.Ctx) error {
	acc, ctx, err := h.accountOf(c)
	if err != nil {
		return err
	}
	var creds service.ZaloCredentials
	if err := c.BodyParser(&creds); err != nil || len(creds.Cookie) == 0 || creds.IMEI == "" || creds.UserAgent == "" {
		return httpx.Fail(c, fiber.StatusBadRequest, "Thiếu cookie, IMEI hoặc user-agent Zalo cá nhân.")
	}
	creds.Persist = true // a login a person typed in has to be kept: the bridge reports it back once it works
	a := middleware.ActorOf(c)
	h.audit.Log(nil, a.ID, a.Email, "zalo.account.connect", "zalo_account", acc.ID.Hex(), acc.Label, a.IP)
	st, err := h.bridge.Connect(ctx, creds)
	if err != nil {
		return httpx.NewError(fiber.StatusBadGateway, "Đăng nhập Zalo không thành công: "+err.Error())
	}
	return httpx.OK(c, viewOf(*acc, st))
}

// ReceiveSession handles POST /internal/zalo/session, called by the zalo-bridge after a login that has to be kept (a
// QR scan, a cookie typed in) or when Zalo renewed the cookie. It seals and stores the credentials.
func (h *ZaloAccountsHandler) ReceiveSession(c *fiber.Ctx) error {
	var in struct {
		AccountID   string `json:"accountId"`
		ZaloUID     string `json:"zaloUid"`
		DisplayName string `json:"displayName"`
		service.ZaloCredentials
	}
	if err := c.BodyParser(&in); err != nil || len(in.Cookie) == 0 || in.IMEI == "" || in.UserAgent == "" {
		return httpx.Fail(c, fiber.StatusBadRequest, "Thiếu cookie, IMEI hoặc user-agent.")
	}
	ctx := c.UserContext()
	acc, err := h.sessionAccount(ctx, in.AccountID)
	if err != nil {
		return err
	}
	saved, err := h.accounts.SaveCredentials(ctx, acc.ID, in.ZaloUID, in.DisplayName, in.ZaloCredentials)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "accountId": saved.ID.Hex()})
}

// sessionAccount is the account a login belongs to. The bridge's old single key ("default", or none) is the default
// account; a bridge that logged in with no account stored at all gets the first one made for it, so that a QR
// scan is never lost.
func (h *ZaloAccountsHandler) sessionAccount(ctx context.Context, key string) (*domain.ZaloAccount, error) {
	if key = strings.TrimSpace(key); key != "" && key != "default" {
		id, err := domain.ParseID(key)
		if err != nil {
			return nil, domain.Invalid("Mã tài khoản Zalo không hợp lệ.")
		}
		return h.accounts.Get(ctx, id)
	}
	if def, err := h.accounts.Default(ctx); err != nil || def != nil {
		return def, err
	}
	return h.accounts.Create(ctx, "Tài khoản chính")
}
