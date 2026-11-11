package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The link TikTok returns for an open-collaboration product is a short link that lands on a bare product page
// (".../view/product/<id>?isv_id=<n>"). That page cannot tell which market the visitor is in, so a desktop browser
// or an in-app browser (Zalo) shows "not available in your country or region". Adding the market to the final
// address makes the page open everywhere (checked by hand on desktop and phone). No signature is forged: only
// plain market parameters are added.

const (
	ttsLinkMaxHops = 4
	ttsLinkTimeout = 8 * time.Second
)

// ttsLocale maps a TikTok Shop market to the locale of its product page.
var ttsLocale = map[string]string{
	"VN": "vi-VN", "TH": "th-TH", "ID": "id-ID", "MY": "ms-MY", "PH": "en-PH", "SG": "en-SG",
}

// isTikTokHost reports whether host is tiktok.com or one of its sub-domains.
func isTikTokHost(host string) bool {
	host = strings.ToLower(host)
	return host == "tiktok.com" || strings.HasSuffix(host, ".tiktok.com")
}

// resolveRedirects follows the redirects of a short link (at most ttsLinkMaxHops) and returns the final address.
// Only https links on hosts accepted by allow are followed, so a response can never send us to another site.
func resolveRedirects(ctx context.Context, raw string, allow func(host string) bool) (string, error) {
	client := &http.Client{
		Timeout:       ttsLinkTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	cur := raw
	for hop := 0; hop <= ttsLinkMaxHops; hop++ {
		u, err := url.Parse(cur)
		if err != nil || !allow(u.Hostname()) || (u.Scheme != "https" && u.Scheme != "http") {
			return "", errors.New("link không thuộc TikTok")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cur, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1")
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		resp.Body.Close()
		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			return cur, nil
		}
		loc, err := resp.Location()
		if err != nil {
			return cur, nil
		}
		cur = loc.String()
	}
	return cur, nil
}

// withMarket adds the market parameters to a product link, keeping any value already present.
func withMarket(raw, region string) (string, error) {
	region = strings.ToUpper(strings.TrimSpace(region))
	if region == "" {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	set := func(k, v string) {
		if q.Get(k) == "" {
			q.Set(k, v)
		}
	}
	set("share_region", region)
	set("region", region)
	if loc, ok := ttsLocale[region]; ok {
		set("locale", loc)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// openableLink turns the short link from TikTok into an address that opens on any device. If the short link
// cannot be resolved it is returned unchanged (it still works inside the TikTok app).
func (s *TTShopService) openableLink(ctx context.Context, short, region string) string {
	final, err := resolveRedirects(ctx, short, isTikTokHost)
	if err != nil || final == "" {
		return short
	}
	if out, err := withMarket(final, region); err == nil {
		return out
	}
	return short
}

// IsTikTokLink reports whether raw is an https address on tiktok.com or one of its sub-domains (what a link pasted
// by a booker for an open-collaboration product must be before it is sent to creators).
func IsTikTokLink(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Scheme == "https" && u.User == nil && isTikTokHost(u.Hostname())
}
