package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const polo = "https://www.tiktok.com/view/product/1736613031755089792?_d=x&og_info=%7B%22title%22%3A%22%C3%81o+Polo+Cooltech%22%2C%22image%22%3A%22https%3A%5C%2F%5C%2Fp16.example%5C%2Fa.webp%22%7D&unique_id=someone"

func TestTheProductIsReadFromTheAddressTheLinkLeadsTo(t *testing.T) {
	info, ok := productFromAddress(polo, isTikTokHost)
	if !ok || info.ProductID != "1736613031755089792" || info.Name != "Áo Polo Cooltech" || info.Image != "https://p16.example/a.webp" {
		t.Fatalf("%+v %v", info, ok)
	}
	// without the share details the id alone is still there
	if info, ok := productFromAddress("https://www.tiktok.com/view/product/1736294693654136704", isTikTokHost); !ok || info.ProductID != "1736294693654136704" || info.Name != "" {
		t.Fatalf("%+v %v", info, ok)
	}
	for _, not := range []string{
		"https://www.tiktok.com/@someone/video/7400000000000000000",
		"https://www.tiktok.com/view/product/abc",
		"https://evil.example/view/product/1736613031755089792",
		"https://tiktok.com.evil.example/view/product/1736613031755089792",
		"not a url",
	} {
		if _, ok := productFromAddress(not, isTikTokHost); ok {
			t.Fatalf("%q is not a TikTok product address", not)
		}
	}
	// a picture that is not https is not kept
	if info, _ := productFromAddress("https://www.tiktok.com/view/product/1736613031755089792?og_info=%7B%22image%22%3A%22http%3A%2F%2Fx%22%7D", isTikTokHost); info.Image != "" {
		t.Fatalf("only https pictures are kept: %q", info.Image)
	}
}

func TestAShortLinkIsFollowedToItsProductAndNowhereElse(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/short":
			http.Redirect(w, r, srv.URL+"/hop", http.StatusFound)
		case "/hop":
			http.Redirect(w, r, srv.URL+"/view/product/1736611408712009600?og_info=%7B%22title%22%3A%22Khan%22%7D", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, srv.URL+"/loop", http.StatusFound)
		case "/elsewhere":
			http.Redirect(w, r, "https://evil.example/view/product/1736611408712009600", http.StatusFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	local := func(h string) bool { return h == "127.0.0.1" } // stands in for "a tiktok.com host"
	ctx := context.Background()

	info, err := productFromLink(ctx, srv.URL+"/short", local)
	if err != nil || info.ProductID != "1736611408712009600" || info.Name != "Khan" {
		t.Fatalf("%+v %v", info, err)
	}
	if _, err := productFromLink(ctx, srv.URL+"/elsewhere", local); err == nil {
		t.Fatal("a redirect to another site must not be followed")
	}
	if _, err := productFromLink(ctx, srv.URL+"/loop", local); err == nil {
		t.Fatal("a redirect loop must end in an error, not hang")
	}
	if _, err := productFromLink(ctx, srv.URL+"/plain", local); err == nil || !strings.Contains(err.Error(), "không dẫn tới") {
		t.Fatalf("a page that is not a product: %v", err)
	}
}

func TestOnlyTikTokLinksAreFollowed(t *testing.T) {
	for _, bad := range []string{"", "http://vt.tiktok.com/x/", "https://example.com/x", "ftp://vt.tiktok.com/x", "https://user@vt.tiktok.com/x", "https://vt.tiktok.com/" + strings.Repeat("a", 2100)} {
		if _, err := ProductFromLink(context.Background(), bad); err == nil {
			t.Fatalf("%q must be refused before any request is made", bad)
		}
	}
}
