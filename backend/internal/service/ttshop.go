package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

const (
	ttsRefreshMargin  = 5 * time.Minute
	ttsMaxResponseLen = 2 << 20
)

// ErrTTShopNotConnected is returned when no shop has authorized the app yet (or the grant expired).
var ErrTTShopNotConnected = errors.New("Chưa ủy quyền shop cho ứng dụng TikTok Shop")

// TTShopInfo is one shop that authorized the app.
type TTShopInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Region     string `json:"region"`
	SellerType string `json:"sellerType"`
	Cipher     string `json:"-"` // shop_cipher: identifies the shop in API calls, never sent to the browser
}

// TTShopStatus is what the admin page needs to show the connection state.
type TTShopStatus struct {
	Configured     bool         `json:"configured"`
	Env            string       `json:"env"`
	Connected      bool         `json:"connected"`
	SellerName     string       `json:"sellerName,omitempty"`
	AccessExpireAt *time.Time   `json:"accessExpireAt,omitempty"`
	Shops          []TTShopInfo `json:"shops"`
	Error          string       `json:"error,omitempty"`
}

// TokenStore keeps the shop's TikTok authorization. The service owns this interface; the repository package
// implements it, so the service does not know MongoDB exists (and tests can use an in-memory fake).
type TokenStore interface {
	// Load returns the stored authorization, or (nil, nil) when none was stored yet.
	Load(ctx context.Context) (*domain.TTSToken, error)
	Save(ctx context.Context, t *domain.TTSToken) error
}

type ttsEnvelope struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

type ttsTokenData struct {
	AccessToken          string `json:"access_token"`
	AccessTokenExpireIn  int64  `json:"access_token_expire_in"`
	RefreshToken         string `json:"refresh_token"`
	RefreshTokenExpireIn int64  `json:"refresh_token_expire_in"`
	OpenID               string `json:"open_id"`
	SellerName           string `json:"seller_name"`
}

// TTShopService is a read-only client of the TikTok Shop Open Platform for the one shop that authorized the app.
// It only calls GET endpoints and POST ".../search" endpoints (which only read), plus the one POST that creates an
// affiliate promotion link ("/promotion_link/generate", see postGenerate); it never edits or deletes anything
// in the shop.
type TTShopService struct {
	cfg    config.Config
	http   *http.Client
	tokens TokenStore

	cipherMu sync.Mutex
	cipher   string
	region   string // market of the authorized shop, e.g. VN
	cipherAt time.Time

	promoMu      sync.Mutex
	promo        map[string][]promoEntry // product id -> ongoing activities that set a price for it
	promoAt      time.Time
	skuCacheM    sync.Mutex
	creatorMu    sync.Mutex
	creatorCache map[string]creatorCacheEntry
	skuCache     map[string]skuPrices

	contentState // cache of the affiliate orders and video reads (ttshop_content.go)

	targets targetListCache // invitation lists kept for a short time (ttshop_target_cache.go)

	pace pacer // spacing and cool-down of the calls we send (ttshop_pacer.go)

	logs *ttsLog
}

// NewTTShopService builds a TTShopService that keeps the shop's authorization in the given store.
func NewTTShopService(cfg config.Config, tokens TokenStore) *TTShopService {
	return &TTShopService{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}, tokens: tokens, logs: newTTSLog(cfg.TTSLogFile)}
}

// Configured reports whether the app key and secret are set.
func (s *TTShopService) Configured() bool {
	return s.cfg.TTSAppKey != "" && s.cfg.TTSAppSecret != ""
}

// sign computes the Open Platform request signature: HMAC-SHA256 (key = app secret) over
// secret + path + sorted key/value query pairs (without sign and access_token) + body + secret.
func (s *TTShopService) sign(path string, q url.Values, body []byte) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		if k == "sign" || k == "access_token" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(s.cfg.TTSAppSecret)
	b.WriteString(path)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(q.Get(k))
	}
	b.Write(body)
	b.WriteString(s.cfg.TTSAppSecret)

	mac := hmac.New(sha256.New, []byte(s.cfg.TTSAppSecret))
	mac.Write([]byte(b.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *TTShopService) key() []byte {
	h := sha256.Sum256([]byte("tts:" + s.cfg.JWTSecret))
	return h[:]
}

func (s *TTShopService) seal(plain string) (string, error) {
	block, err := aes.NewCipher(s.key())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (s *TTShopService) open(sealed string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", errors.New("token không hợp lệ")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), err
}

// TTSRejected is TikTok Shop answering with a non-zero code: it received the request and refused it, so nothing was
// done. Any other error (timeout, broken connection) leaves it unknown whether a write took effect.
type TTSRejected struct {
	Code    int
	Message string
}

func (e *TTSRejected) Error() string {
	return fmt.Sprintf("TikTok Shop từ chối (mã %d): %s", e.Code, e.Message)
}

// do runs a request and unwraps the {code,message,data} envelope. Transport errors are reduced to their cause
// because net/http embeds the full URL (including app_secret on the token endpoints) in its error text.
func (s *TTShopService) do(req *http.Request) (json.RawMessage, error) {
	start := time.Now()
	call := req.Method + " " + req.URL.Path
	if q := safeQuery(req.URL.Query()); q != "" {
		call += " q=" + q
	}
	if b := requestBodyForLog(req); b != "" && b != "{}" {
		call += " body=" + b
	}
	resp, err := s.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		s.logs.add(fmt.Sprintf("%s result=transport_error error=%q ms=%d", call, err.Error(), time.Since(start).Milliseconds()))
		return nil, fmt.Errorf("không gọi được TikTok Shop: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, ttsMaxResponseLen))
	if err != nil {
		s.logs.add(fmt.Sprintf("%s http=%d result=read_error ms=%d", call, resp.StatusCode, time.Since(start).Milliseconds()))
		return nil, fmt.Errorf("không đọc được phản hồi TikTok Shop: %w", err)
	}
	var env ttsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		s.logs.add(fmt.Sprintf("%s http=%d result=invalid_json bytes=%d ms=%d", call, resp.StatusCode, len(raw), time.Since(start).Milliseconds()))
		return nil, fmt.Errorf("phản hồi TikTok Shop không hợp lệ (HTTP %d)", resp.StatusCode)
	}
	result := "ok"
	if env.Code != 0 {
		result = "rejected"
	}
	s.logs.add(fmt.Sprintf("%s http=%d code=%d result=%s msg=%q request_id=%s bytes=%d ms=%d",
		call, resp.StatusCode, env.Code, result, env.Message, env.RequestID, len(raw), time.Since(start).Milliseconds()))
	if env.Code != 0 {
		return nil, &TTSRejected{Code: env.Code, Message: env.Message}
	}
	return env.Data, nil
}

// ttsExpiry converts a token expiry field to a time. The auth API returns a unix timestamp; a small value is
// treated as a lifetime in seconds.
func ttsExpiry(v int64) time.Time {
	if v > 1_000_000_000 {
		return time.Unix(v, 0)
	}
	return time.Now().Add(time.Duration(v) * time.Second)
}

func (s *TTShopService) tokenCall(ctx context.Context, path string, extra url.Values) (*ttsTokenData, error) {
	q := url.Values{"app_key": {s.cfg.TTSAppKey}, "app_secret": {s.cfg.TTSAppSecret}}
	for k, v := range extra {
		q[k] = v
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.TTSAuthBaseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	data, err := s.do(req)
	if err != nil {
		return nil, err
	}
	var t ttsTokenData
	if err := json.Unmarshal(data, &t); err != nil || t.AccessToken == "" {
		return nil, errors.New("TikTok Shop không trả về access token")
	}
	return &t, nil
}

func (s *TTShopService) saveToken(ctx context.Context, t *ttsTokenData) error {
	access, err := s.seal(t.AccessToken)
	if err != nil {
		return err
	}
	refresh, err := s.seal(t.RefreshToken)
	if err != nil {
		return err
	}
	doc := &domain.TTSToken{Access: access, Refresh: refresh,
		AccessExpireAt: ttsExpiry(t.AccessTokenExpireIn), RefreshExpireAt: ttsExpiry(t.RefreshTokenExpireIn),
		OpenID: t.OpenID, SellerName: t.SellerName, UpdatedAt: time.Now()}
	return s.tokens.Save(ctx, doc)
}

func (s *TTShopService) loadToken(ctx context.Context) (*domain.TTSToken, error) {
	if s.tokens == nil {
		return nil, ErrTTShopNotConnected
	}
	doc, err := s.tokens.Load(ctx)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, ErrTTShopNotConnected
	}
	return doc, nil
}

// Authorize exchanges the auth_code that TikTok sent to the redirect URL for tokens and stores them.
func (s *TTShopService) Authorize(ctx context.Context, authCode string) error {
	if !s.Configured() {
		return errors.New("Chưa cấu hình TTS_APP_KEY / TTS_APP_SECRET")
	}
	authCode = strings.TrimSpace(authCode)
	if authCode == "" {
		return errors.New("Thiếu auth_code")
	}
	t, err := s.tokenCall(ctx, "/api/v2/token/get", url.Values{"auth_code": {authCode}, "grant_type": {"authorized_code"}})
	if err != nil {
		return err
	}
	return s.saveToken(ctx, t)
}

// accessToken returns a usable access token, refreshing it first when it is about to expire.
func (s *TTShopService) accessToken(ctx context.Context) (string, error) {
	doc, err := s.loadToken(ctx)
	if err != nil {
		return "", err
	}
	if time.Until(doc.AccessExpireAt) > ttsRefreshMargin {
		return s.open(doc.Access)
	}
	if time.Now().After(doc.RefreshExpireAt) {
		return "", ErrTTShopNotConnected
	}
	refresh, err := s.open(doc.Refresh)
	if err != nil {
		return "", err
	}
	t, err := s.tokenCall(ctx, "/api/v2/token/refresh", url.Values{"refresh_token": {refresh}, "grant_type": {"refresh_token"}})
	if err != nil {
		return "", err
	}
	if err := s.saveToken(ctx, t); err != nil {
		return "", err
	}
	return t.AccessToken, nil
}

// get performs a signed GET against the Open Platform API.
func (s *TTShopService) get(ctx context.Context, path string, q url.Values) (json.RawMessage, error) {
	return s.call(ctx, http.MethodGet, path, q, nil)
}

// postSearch performs a signed POST against a ".../search" endpoint. Those endpoints only read data, so they
// keep this client read-only; any other POST path is refused.
func (s *TTShopService) postSearch(ctx context.Context, path string, q url.Values, body any) (json.RawMessage, error) {
	if !strings.HasSuffix(path, "/search") {
		return nil, errors.New("chỉ được gọi các API tìm kiếm (chỉ đọc) bằng POST")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return s.call(ctx, http.MethodPost, path, q, raw)
}

// ttsWriteRoutes are the ONLY calls that change something in the shop or reach creators. Anything else is refused
// before it leaves this process: the app is otherwise read-only (GET and POST ".../search").
var ttsWriteRoutes = []struct {
	method string
	path   *regexp.Regexp
}{
	{http.MethodPost, regexp.MustCompile(`^/affiliate_seller/202508/target_collaborations$`)},                 // create invitation
	{http.MethodPost, regexp.MustCompile(`^/affiliate_seller/202509/target_collaboration/[0-9]{1,30}/link$`)}, // invitation link
	{http.MethodDelete, regexp.MustCompile(`^/affiliate_seller/202409/target_collaborations/[0-9]{1,30}$`)},   // cancel invitation
}

// ttsWriteAllowed reports whether method+path is one of ttsWriteRoutes.
func ttsWriteAllowed(method, path string) bool {
	for _, r := range ttsWriteRoutes {
		if r.method == method && r.path.MatchString(path) {
			return true
		}
	}
	return false
}

// callWrite runs one of ttsWriteRoutes. body is marshalled to JSON (nil sends no body).
func (s *TTShopService) callWrite(ctx context.Context, method, path string, q url.Values, body any) (json.RawMessage, error) {
	if !ttsWriteAllowed(method, path) {
		return nil, errors.New("thao tác ghi này không nằm trong danh sách được phép")
	}
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}
	return s.call(ctx, method, path, q, raw)
}

// postGenerate performs the signed POST that creates an affiliate promotion link for a product. Creating a link does
// not change the product or the shop, and it is the only non-search POST this client accepts.
func (s *TTShopService) postGenerate(ctx context.Context, path string, q url.Values) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/affiliate_seller/") || !strings.HasSuffix(path, "/promotion_link/generate") {
		return nil, errors.New("chỉ được gọi API tạo link khuyến mại bằng POST ngoài các API tìm kiếm")
	}
	return s.call(ctx, http.MethodPost, path, q, []byte("{}"))
}

func (s *TTShopService) call(ctx context.Context, method, path string, q url.Values, body []byte) (json.RawMessage, error) {
	data, err := s.callOnce(ctx, method, path, q, body)
	if !IsRateLimited(err) || errors.As(err, new(*ErrRateLimited)) {
		return data, err
	}
	// TikTok said "too many requests". A read changes nothing, so one more try after a short wait is safe; a write is
	// never repeated (it might have gone through). If it still fails, every call pauses for a while.
	if method == http.MethodGet || strings.HasSuffix(path, "/search") {
		t := time.NewTimer(ttsRetryDelay)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if data, err = s.callOnce(ctx, method, path, q, body); !IsRateLimited(err) {
			return data, err
		}
	}
	s.pace.penalize(ttsRateLimitPause)
	return nil, err
}

func (s *TTShopService) callOnce(ctx context.Context, method, path string, q url.Values, body []byte) (json.RawMessage, error) {
	if err := s.pace.wait(ctx); err != nil {
		return nil, err
	}
	token, err := s.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("app_key", s.cfg.TTSAppKey)
	q.Set("timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	q.Set("sign", s.sign(path, q, body))
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.cfg.TTSAPIBaseURL+path+"?"+q.Encode(), rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-tts-access-token", token)
	req.Header.Set("Content-Type", "application/json")
	return s.do(req)
}

// AuthorizedShops lists the shops that authorized the app (GET /authorization/202309/shops, read-only).
func (s *TTShopService) AuthorizedShops(ctx context.Context) ([]TTShopInfo, error) {
	data, err := s.get(ctx, "/authorization/202309/shops", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Shops []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Region     string `json:"region"`
			SellerType string `json:"seller_type"`
			Cipher     string `json:"cipher"`
		} `json:"shops"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errors.New("không đọc được danh sách shop")
	}
	shops := make([]TTShopInfo, 0, len(out.Shops))
	for _, sh := range out.Shops {
		shops = append(shops, TTShopInfo{ID: sh.ID, Name: sh.Name, Region: sh.Region, SellerType: sh.SellerType, Cipher: sh.Cipher})
	}
	return shops, nil
}

// Status reports configuration, whether a shop authorized the app, and the shops visible to it.
func (s *TTShopService) Status(ctx context.Context) TTShopStatus {
	st := TTShopStatus{Configured: s.Configured(), Env: s.cfg.TTSEnv, Shops: []TTShopInfo{}}
	if !st.Configured {
		return st
	}
	doc, err := s.loadToken(ctx)
	if err != nil {
		if !errors.Is(err, ErrTTShopNotConnected) {
			st.Error = err.Error()
		}
		return st
	}
	st.Connected = true
	st.SellerName = doc.SellerName
	st.AccessExpireAt = &doc.AccessExpireAt
	shops, err := s.AuthorizedShops(ctx)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Shops = shops
	return st
}
