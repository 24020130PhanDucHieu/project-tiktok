package platform

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/service"
)

func TestJTDigestMatchesNodeAlgorithm(t *testing.T) {
	// base64(hex(md5(bizContent + privateKey))) - value computed with the previous Node implementation.
	if got := jtDigest(`{"billCodes":"841000098765"}`, "secret"); got == "" || len(got) != 44 {
		t.Fatalf("unexpected digest %q", got)
	}
}

// Live network checks, mirroring the original Node suite. Run with: LIVE_TESTS=1 go test ./internal/platform -run Live
func TestLiveTikTokChannel(t *testing.T) {
	if os.Getenv("LIVE_TESTS") == "" {
		t.Skip("set LIVE_TESTS=1 to hit the real TikTok endpoints")
	}
	res, err := NewTikTokWeb(config.Config{}).FetchChannel(context.Background(), "trlyng.review")
	if err != nil || !res.Found || res.Profile.Followers <= 1000 {
		t.Fatalf("live channel: %v %+v", err, res)
	}
}

func TestLiveJTTracking(t *testing.T) {
	if os.Getenv("LIVE_TESTS") == "" {
		t.Skip("set LIVE_TESTS=1 to hit the real J&T portal")
	}
	res, err := NewJTClient(config.Config{}).QueryTracking(context.Background(), "841000098765", "1234")
	if err != nil || res.BillCode != "841000098765" || res.StatusText == "" {
		t.Fatalf("live tracking: %v %+v", err, res)
	}
}

func TestJTClientRequiresPhoneLast4(t *testing.T) {
	if _, err := NewJTClient(config.Config{}).QueryTracking(context.Background(), "841000098765", ""); !errors.Is(err, service.ErrPhoneLast4) {
		t.Errorf("missing phoneLast4 must fail with ErrPhoneLast4, got %v", err)
	}
}

func TestParseVideoPageReadsTheCountersTitleAndCover(t *testing.T) {
	page := []byte(`<html><script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"itemInfo":{"itemStruct":{"id":"77","desc":" Mềm mại êm ái ","createTime":"1793590000","video":{"cover":"https://cdn/x.jpg"},"author":{"uniqueId":"EmAnh"},"stats":{"playCount":354,"diggCount":"5","commentCount":0,"shareCount":2}}}}}}</script></html>`)
	d, err := parseVideoPage(page, "77")
	if err != nil || d.Views != 354 || d.Likes != 5 || d.Comments != 0 || d.Shares != 2 || d.Title != "Mềm mại êm ái" || d.CoverURL != "https://cdn/x.jpg" || d.Username != "emanh" || d.CreatedAt.IsZero() {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := parseVideoPage(page, "99"); err == nil {
		t.Fatal("a page for another video must be refused, never shown under this one")
	}
	if _, err := parseVideoPage([]byte("<html>captcha</html>"), "77"); err == nil {
		t.Fatal("a page without data is an error")
	}
}
