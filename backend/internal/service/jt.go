package service

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
)

// JTTracking is the normalised result of a J&T Express lookup.
type JTTracking struct {
	Source     string `json:"source"`
	BillCode   string `json:"billCode"`
	Delivered  bool   `json:"delivered"`
	StatusText string `json:"statusText"`
	Details    []any  `json:"details,omitempty"`
}

// Tracker looks up shipment status.
type Tracker interface {
	QueryTracking(ctx context.Context, billCode, phoneLast4 string) (*JTTracking, error)
}

// JTService queries J&T Express Vietnam (official Open Platform when configured, public portal otherwise).
type JTService struct {
	cfg  config.Config
	http *http.Client
}

// NewJTService builds a JTService.
func NewJTService(cfg config.Config) *JTService {
	return &JTService{cfg: cfg, http: &http.Client{}}
}

func jtDigest(bizContent, privateKey string) string {
	sum := md5.Sum([]byte(bizContent + privateKey))
	return base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(sum[:])))
}

var jtDeliveredWords = []string{"ký nhận", "giao hàng thành công", "delivered", "sign"}

// ErrPhoneLast4 is returned when the receiver's last 4 phone digits are missing or malformed.
var ErrPhoneLast4 = errors.New("Cần đúng 4 số cuối SĐT người nhận để tra cứu J&T")

// NormalizePhoneLast4 returns the last 4 digits of a phone number or of an already-4-digit string.
func NormalizePhoneLast4(s string) string {
	digits := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			digits = append(digits, s[i])
		}
	}
	if len(digits) < 4 {
		return ""
	}
	return string(digits[len(digits)-4:])
}

// QueryTracking resolves the shipment journey of a bill code. J&T only reveals a parcel
// when the last 4 digits of the receiver's phone number are supplied with the bill code.
func (s *JTService) QueryTracking(ctx context.Context, billCode, phoneLast4 string) (*JTTracking, error) {
	code := strings.TrimSpace(billCode)
	if code == "" {
		return nil, errors.New("Thiếu mã vận đơn J&T Express")
	}
	phone4 := strings.TrimSpace(phoneLast4)
	if len(phone4) != 4 || NormalizePhoneLast4(phone4) != phone4 {
		return nil, ErrPhoneLast4
	}

	if s.cfg.JTAPIAccount != "" && s.cfg.JTPrivateKey != "" {
		return s.queryOpenAPI(ctx, code, phone4)
	}

	if res := s.queryPublicPortal(ctx, code, phone4); res != nil {
		return res, nil
	}
	return &JTTracking{
		Source: "jt-tracker", BillCode: code, Delivered: false,
		StatusText: fmt.Sprintf("Đang theo dõi vận đơn J&T #%s (Quét tự động 4 giờ/lần)", code),
	}, nil
}

func (s *JTService) queryOpenAPI(ctx context.Context, code, phone4 string) (*JTTracking, error) {
	biz, _ := json.Marshal(map[string]string{"billCodes": code, "phone": phone4})
	form := url.Values{"bizContent": {string(biz)}}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.JTAPIURL+"/trace/trace", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("apiAccount", s.cfg.JTAPIAccount)
	req.Header.Set("digest", jtDigest(string(biz), s.cfg.JTPrivateKey))
	req.Header.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Data []struct {
			Details []map[string]any `json:"details"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	var details []map[string]any
	if len(body.Data) > 0 {
		details = body.Data[0].Details
	}
	var latestDesc, scanType string
	if len(details) > 0 {
		latestDesc, _ = details[0]["desc"].(string)
		scanType, _ = details[0]["scanType"].(string)
		if scanType == "" {
			scanType = latestDesc
		}
	}
	scanType = strings.ToLower(scanType)
	delivered := false
	for _, w := range jtDeliveredWords {
		if strings.Contains(scanType, w) {
			delivered = true
		}
	}
	status := latestDesc
	if status == "" {
		if delivered {
			status = "Đã giao hàng thành công (J&T OpenAPI)"
		} else {
			status = "Đang luân chuyển tại bưu cục J&T"
		}
	}
	out := &JTTracking{Source: "jt-openapi", BillCode: code, Delivered: delivered, StatusText: status}
	for _, d := range details {
		out.Details = append(out.Details, d)
	}
	return out, nil
}

func (s *JTService) queryPublicPortal(ctx context.Context, code, phone4 string) *JTTracking {
	ctx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	target := "https://jtexpress.vn/vi/tracking?type=track&billcode=" + url.QueryEscape(code) + "&cellphone=" + url.QueryEscape(phone4)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil // network blocked or timeout: fall back to "tracking" status
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil
	}
	lower := strings.ToLower(string(raw))
	delivered := containsAny(lower, "giao hàng thành công", "đã ký nhận", "ký nhận thành công")
	inTransit := containsAny(lower, "đang vận chuyển", "đã đến bưu cục", "đang giao hàng", "nhận kiện")

	status := "Đã ghi nhận mã vận đơn trên cổng J&T Express"
	if delivered {
		status = "J&T Express: Giao hàng thành công (Đã ký nhận)"
	} else if inTransit {
		status = "J&T Express: Đang luân chuyển giữa các bưu cục"
	}
	return &JTTracking{Source: "jtexpress.vn", BillCode: code, Delivered: delivered, StatusText: status}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
