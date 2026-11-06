package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

const (
	gmailScopes   = "https://www.googleapis.com/auth/gmail.send https://www.googleapis.com/auth/gmail.readonly https://www.googleapis.com/auth/userinfo.email"
	googleAuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
	googleToken   = "https://oauth2.googleapis.com/token"
	gmailAPI      = "https://gmail.googleapis.com/gmail/v1/users/me"
	// KocMailSubject is the subject of every mail sent from the inbox.
	KocMailSubject = "Bạn có liên hệ từ Tiktoday Booking"
)

// ErrGmailNotConnected is returned when the user has no usable Gmail account.
var ErrGmailNotConnected = errors.New("Chưa kết nối tài khoản Gmail")

// ErrGmailBroken is returned when Google rejected the stored token.
var ErrGmailBroken = errors.New("Tài khoản Gmail đã mất kết nối")

// InboundMail is a received email reduced to what the chat needs.
type InboundMail struct {
	ID      string
	From    string
	Subject string
	Text    string
	At      time.Time
}

// GmailAccountStore keeps the linked mailboxes. The service owns this interface and the repository package
// implements it, so the service does not know which database is behind it.
type GmailAccountStore interface {
	Save(ctx context.Context, a *domain.GmailAccount) error
	// Find returns the user's mailbox, or (nil, nil) when none is linked.
	Find(ctx context.Context, userID string) (*domain.GmailAccount, error)
	Delete(ctx context.Context, userID string) error
	ListHealthy(ctx context.Context) ([]domain.GmailAccount, error)
	MarkBroken(ctx context.Context, userID string) error
}

// GmailService sends and receives mail through the Gmail REST API with per-user OAuth.
type GmailService struct {
	cfg      config.Config
	http     *http.Client
	accounts GmailAccountStore
}

// NewGmailService builds a GmailService that keeps linked mailboxes in the given store.
func NewGmailService(cfg config.Config, accounts GmailAccountStore) *GmailService {
	return &GmailService{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second}, accounts: accounts}
}

// Configured reports whether Google OAuth credentials are set.
func (g *GmailService) Configured() bool {
	return g.cfg.GoogleClientID != "" && g.cfg.GoogleClientSecret != ""
}

func (g *GmailService) key() []byte {
	s := sha256.Sum256([]byte("gmail:" + g.cfg.JWTSecret))
	return s[:]
}

func (g *GmailService) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(g.key())
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (g *GmailService) seal(plain string) (string, error) {
	gcm, err := g.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (g *GmailService) open(sealed string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	gcm, err := g.gcm()
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", errors.New("token không hợp lệ")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), err
}

// SignState returns a short-lived signed OAuth state bound to a user.
func (g *GmailService) SignState(userID string) string {
	payload := userID + "|" + strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)
	mac := hmac.New(sha256.New, g.key())
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyState returns the user id carried by a valid, unexpired state.
func (g *GmailService) VerifyState(state string) (string, bool) {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	sig, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, g.key())
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	f := strings.SplitN(string(payload), "|", 2)
	if len(f) != 2 {
		return "", false
	}
	exp, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return f[0], true
}

// AuthURL builds the Google consent URL.
func (g *GmailService) AuthURL(userID string) string {
	q := url.Values{
		"client_id": {g.cfg.GoogleClientID}, "redirect_uri": {g.cfg.GoogleRedirectURL}, "response_type": {"code"},
		"scope": {gmailScopes}, "access_type": {"offline"}, "prompt": {"consent"}, "state": {g.SignState(userID)},
	}
	return googleAuthURL + "?" + q.Encode()
}

func (g *GmailService) tokenRequest(ctx context.Context, form url.Values) (map[string]any, error) {
	form.Set("client_id", g.cfg.GoogleClientID)
	form.Set("client_secret", g.cfg.GoogleClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleToken, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		if out["error"] == "invalid_grant" {
			return nil, ErrGmailBroken
		}
		return nil, fmt.Errorf("Google từ chối yêu cầu (%d)", resp.StatusCode)
	}
	return out, nil
}

// Connect exchanges the OAuth code and stores the refresh token for the user.
func (g *GmailService) Connect(ctx context.Context, userID, code string) (string, error) {
	tok, err := g.tokenRequest(ctx, url.Values{"code": {code}, "grant_type": {"authorization_code"}, "redirect_uri": {g.cfg.GoogleRedirectURL}})
	if err != nil {
		return "", err
	}
	refresh, _ := tok["refresh_token"].(string)
	access, _ := tok["access_token"].(string)
	if refresh == "" || access == "" {
		return "", errors.New("Google không trả refresh token. Hãy thu hồi quyền và kết nối lại.")
	}
	var info struct {
		Email string `json:"email"`
	}
	if err := g.getJSON(ctx, access, "https://www.googleapis.com/oauth2/v2/userinfo", &info); err != nil {
		return "", err
	}
	sealed, err := g.seal(refresh)
	if err != nil {
		return "", err
	}
	acct := &domain.GmailAccount{UserID: userID, Email: info.Email, RefreshToken: sealed, UpdatedAt: time.Now()}
	return info.Email, g.accounts.Save(ctx, acct)
}

// Account returns the user's linked account, or (nil, nil).
func (g *GmailService) Account(ctx context.Context, userID string) (*domain.GmailAccount, error) {
	return g.accounts.Find(ctx, userID)
}

// Disconnect removes the user's linked account.
func (g *GmailService) Disconnect(ctx context.Context, userID string) error {
	return g.accounts.Delete(ctx, userID)
}

// Linked lists every healthy linked account (for the reply poller).
func (g *GmailService) Linked(ctx context.Context) ([]domain.GmailAccount, error) {
	return g.accounts.ListHealthy(ctx)
}

func (g *GmailService) accessToken(ctx context.Context, userID string) (string, *domain.GmailAccount, error) {
	acct, err := g.Account(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	if acct == nil {
		return "", nil, ErrGmailNotConnected
	}
	if acct.Broken {
		return "", acct, ErrGmailBroken
	}
	refresh, err := g.open(acct.RefreshToken)
	if err != nil {
		return "", acct, ErrGmailBroken
	}
	tok, err := g.tokenRequest(ctx, url.Values{"refresh_token": {refresh}, "grant_type": {"refresh_token"}})
	if errors.Is(err, ErrGmailBroken) {
		_ = g.accounts.MarkBroken(ctx, userID)
		return "", acct, ErrGmailBroken
	}
	if err != nil {
		return "", acct, err
	}
	access, _ := tok["access_token"].(string)
	return access, acct, nil
}

func (g *GmailService) getJSON(ctx context.Context, access, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Gmail trả lỗi %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// BuildRawMail builds a base64url RFC 5322 message with a UTF-8 subject and body.
func BuildRawMail(from, to, subject, text string) string {
	h := "From: " + from + "\r\nTo: " + to + "\r\nSubject: " + mime.BEncoding.Encode("UTF-8", subject) +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n"
	enc := base64.StdEncoding.EncodeToString([]byte(text))
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc)
	return base64.URLEncoding.EncodeToString([]byte(h + b.String()))
}

// Send mails text from the user's Gmail to one address and returns the Gmail message id.
func (g *GmailService) Send(ctx context.Context, userID, to, subject, text string) (string, error) {
	if _, err := mail.ParseAddress(to); err != nil {
		return "", errors.New("Địa chỉ email không hợp lệ")
	}
	access, acct, err := g.accessToken(ctx, userID)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"raw": BuildRawMail(acct.Email, to, subject, text)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gmailAPI+"/messages/send", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gmail không gửi được thư (%d)", resp.StatusCode)
	}
	return out.ID, nil
}

type gmailPart struct {
	MimeType string `json:"mimeType"`
	Body     struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

func plainText(p gmailPart) string {
	if p.MimeType == "text/plain" && p.Body.Data != "" {
		if b, err := base64.URLEncoding.DecodeString(p.Body.Data); err == nil {
			return string(b)
		}
		if b, err := base64.RawURLEncoding.DecodeString(p.Body.Data); err == nil {
			return string(b)
		}
	}
	for _, c := range p.Parts {
		if t := plainText(c); t != "" {
			return t
		}
	}
	return ""
}

// FetchInbound returns recent received mails (last 2 days, not sent by the user).
func (g *GmailService) FetchInbound(ctx context.Context, userID string) ([]InboundMail, error) {
	access, acct, err := g.accessToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	var list struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	q := url.Values{"q": {"in:inbox newer_than:2d -from:" + acct.Email}, "maxResults": {"30"}}
	if err := g.getJSON(ctx, access, gmailAPI+"/messages?"+q.Encode(), &list); err != nil {
		return nil, err
	}
	out := make([]InboundMail, 0, len(list.Messages))
	for _, m := range list.Messages {
		var full struct {
			ID           string `json:"id"`
			InternalDate string `json:"internalDate"`
			Snippet      string `json:"snippet"`
			Payload      struct {
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
				MimeType string `json:"mimeType"`
				Body     struct {
					Data string `json:"data"`
				} `json:"body"`
				Parts []gmailPart `json:"parts"`
			} `json:"payload"`
		}
		if err := g.getJSON(ctx, access, gmailAPI+"/messages/"+m.ID+"?format=full", &full); err != nil {
			continue
		}
		root := gmailPart{MimeType: full.Payload.MimeType, Parts: full.Payload.Parts}
		root.Body.Data = full.Payload.Body.Data
		im := InboundMail{ID: full.ID, At: time.Now(), Text: strings.TrimSpace(plainText(root))}
		if ms, err := strconv.ParseInt(full.InternalDate, 10, 64); err == nil {
			im.At = time.UnixMilli(ms)
		}
		if im.Text == "" {
			im.Text = full.Snippet
		}
		for _, h := range full.Payload.Headers {
			switch strings.ToLower(h.Name) {
			case "from":
				if a, err := mail.ParseAddress(h.Value); err == nil {
					im.From = strings.ToLower(a.Address)
				}
			case "subject":
				im.Subject = h.Value
			}
		}
		if im.From != "" {
			out = append(out, im)
		}
	}
	return out, nil
}
