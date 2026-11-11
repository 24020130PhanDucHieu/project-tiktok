package platform

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// jtPortalURL is the public tracking page of J&T Express Vietnam.
const jtPortalURL = "https://jtexpress.vn/vi/tracking"

// JTClient queries J&T Express Vietnam (official Open Platform when configured, public portal otherwise).
type JTClient struct {
	cfg       config.Config
	http      *http.Client
	portalURL string // the public page; a field so that tests can point it at a fake
}

// NewJTClient builds a JTClient.
func NewJTClient(cfg config.Config) *JTClient {
	return &JTClient{cfg: cfg, http: jtHTTPClient(), portalURL: jtPortalURL}
}

// jtHTTPError is the official API answering with an error status (bad credentials, maintenance...), as opposed to
// a network failure or an unreadable body.
type jtHTTPError struct{ status int }

func (e *jtHTTPError) Error() string { return fmt.Sprintf("J&T OpenAPI trả HTTP %d", e.status) }

func jtDigest(bizContent, privateKey string) string {
	sum := md5.Sum([]byte(bizContent + privateKey))
	return base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(sum[:])))
}

// jtDeliveredRe recognises a "delivered" scan. "sign" only counts at the start of a word (sign, signed, signing):
// a plain substring would also match "assigned" or "design".
var jtDeliveredRe = regexp.MustCompile(`ký nhận|giao hàng thành công|delivered|\bsign`)

func isJTDelivered(scanType string) bool {
	return jtDeliveredRe.MatchString(strings.ToLower(scanType))
}

// QueryTracking resolves the shipment journey of a bill code. J&T only reveals a parcel
// when the last 4 digits of the receiver's phone number are supplied with the bill code.
func (s *JTClient) QueryTracking(ctx context.Context, billCode, phoneLast4 string) (*service.JTTracking, error) {
	code := strings.TrimSpace(billCode)
	if code == "" {
		return nil, errors.New("Thiếu mã vận đơn J&T Express")
	}
	phone4 := strings.TrimSpace(phoneLast4)
	if len(phone4) != 4 || service.NormalizePhoneLast4(phone4) != phone4 {
		return nil, service.ErrPhoneLast4
	}

	if s.cfg.JTAPIAccount != "" && s.cfg.JTPrivateKey != "" {
		res, err := s.queryOpenAPI(ctx, code, phone4)
		var refused *jtHTTPError
		if !errors.As(err, &refused) {
			return res, err // an answer, or a network/parse failure the caller already handles
		}
		// the official API refused (credentials, maintenance): the public portal answers with the same bill code
		log.Printf("J&T OpenAPI: %v, using the public portal", err)
	}

	if res := s.queryPublicPortal(ctx, code, phone4); res != nil {
		return res, nil
	}
	return &service.JTTracking{
		Source: "jt-tracker", BillCode: code, Delivered: false, Events: []service.JTEvent{},
		StatusText: fmt.Sprintf("Đang theo dõi vận đơn J&T #%s (Quét tự động 4 giờ/lần)", code),
	}, nil
}

func (s *JTClient) queryOpenAPI(ctx context.Context, code, phone4 string) (*service.JTTracking, error) {
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
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &jtHTTPError{status: resp.StatusCode}
	}

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
	delivered := isJTDelivered(scanType)
	status := latestDesc
	if status == "" {
		switch {
		case delivered:
			status = "Đã giao hàng thành công (J&T OpenAPI)"
		case len(details) == 0: // J&T knows no journey for this bill yet: do not pretend the parcel is moving
			status = "Chưa có hành trình cho vận đơn này trên J&T"
		default:
			status = "Đang luân chuyển tại bưu cục J&T"
		}
	}
	out := &service.JTTracking{Source: "jt-openapi", BillCode: code, Delivered: delivered, StatusText: status}
	out.Events = []service.JTEvent{}
	for _, d := range details {
		out.Details = append(out.Details, d)
		text, _ := d["desc"].(string)
		at, _ := d["scanTime"].(string)
		when, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(at), jtZone)
		if err != nil || strings.TrimSpace(text) == "" {
			continue
		}
		out.Events = append(out.Events, service.JTEvent{Time: when, Text: strings.TrimSpace(text)})
	}
	sortJTEvents(out.Events)
	return out, nil
}

// queryPublicPortal reads the journey from the public tracking page. nil means the page gave no answer (network,
// HTTP error), which the caller reports as "still tracking"; the reason is logged, because a server that J&T does
// not answer looks exactly like a parcel that is not moving.
func (s *JTClient) queryPublicPortal(ctx context.Context, code, phone4 string) *service.JTTracking {
	ctx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	target := s.portalURL + "?type=track&billcode=" + url.QueryEscape(code) + "&cellphone=" + url.QueryEscape(phone4)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := s.http.Do(req)
	if err != nil {
		log.Printf("J&T portal: no answer for bill %s: %v", code, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		log.Printf("J&T portal: HTTP %d for bill %s", resp.StatusCode, code)
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		log.Printf("J&T portal: unreadable answer for bill %s: %v", code, err)
		return nil
	}
	page := string(raw)
	out := &service.JTTracking{Source: "jtexpress.vn", BillCode: code, Events: []service.JTEvent{}}

	block, hasBlock := jtResultBlock(page)
	if !hasBlock {
		// not the page we know (J&T changed it, or a block page): the old reading of the whole text, and a log line
		log.Printf("J&T portal: no result block in the page for bill %s (%d bytes)", code, len(raw))
		lower := strings.ToLower(page)
		out.Delivered = containsAny(lower, "giao hàng thành công", "đã ký nhận", "ký nhận thành công")
		switch {
		case out.Delivered:
			out.StatusText = "J&T Express: Giao hàng thành công (Đã ký nhận)"
		case containsAny(lower, "đang vận chuyển", "đã đến bưu cục", "đang giao hàng", "nhận kiện"):
			out.StatusText = "J&T Express: Đang luân chuyển giữa các bưu cục"
		default:
			out.StatusText = "Đã ghi nhận mã vận đơn trên cổng J&T Express"
		}
		return out
	}
	events, found := parseJTBlock(block)
	switch {
	case !found:
		out.StatusText = "J&T không tìm thấy vận đơn này. Hãy kiểm tra mã vận đơn và 4 số cuối SĐT người nhận."
	case len(events) == 0:
		log.Printf("J&T portal: a result without readable scans for bill %s", code)
		out.StatusText = "Đã ghi nhận mã vận đơn trên cổng J&T Express"
	default:
		out.Events = events
		out.StatusText = events[0].Text
		out.Delivered = isJTDelivered(events[0].Text)
	}
	return out
}

// jtZone is Vietnam time: the clock J&T prints on its page and in its API.
var jtZone = time.FixedZone("ICT", 7*3600)

var (
	jtBlockStartRe = regexp.MustCompile(`<div[^>]*class="[^"]*\bresult-tracking\b[^"]*"[^>]*>`)
	jtBlockEndRe   = regexp.MustCompile(`<div[^>]*class="[^"]*\bresult-(?:office|price-list|prohibited)\b`)
	jtNoiseRe      = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	jtTagRe        = regexp.MustCompile(`(?s)<[^>]*>`)
	jtSpaceRe      = regexp.MustCompile(`\s+`)
	jtOpenRe       = regexp.MustCompile(`\s*【\s*`)
	jtCloseRe      = regexp.MustCompile(`\s*】\s*`)
	// a scan starts with its clock and its date, in either order
	jtStampRe = regexp.MustCompile(`(\d{2}:\d{2}:\d{2})\s+(\d{4}-\d{2}-\d{2})|(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2})`)
)

// jtResultBlock cuts the tracking result out of the page: everything else on it (price list, prohibited goods,
// slogans) contains words that would be mistaken for a status.
func jtResultBlock(page string) (string, bool) {
	loc := jtBlockStartRe.FindStringIndex(page)
	if loc == nil {
		return "", false
	}
	rest := page[loc[1]:]
	if end := jtBlockEndRe.FindStringIndex(rest); end != nil {
		rest = rest[:end[0]]
	}
	return rest, true
}

// parseJTPortal reads the journey from a tracking page: the scans newest first, and whether J&T knows the bill.
func parseJTPortal(page string) ([]service.JTEvent, bool) {
	block, ok := jtResultBlock(page)
	if !ok {
		return nil, false
	}
	return parseJTBlock(block)
}

// parseJTBlock reads the scans of a result block. It works on the visible text (clock, date, description), not on
// the markup around it, so a restyled page still reads.
func parseJTBlock(block string) ([]service.JTEvent, bool) {
	if strings.Contains(block, "empty-vandon") {
		return nil, false
	}
	text := jtNoiseRe.ReplaceAllString(block, " ")
	text = jtTagRe.ReplaceAllString(text, " ")
	text = strings.ReplaceAll(html.UnescapeString(text), " ", " ") // the page pads names with non-breaking spaces
	text = strings.TrimSpace(jtSpaceRe.ReplaceAllString(text, " "))

	stamps := jtStampRe.FindAllStringSubmatchIndex(text, -1)
	events := make([]service.JTEvent, 0, len(stamps))
	for i, m := range stamps {
		end := len(text)
		if i+1 < len(stamps) {
			end = stamps[i+1][0]
		}
		clock, day := sub(text, m, 1), sub(text, m, 2)
		if clock == "" {
			day, clock = sub(text, m, 3), sub(text, m, 4)
		}
		when, err := time.ParseInLocation("2006-01-02 15:04:05", day+" "+clock, jtZone)
		desc := tidyJTText(text[m[1]:end])
		if err != nil || desc == "" {
			continue
		}
		events = append(events, service.JTEvent{Time: when, Text: desc})
	}
	sortJTEvents(events)
	return events, true
}

func sub(s string, m []int, group int) string {
	if m[2*group] < 0 {
		return ""
	}
	return s[m[2*group]:m[2*group+1]]
}

// tidyJTText removes the spaces the stripped tags left inside J&T's brackets ("【 TTKT 】" -> "【TTKT】").
func tidyJTText(s string) string {
	s = strings.TrimSpace(s)
	s = jtOpenRe.ReplaceAllString(s, "【")
	s = jtCloseRe.ReplaceAllString(s, "】")
	return strings.TrimSpace(s)
}

func sortJTEvents(events []service.JTEvent) {
	sort.SliceStable(events, func(i, j int) bool { return events[i].Time.After(events[j].Time) })
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
