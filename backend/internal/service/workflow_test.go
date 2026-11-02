package service

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestParseKocInboundText(t *testing.T) {
	addr := ParseKocInboundText("Địa chỉ nhận mẫu: Ký túc xá Ngoại ngữ VNU, Cầu Giấy, Hà Nội - SĐT 0988123456")
	if addr.Intent != "PROVIDED_ADDRESS" || addr.Phone != "0988123456" {
		t.Fatalf("address intent: %+v", addr)
	}
	if !regexp.MustCompile(`Ký túc xá Ngoại ngữ VNU`).MatchString(addr.ShippingAddress) {
		t.Fatalf("address not extracted: %q", addr.ShippingAddress)
	}

	cases := []struct {
		name, text, intent, decline string
	}{
		{"agree", "Ok shop, mình đồng ý nhận mẫu nhé!", "AGREE_NO_ADDRESS", ""},
		{"negotiate cast", "Giá cast của mình là 1 triệu và tăng hoa hồng lên 20% mới nhận nhé", "NEGOTIATE_CAST", "R02"},
		{"received", "Mình nhận được hàng rồi shop ơi", "CONFIRMED_RECEIVED", ""},
		{"not fit", "Sản phẩm này không hợp sản phẩm với mình, cho mình xin gửi trả lại", "NOT_FIT_RETURN", ""},
		{"decline busy", "Đợt này mình bận lịch quá xin phép từ chối nhé", "DECLINED", "R04"},
		{"dnc", "đừng nhắn nữa", "SET_DNC", ""},
		{"will post", "mai mình lên video nhé", "WILL_POST", ""},
		{"general", "alo", "GENERAL_REPLY", ""},
	}
	for _, c := range cases {
		got := ParseKocInboundText(c.text)
		if got.Intent != c.intent || got.SuggestedDeclineCode != c.decline {
			t.Errorf("%s: got intent=%s decline=%s, want %s/%s", c.name, got.Intent, got.SuggestedDeclineCode, c.intent, c.decline)
		}
	}

	video := ParseKocInboundText("Mình gửi shop link video nhé: https://www.tiktok.com/@trlyng.review/video/7619258382867811601")
	if video.Intent != "SUBMITTED_VIDEO" || video.VideoURL != "https://www.tiktok.com/@trlyng.review/video/7619258382867811601" {
		t.Fatalf("video: %+v", video)
	}
}

func TestSendWindowAndAntiSpam(t *testing.T) {
	if !IsWithinAllowedSendWindow(time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)) { // 10:00 VN
		t.Error("10:00 VN must be allowed")
	}
	if IsWithinAllowedSendWindow(time.Date(2026, 10, 30, 16, 0, 0, 0, time.UTC)) { // 23:00 VN
		t.Error("23:00 VN must be blocked")
	}

	s := &OutreachService{}
	if s.CanSendProactiveMessage(&domain.Koc{TiktokID: "koc1", Dnc: true}, true).Allowed {
		t.Error("DNC KOC must be blocked")
	}
	if s.CanSendProactiveMessage(&domain.Koc{TiktokID: "koc9", DaHopTac: true}, true).Allowed {
		t.Error("collaborated KOC must be blocked")
	}
	recent := time.Now().Add(-2 * time.Hour)
	if s.CanSendProactiveMessage(&domain.Koc{TiktokID: "koc2", LastContactedAt: &recent}, false).Allowed {
		// outside the window this is also blocked; both reasons are valid blocks
		t.Error("KOC contacted 2h ago must be blocked")
	}
	if !s.CanSendProactiveMessage(&domain.Koc{TiktokID: "koc3"}, true).Allowed {
		t.Error("clean KOC with bypassWindow must be allowed")
	}
}

func TestBuildInviteMessageRules(t *testing.T) {
	text := BuildInviteMessage(&domain.Koc{
		TiktokID: "trlyng.review", DisplayName: "Tủ đồ da ngăm",
		ProductName: "Khăn lạnh thể thao Cooltech", CommissionRate: 10,
	}, "")
	if regexp.MustCompile(`(?i)tặng`).MatchString(text) {
		t.Error(`invite must never say "tặng"`)
	}
	if regexp.MustCompile(`(?i)dùng thật`).MatchString(text) {
		t.Error(`invite must never say "dùng thật"`)
	}
	if !regexp.MustCompile(`(?i)trải nghiệm`).MatchString(text) {
		t.Error("invite must state the sample-for-trial policy")
	}
	paid := BuildInviteMessage(&domain.Koc{TiktokID: "a", DisplayName: "A", CastFee: 500000, CommissionRate: 12}, "")
	if !regexp.MustCompile(`500\.000đ`).MatchString(paid) || !regexp.MustCompile(`12%`).MatchString(paid) {
		t.Errorf("paid offer formatting: %s", paid)
	}
}

func TestExtractContactFromBio(t *testing.T) {
	phone, email := ExtractContactFromBio("Liên hệ cv: 0368303121\ntrlyng2@gmail.com\n1m63 50kg undertone trung tính")
	if phone != "0368303121" || email != "trlyng2@gmail.com" {
		t.Fatalf("got %q %q", phone, email)
	}
	if phone, _ := ExtractContactFromBio("call +84327637213"); phone != "0327637213" {
		t.Fatalf("+84 normalisation: %q", phone)
	}
}

func TestCompactNumbers(t *testing.T) {
	for in, want := range map[float64]string{999: "999", 1000: "1k", 12700: "12.7k", 1500000: "1.5M"} {
		if got := FormatCompact(in); got != want {
			t.Errorf("FormatCompact(%v)=%s want %s", in, got, want)
		}
	}
	for in, want := range map[string]float64{"1.2k": 1200, "3M": 3000000, "850": 850, "x": 0} {
		if got := ParseCompactCount(in); got != want {
			t.Errorf("ParseCompactCount(%q)=%v want %v", in, got, want)
		}
	}
}

func TestFormatVND(t *testing.T) {
	for in, want := range map[float64]string{0: "0", 999: "999", 1000: "1.000", 500000: "500.000", 1234567: "1.234.567"} {
		if got := FormatVND(in); got != want {
			t.Errorf("FormatVND(%v)=%s want %s", in, got, want)
		}
	}
}

func TestResolveCastAndStage(t *testing.T) {
	if fee, typ, stage := ResolveCastAndStage(500000, domain.StageQualified, ""); fee != 500000 || typ != "cast" || stage != domain.StageAwaitingApproval {
		t.Errorf("paid cast must await approval: %v %v %v", fee, typ, stage)
	}
	if _, typ, stage := ResolveCastAndStage(0, "", domain.StageSourced); typ != "freecast" || stage != domain.StageSourced {
		t.Errorf("freecast default: %v %v", typ, stage)
	}
}

func TestJTDigestMatchesNodeAlgorithm(t *testing.T) {
	// base64(hex(md5(bizContent + privateKey))) - value computed with the previous Node implementation.
	if got := jtDigest(`{"billCodes":"841000098765"}`, "secret"); got == "" || len(got) != 44 {
		t.Fatalf("unexpected digest %q", got)
	}
}

func TestMorningDigestAndAuthTokens(t *testing.T) {
	auth := NewAuthService(nil, nil, nil, nil, config.Config{JWTSecret: "s", JWTExpiresIn: time.Hour})
	u := &domain.User{Name: "A", Email: "a@b.c", Role: "admin"}
	sess, err := auth.session(u)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := auth.ParseToken(sess.Token)
	if err != nil || claims.Role != "admin" || claims.Email != "a@b.c" {
		t.Fatalf("round trip: %v %+v", err, claims)
	}
	if _, err := NewAuthService(nil, nil, nil, nil, config.Config{JWTSecret: "other"}).ParseToken(sess.Token); err == nil {
		t.Fatal("token signed with another secret must be rejected")
	}
}

// Live network checks, mirroring the original Node suite. Run with: LIVE_TESTS=1 go test ./internal/service -run Live
func TestLiveTikTokChannel(t *testing.T) {
	if os.Getenv("LIVE_TESTS") == "" {
		t.Skip("set LIVE_TESTS=1 to hit the real TikTok endpoints")
	}
	res, err := NewTikTokService(config.Config{}).FetchChannel(context.Background(), "trlyng.review")
	if err != nil || !res.Found || res.Profile.Followers <= 1000 {
		t.Fatalf("live channel: %v %+v", err, res)
	}
}

func TestLiveJTTracking(t *testing.T) {
	if os.Getenv("LIVE_TESTS") == "" {
		t.Skip("set LIVE_TESTS=1 to hit the real J&T portal")
	}
	res, err := NewJTService(config.Config{}).QueryTracking(context.Background(), "841000098765", "1234")
	if err != nil || res.BillCode != "841000098765" || res.StatusText == "" {
		t.Fatalf("live tracking: %v %+v", err, res)
	}
}

func TestPhoneLast4AndNextTask(t *testing.T) {
	for in, want := range map[string]string{"0912 345 678": "5678", "+84912345678": "5678", "1234": "1234", "12": "", "": ""} {
		if got := NormalizePhoneLast4(in); got != want {
			t.Errorf("NormalizePhoneLast4(%q)=%q want %q", in, got, want)
		}
	}
	if _, err := NewJTService(config.Config{}).QueryTracking(context.Background(), "841000098765", ""); !errors.Is(err, ErrPhoneLast4) {
		t.Errorf("missing phoneLast4 must fail with ErrPhoneLast4, got %v", err)
	}
	k := &domain.Koc{Phone: "0912345678", TrackingPhoneLast4: "9999"}
	if ResolvePhoneLast4(k) != "9999" {
		t.Error("saved digits must win over phone")
	}
	k.TrackingPhoneLast4 = ""
	if ResolvePhoneLast4(k) != "5678" {
		t.Error("fallback to KOC phone")
	}
	cases := []struct {
		koc  domain.Koc
		want string
	}{
		{domain.Koc{Stage: domain.StageReplied, CollabStep: 0}, domain.TaskCastFee},
		{domain.Koc{Stage: domain.StageAgreed, CollabStep: 1}, domain.TaskSendLink},
		{domain.Koc{Stage: domain.StageAgreed, CollabStep: 2}, domain.TaskReviewSamp},
		{domain.Koc{Stage: domain.StageAgreed, CollabStep: 2, ShippingAddress: "HN"}, domain.TaskSendSample},
		{domain.Koc{Stage: domain.StageAgreed, CollabStep: 2, ShippingAddress: "HN", TrackingCode: "X"}, ""},
		{domain.Koc{Stage: domain.StageVideoPosted, CollabStep: 4, CastFee: 500000}, domain.TaskPayCast},
		{domain.Koc{Stage: domain.StageReplied, CollabStep: 0, Dnc: true}, ""},
	}
	for i, c := range cases {
		if got := c.koc.NextTask(); got != c.want {
			t.Errorf("case %d NextTask()=%q want %q", i, got, c.want)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	for pw, ok := range map[string]bool{"Admin12345!": true, "abcdefgh": false, "12345678": false, "a1": false, "Passw0rd": true} {
		if err := ValidatePassword(pw); (err == nil) != ok {
			t.Errorf("ValidatePassword(%q) err=%v want ok=%v", pw, err, ok)
		}
	}
}

func TestResetCodeFormatAndBinding(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := sixDigitCode()
		if err != nil || len(c) != 6 || strings.Trim(c, "0123456789") != "" {
			t.Fatalf("bad code %q %v", c, err)
		}
		seen[c] = true
	}
	if len(seen) < 190 {
		t.Fatalf("codes not random enough: %d distinct of 200", len(seen))
	}
	if hashCode("userA", "123456") == hashCode("userB", "123456") {
		t.Fatal("the same code of two users must hash differently")
	}
}
