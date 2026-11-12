package platform

import (
	"bytes"
	"context"
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
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// ZaloBridge implements PersonalZalo over the sidecar's HTTP API.
type ZaloBridge struct {
	cfg  config.Config
	http *http.Client
}

// NewZaloBridge builds a ZaloBridge.
func NewZaloBridge(cfg config.Config) *ZaloBridge {
	return &ZaloBridge{cfg: cfg, http: &http.Client{Timeout: 45 * time.Second}}
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
	resp, err := b.http.Do(req)
	if err != nil {
		return fmt.Errorf("zalo-bridge không phản hồi: %w", err)
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
	return b.status(s), nil
}

// StartQR begins a QR-code login (poll Status for the image and progress).
func (b *ZaloBridge) StartQR(ctx context.Context) service.ZaloStatus {
	s, err := b.fetchState(ctx, http.MethodPost, "/qr/start", map[string]string{"userAgent": b.cfg.ZaloPersonalUserAgent})
	if err != nil {
		return b.status(offline(err))
	}
	return b.status(s)
}

// Disconnect closes the session.
func (b *ZaloBridge) Disconnect(ctx context.Context) service.ZaloStatus {
	s, err := b.fetchState(ctx, http.MethodPost, "/disconnect", nil)
	if err != nil {
		return b.status(bridgeState{State: "disconnected", Error: err.Error()})
	}
	return b.status(s)
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
