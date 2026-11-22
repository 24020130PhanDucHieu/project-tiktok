package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// ZaloAccountSource is the stored Zalo accounts the bridge is kept connected for (*service.ZaloAccountService).
type ZaloAccountSource interface {
	List(ctx context.Context) ([]domain.ZaloAccount, error)
	Default(ctx context.Context) (*domain.ZaloAccount, error)
	Credentials(ctx context.Context, id domain.ID) (*service.ZaloCredentials, error)
	SetDisabled(ctx context.Context, id domain.ID, disabled bool) error
}

// loginState is what the automatic login remembers about one account.
type loginState struct {
	disconnectedByUser bool      // the person disconnected the account on purpose: the automatic login must leave it that way
	failures           int       // automatic logins in a row that the bridge refused
	retryAt            time.Time // no automatic login before this moment
}

// How long the id of the default account is remembered, so a call that names no account does not read the database.
const defaultAccountTTL = 10 * time.Second

// ZaloBridge implements PersonalZalo over the sidecar's HTTP API. The sidecar keeps one session per Zalo account; a
// call is for the account in its context (service.WithZaloAccount), or for the default account when it names none.
type ZaloBridge struct {
	cfg  config.Config
	http *http.Client
	now  func() time.Time // tests replace it

	accounts ZaloAccountSource // nil: the old single account, logged in from the environment

	mu     sync.Mutex // guards the fields below
	logins map[string]*loginState
	defID  string
	defTil time.Time
}

// NewZaloBridge builds a ZaloBridge.
func NewZaloBridge(cfg config.Config) *ZaloBridge {
	return &ZaloBridge{cfg: cfg, http: &http.Client{Timeout: 45 * time.Second}, now: time.Now, logins: map[string]*loginState{}}
}

// SetAccounts gives the bridge the stored accounts: from then on it keeps each of them connected, with its own
// credentials, instead of the one account of the environment.
func (b *ZaloBridge) SetAccounts(src ZaloAccountSource) { b.accounts = src }

// accountID is the account a call is for: the one in its context, else the default account's, else "" (the old
// single account).
func (b *ZaloBridge) accountID(ctx context.Context) string {
	if id := service.ZaloAccountFrom(ctx); id != "" {
		return id
	}
	if b.accounts == nil {
		return ""
	}
	b.mu.Lock()
	if b.defID != "" && b.now().Before(b.defTil) {
		id := b.defID
		b.mu.Unlock()
		return id
	}
	b.mu.Unlock()
	a, err := b.accounts.Default(ctx)
	if err != nil || a == nil {
		return ""
	}
	b.mu.Lock()
	b.defID, b.defTil = a.ID.Hex(), b.now().Add(defaultAccountTTL)
	b.mu.Unlock()
	return a.ID.Hex()
}

// login returns the automatic-login memory of an account. The caller holds b.mu.
func (b *ZaloBridge) login(id string) *loginState {
	st := b.logins[id]
	if st == nil {
		st = &loginState{}
		b.logins[id] = st
	}
	return st
}

// remember records that a person disconnected the account (or logged it in again), also in the database so that a
// restart keeps it that way.
func (b *ZaloBridge) remember(ctx context.Context, id string, disconnected bool) {
	b.mu.Lock()
	b.login(id).disconnectedByUser = disconnected
	b.mu.Unlock()
	if b.accounts == nil {
		return
	}
	if accID, err := domain.ParseID(id); err == nil {
		if err := b.accounts.SetDisabled(ctx, accID, disconnected); err != nil {
			log.Printf("zalo account %s: could not store the disconnect: %v", id, err)
		}
	}
}

// EnvCredentials returns credentials from the environment, or nil when incomplete.
func (b *ZaloBridge) EnvCredentials() *service.ZaloCredentials {
	if b.cfg.ZaloPersonalCookie == "" || b.cfg.ZaloPersonalIMEI == "" || b.cfg.ZaloPersonalUserAgent == "" {
		return nil
	}
	return &service.ZaloCredentials{Cookie: json.RawMessage(b.cfg.ZaloPersonalCookie), IMEI: b.cfg.ZaloPersonalIMEI, UserAgent: b.cfg.ZaloPersonalUserAgent}
}

type bridgeState struct {
	State   string `json:"state"`
	QRImage string `json:"qrImage"`
	Error   string `json:"error"`
	offline bool
}

const bridgeOfflineMsg = "Không kết nối được dịch vụ Zalo (zalo-bridge). Hãy chạy `npm run dev` ở thư mục gốc hoặc kiểm tra container zalo-bridge."

func offline(err error) bridgeState {
	return bridgeState{State: "disconnected", Error: bridgeOfflineMsg + " (" + err.Error() + ")", offline: true}
}

func (b *ZaloBridge) status(s bridgeState) service.ZaloStatus {
	return service.ZaloStatus{
		Configured: b.EnvCredentials() != nil,
		Connected:  s.State == "connected",
		State:      s.State, QRImage: s.QRImage, Error: s.Error,
		BridgeOnline: !s.offline,
	}
}

func (b *ZaloBridge) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(b.cfg.ZaloBridgeURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bridge-Secret", b.cfg.ZaloBridgeSecret)
	if id := b.accountID(ctx); id != "" {
		req.Header.Set("X-Zalo-Account", id)
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return fmt.Errorf("zalo-bridge không phản hồi: %w: %w", service.ErrZaloUnreachable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = fmt.Sprintf("zalo-bridge HTTP %d", resp.StatusCode)
		}
		// the bridge answers 500 for anything zca-js throws, also after Zalo took the message: only a clear refusal (4xx)
		// means "not sent"
		if resp.StatusCode >= 500 {
			return fmt.Errorf("%s: %w", e.Message, service.ErrZaloUnreachable)
		}
		return errors.New(e.Message)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (b *ZaloBridge) fetchState(ctx context.Context, method, path string, in any) (bridgeState, error) {
	var s bridgeState
	err := b.call(ctx, method, path, in, &s)
	return s, err
}

// Status returns the current session state.
func (b *ZaloBridge) Status(ctx context.Context) service.ZaloStatus {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	s, err := b.fetchState(ctx, http.MethodGet, "/status", nil)
	if err != nil {
		return b.status(offline(err))
	}
	return b.status(s)
}

// Connect logs in with cookie/IMEI/user-agent credentials.
func (b *ZaloBridge) Connect(ctx context.Context, creds service.ZaloCredentials) (service.ZaloStatus, error) {
	s, err := b.fetchState(ctx, http.MethodPost, "/connect", creds)
	if err != nil {
		return b.status(bridgeState{State: "error", Error: err.Error()}), err
	}
	b.remember(ctx, b.accountID(ctx), false)
	return b.status(s), nil
}

// StartQR begins a QR-code login (poll Status for the image and progress).
func (b *ZaloBridge) StartQR(ctx context.Context) service.ZaloStatus {
	s, err := b.fetchState(ctx, http.MethodPost, "/qr/start", map[string]string{"userAgent": b.cfg.ZaloPersonalUserAgent})
	if err != nil {
		return b.status(offline(err))
	}
	b.remember(ctx, b.accountID(ctx), false)
	return b.status(s)
}

// Disconnect closes the session of the account.
func (b *ZaloBridge) Disconnect(ctx context.Context) service.ZaloStatus {
	b.remember(ctx, b.accountID(ctx), true)
	s, err := b.fetchState(ctx, http.MethodPost, "/disconnect", nil)
	if err != nil {
		return b.status(bridgeState{State: "disconnected", Error: err.Error()})
	}
	return b.status(s)
}

// AccountStatuses returns the live status of every session the bridge holds, by account id, in one call. The second
// result is false when the bridge cannot be reached: an empty map then does not mean "nobody is logged in".
func (b *ZaloBridge) AccountStatuses(ctx context.Context) (map[string]service.ZaloStatus, bool) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var raw map[string]bridgeState
	if err := b.call(ctx, http.MethodGet, "/accounts", nil, &raw); err != nil {
		return nil, false
	}
	out := make(map[string]service.ZaloStatus, len(raw))
	for id, st := range raw {
		out[id] = b.status(st)
	}
	return out, true
}

// Forget makes the bridge drop the session of the account of ctx (used when the account is removed).
func (b *ZaloBridge) Forget(ctx context.Context) error {
	return b.call(ctx, http.MethodPost, "/forget", map[string]string{}, nil)
}

// IsConnected reports whether the personal account is logged in.
func (b *ZaloBridge) IsConnected(ctx context.Context) bool { return b.Status(ctx).Connected }

var phoneStripRe = regexp.MustCompile(`[\s()-]`)

// ResolveByPhone finds the Zalo account registered with a phone number.
func (b *ZaloBridge) ResolveByPhone(ctx context.Context, phone string) (*service.ZaloUser, error) {
	if !b.IsConnected(ctx) {
		return nil, errors.New("Zalo cá nhân chưa kết nối.")
	}
	normalized := strings.TrimPrefix(phoneStripRe.ReplaceAllString(phone, ""), "+")
	if normalized == "" {
		return nil, errors.New("Thiếu số điện thoại Zalo.")
	}
	var user service.ZaloUser
	if err := b.call(ctx, http.MethodPost, "/find-user", map[string]string{"phone": normalized}, &user); err != nil {
		return nil, err
	}
	if user.UserID == "" {
		return nil, errors.New("Không tìm thấy tài khoản Zalo với số điện thoại này.")
	}
	return &user, nil
}

// Send delivers a text message to a Zalo user.
func (b *ZaloBridge) Send(ctx context.Context, userID, text string) error {
	if !b.IsConnected(ctx) {
		return errors.New("Zalo cá nhân chưa kết nối.")
	}
	return b.call(ctx, http.MethodPost, "/send", map[string]string{"userId": userID, "text": text}, nil)
}

// SendImage delivers one picture to a Zalo user.
func (b *ZaloBridge) SendImage(ctx context.Context, userID string, img service.ZaloImage) error {
	if !b.IsConnected(ctx) {
		return errors.New("Zalo cá nhân chưa kết nối.")
	}
	return b.call(ctx, http.MethodPost, "/send-image", map[string]any{
		"userId": userID, "filename": img.Filename, "dataBase64": base64.StdEncoding.EncodeToString(img.Data),
		"width": img.Width, "height": img.Height, "caption": img.Caption,
	}, nil)
}

// FriendStatus tells whether the user is already a friend, and whether a request is open either way.
func (b *ZaloBridge) FriendStatus(ctx context.Context, userID string) (service.ZaloFriend, error) {
	var out service.ZaloFriend
	if !b.IsConnected(ctx) {
		return out, errors.New("Zalo cá nhân chưa kết nối.")
	}
	err := b.call(ctx, http.MethodPost, "/friend-status", map[string]string{"userId": userID}, &out)
	return out, err
}

// SendFriendRequest asks the user to become a friend, with a short note.
func (b *ZaloBridge) SendFriendRequest(ctx context.Context, userID, note string) error {
	if !b.IsConnected(ctx) {
		return errors.New("Zalo cá nhân chưa kết nối.")
	}
	return b.call(ctx, http.MethodPost, "/friend-request", map[string]string{"userId": userID, "message": note}, nil)
}

// React puts a reaction on one message of the contact ("" takes it back).
func (b *ZaloBridge) React(ctx context.Context, userID, msgID, cliMsgID, code string) error {
	if !b.IsConnected(ctx) {
		return errors.New("Zalo cá nhân chưa kết nối.")
	}
	return b.call(ctx, http.MethodPost, "/react", map[string]string{"userId": userID, "msgId": msgID, "cliMsgId": cliMsgID, "icon": code}, nil)
}

// InitializeFromEnv connects the personal account at startup when credentials are configured.
func (b *ZaloBridge) InitializeFromEnv(ctx context.Context) {
	creds := b.EnvCredentials()
	if creds == nil {
		return
	}
	if _, err := b.Connect(ctx, *creds); err != nil {
		log.Printf("Unable to connect personal Zalo: %v", err)
	}
}

// How long the automatic login waits after the bridge refused it: it doubles each time, up to the ceiling. A cookie that
// Zalo no longer accepts would otherwise be sent every few seconds, which looks like an attack on the account.
const (
	reconnectBackoffMin = time.Minute
	reconnectBackoffMax = 15 * time.Minute
)

// EnsureConnected logs every account in again when the bridge has lost its session: the bridge keeps nothing on
// disk, so after it restarts (a deploy, a crash, a reboot of its machine) it is disconnected until somebody logs in.
// For each account it does nothing when the account is connected, when somebody is logging in (QR), when the person
// disconnected it on purpose, when it has no credentials (it waits for its QR code), or when the bridge cannot be
// reached (nobody to log in to). One account that Zalo refuses never holds back the others: each has its own wait.
func (b *ZaloBridge) EnsureConnected(ctx context.Context) {
	if b.accounts != nil {
		if all, err := b.accounts.List(ctx); err == nil && len(all) > 0 {
			for _, a := range all {
				if !a.HasCredentials() || a.Disabled {
					continue
				}
				id := a.ID
				b.ensureOne(service.WithZaloAccount(ctx, id.Hex()), id.Hex(), func() (*service.ZaloCredentials, error) {
					return b.accounts.Credentials(ctx, id)
				})
			}
			return
		}
	}
	if creds := b.EnvCredentials(); creds != nil { // no stored accounts: the old single account of the environment
		b.ensureOne(ctx, "", func() (*service.ZaloCredentials, error) { return creds, nil })
	}
}

func (b *ZaloBridge) ensureOne(ctx context.Context, id string, load func() (*service.ZaloCredentials, error)) {
	name := id
	if name == "" {
		name = "default"
	}
	b.mu.Lock()
	st := b.login(id)
	skip := st.disconnectedByUser || b.now().Before(st.retryAt)
	b.mu.Unlock()
	if skip {
		return
	}
	cur := b.Status(ctx)
	if !cur.BridgeOnline || cur.Connected || (cur.State != "disconnected" && cur.State != "error") {
		return
	}
	creds, err := load()
	if err != nil {
		log.Printf("zalo account %s: cannot read its credentials: %v", name, err)
		return
	}
	_, err = b.Connect(ctx, *creds)
	b.mu.Lock()
	defer b.mu.Unlock()
	st = b.login(id)
	if err != nil {
		st.failures++
		wait := reconnectBackoffMin << (st.failures - 1)
		if wait > reconnectBackoffMax || wait <= 0 {
			wait = reconnectBackoffMax
		}
		st.retryAt = b.now().Add(wait)
		log.Printf("Unable to connect Zalo account %s (attempt %d, next one in %s): %v", name, st.failures, wait, err)
		return
	}
	if st.failures > 0 {
		log.Printf("Zalo account %s is connected again after %d refused attempts", name, st.failures)
	} else {
		log.Printf("Zalo account %s connected", name)
	}
	st.failures, st.retryAt = 0, time.Time{}
}

// KeepConnected calls EnsureConnected now and then every interval, until ctx ends. It replaces the single login at
// startup, so that the bridge may run on another machine and restart on its own.
func (b *ZaloBridge) KeepConnected(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		b.EnsureConnected(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// EnsureBridgeProcess auto-spawns the local node sidecar if not already running.
func (b *ZaloBridge) EnsureBridgeProcess(ctx context.Context) func() {
	cleanURL := strings.TrimRight(b.cfg.ZaloBridgeURL, "/")
	if !strings.Contains(cleanURL, "127.0.0.1") && !strings.Contains(cleanURL, "localhost") {
		return func() {}
	}

	// 1. Check if sidecar is already healthy
	checkCtx, checkCancel := context.WithTimeout(ctx, 1*time.Second)
	req, _ := http.NewRequestWithContext(checkCtx, http.MethodGet, cleanURL+"/health", nil)
	resp, err := b.http.Do(req)
	checkCancel()
	if err == nil && resp != nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return func() {}
		}
	}

	// 2. Discover zalo-bridge folder relative to current working dir
	candidates := []string{
		"zalo-bridge",
		filepath.Join("..", "zalo-bridge"),
		filepath.Join("..", "..", "zalo-bridge"),
	}
	var bridgeDir string
	for _, c := range candidates {
		if fi, err := os.Stat(filepath.Join(c, "src", "server.js")); err == nil && !fi.IsDir() {
			bridgeDir = c
			break
		}
	}
	if bridgeDir == "" {
		return func() {}
	}

	cmd := exec.CommandContext(ctx, "node", "src/server.js")
	cmd.Dir = bridgeDir
	cmd.Env = append(os.Environ(),
		"PORT=5100",
		"BRIDGE_SECRET="+b.cfg.ZaloBridgeSecret,
		"API_URL=http://127.0.0.1:"+b.cfg.Port,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("[zalo-bridge auto-spawn] Không thể tự động khởi động: %v", err)
		return func() {}
	}
	log.Printf("[zalo-bridge auto-spawn] Đã tự động kích hoạt Zalo Bridge song song (PID %d)...", cmd.Process.Pid)

	// Poll until ready then initialize credentials if present
	go func() {
		for i := 0; i < 20; i++ {
			time.Sleep(250 * time.Millisecond)
			if pReq, err := http.NewRequest(http.MethodGet, cleanURL+"/health", nil); err == nil {
				if pResp, err := b.http.Do(pReq); err == nil && pResp != nil {
					pResp.Body.Close()
					if pResp.StatusCode == 200 {
						log.Printf("[zalo-bridge auto-spawn] Zalo Bridge đã sẵn sàng tại %s", cleanURL)
						b.InitializeFromEnv(context.Background())
						return
					}
				}
			}
		}
	}()

	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}
