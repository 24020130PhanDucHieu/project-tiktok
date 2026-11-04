package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
)

func newTTShopForTest(baseURL string) *TTShopService {
	return &TTShopService{
		cfg:  config.Config{TTSAppKey: "app-key", TTSAppSecret: "top-secret", TTSAuthBaseURL: baseURL, TTSAPIBaseURL: baseURL},
		http: &http.Client{Timeout: 2 * time.Second},
	}
}

func TestTTShopSignSkipsSignAndAccessTokenAndSortsParams(t *testing.T) {
	s := newTTShopForTest("")
	q := url.Values{"timestamp": {"1700000000"}, "app_key": {"app-key"}, "shop_cipher": {"c"}, "sign": {"old"}, "access_token": {"tok"}}

	canonical := "top-secret" + "/authorization/202309/shops" + "app_keyapp-key" + "shop_cipherc" + "timestamp1700000000" + `{"a":1}` + "top-secret"
	mac := hmac.New(sha256.New, []byte("top-secret"))
	mac.Write([]byte(canonical))
	want := hex.EncodeToString(mac.Sum(nil))

	if got := s.sign("/authorization/202309/shops", q, []byte(`{"a":1}`)); got != want {
		t.Fatalf("sign = %s, want %s", got, want)
	}
	if a, b := s.sign("/p", q, nil), s.sign("/p", url.Values{"timestamp": {"1700000000"}, "app_key": {"app-key"}, "shop_cipher": {"c"}}, nil); a != b {
		t.Fatal("sign and access_token must not influence the signature")
	}
}

func TestTTShopTokenCallParsesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/v2/token/get" || q.Get("app_key") != "app-key" || q.Get("app_secret") != "top-secret" ||
			q.Get("auth_code") != "abc" || q.Get("grant_type") != "authorized_code" {
			w.Write([]byte(`{"code":1,"message":"bad request"}`))
			return
		}
		w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"at","access_token_expire_in":1900000000,"refresh_token":"rt","refresh_token_expire_in":1950000000,"open_id":"o","seller_name":"Cool Mate"}}`))
	}))
	defer srv.Close()

	s := newTTShopForTest(srv.URL)
	tok, err := s.tokenCall(context.Background(), "/api/v2/token/get", url.Values{"auth_code": {"abc"}, "grant_type": {"authorized_code"}})
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "at" || tok.RefreshToken != "rt" || tok.SellerName != "Cool Mate" {
		t.Fatalf("unexpected token %+v", tok)
	}
	if !ttsExpiry(tok.AccessTokenExpireIn).Equal(time.Unix(1900000000, 0)) {
		t.Fatal("a unix timestamp expiry must be used as is")
	}
	if d := time.Until(ttsExpiry(3600)); d < 59*time.Minute || d > 61*time.Minute {
		t.Fatalf("a small expiry must be a lifetime in seconds, got %v", d)
	}

	if _, err := s.tokenCall(context.Background(), "/api/v2/token/get", url.Values{"auth_code": {"wrong"}}); err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("a non-zero code must surface the API message, got %v", err)
	}
}

func TestTTShopErrorsNeverLeakTheSecret(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	closedURL := srv.URL
	srv.Close() // connection refused: net/http would normally embed the full URL (with app_secret) in the error

	s := newTTShopForTest(closedURL)
	_, err := s.tokenCall(context.Background(), "/api/v2/token/get", nil)
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if strings.Contains(err.Error(), "top-secret") {
		t.Fatalf("error leaks the app secret: %v", err)
	}
}

func TestTTShopSealRoundTrip(t *testing.T) {
	s := newTTShopForTest("")
	s.cfg.JWTSecret = "jwt"
	sealed, err := s.seal("access-token")
	if err != nil || strings.Contains(sealed, "access-token") {
		t.Fatalf("seal failed or left plaintext: %v", err)
	}
	if plain, err := s.open(sealed); err != nil || plain != "access-token" {
		t.Fatalf("open = %q, %v", plain, err)
	}
}

func TestParseOpenCollabsConvertsRateAndKeepsStatus(t *testing.T) {
	data := []byte(`{"next_page_token":"nxt","total_count":2,"open_collaborations":[
	 {"id":"oc1","status":"NORMAL","content_creator_count":3,"showcase_creator_count":5,
	  "current_commission":{"rate":3555,"start_time":1,"end_time":0},
	  "product":{"id":"p1","title":"Quạt mini","main_image_url":"https://img/1.jpg","inventory":42,
	   "original_price":{"currency":"VND","minimum_amount":"100000","maximum_amount":"150000"}}},
	 {"id":"oc2","status":"TERMINATING","current_commission":{"rate":100,"end_time":1900000000},"product":{"id":"p2","title":"Tai nghe"}}]}`)
	page, err := parseOpenCollabs(data)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.NextPageToken != "nxt" || len(page.Items) != 2 {
		t.Fatalf("unexpected page %+v", page)
	}
	a, b := page.Items[0], page.Items[1]
	if a.ProductID != "p1" || a.Title != "Quạt mini" || a.CommissionRate != 35.55 || a.Status != OpenCollabNormal ||
		a.MinPrice != "100000" || a.Currency != "VND" || a.CreatorCount != 3 || a.ShowcaseCreatorCount != 5 || a.Inventory != 42 {
		t.Fatalf("unexpected first item %+v", a)
	}
	if b.CommissionRate != 1 || b.Status != "TERMINATING" || b.EndTime != 1900000000 {
		t.Fatalf("unexpected second item %+v", b)
	}
	if _, err := parseOpenCollabs([]byte(`not json`)); err == nil {
		t.Fatal("invalid JSON must fail")
	}
	if empty, err := parseOpenCollabs([]byte(`{}`)); err != nil || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatalf("an empty result must give an empty (non-nil) list, got %+v %v", empty, err)
	}
}

func TestTTShopPostIsLimitedToSearchEndpoints(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	s := newTTShopForTest(srv.URL)
	for _, p := range []string{"/affiliate_seller/202412/open_collaborations", "/product/202309/products", "/affiliate_seller/202409/open_collaborations/products/1"} {
		if _, err := s.postSearch(context.Background(), p, nil, map[string]string{}); err == nil {
			t.Fatalf("POST to %s must be refused", p)
		}
	}
	if hits != 0 {
		t.Fatalf("a refused POST must never reach TikTok, got %d requests", hits)
	}
}
