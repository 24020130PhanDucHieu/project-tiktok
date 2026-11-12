package handler

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// linkApp serves the open-products routes over a fake store and a fake "which product is this link" lookup.
func linkApp(store *fakeOpenProducts, lookup func(string) (service.ProductLinkInfo, error)) *fiber.App {
	h := (&CollabHandler{}).WithOpenProducts(store)
	h.linkInfo = func(_ context.Context, link string) (service.ProductLinkInfo, error) { return lookup(link) }
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Get("/open-products", h.OpenProducts)
	app.Post("/open-products", h.AddOpenProducts)
	app.Put("/open-products/:productId", h.SetOpenProductAds)
	app.Delete("/open-products/:productId", h.DeleteOpenProduct)
	return app
}

var knownLinks = map[string]service.ProductLinkInfo{
	"https://vt.tiktok.com/a/": {ProductID: "111111", Name: "Áo Polo", Image: "https://img/a.webp"},
	"https://vt.tiktok.com/b/": {ProductID: "222222", Name: "Khăn lạnh"},
}

func lookupKnown(link string) (service.ProductLinkInfo, error) {
	if info, ok := knownLinks[link]; ok {
		return info, nil
	}
	return service.ProductLinkInfo{}, errors.New("Link này không dẫn tới một sản phẩm TikTok Shop.")
}

func TestPastedLinksAreFiledUnderTheirProductAndListedLater(t *testing.T) {
	store := &fakeOpenProducts{}
	app := linkApp(store, lookupKnown)

	code, body := socialCall(app, "POST", "/open-products", `{"rows":[{"link":"  https://vt.tiktok.com/a/ "},{"link":"https://vt.tiktok.com/b/","adsCommissionRate":12.5}]}`)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	got := body["data"].([]any)
	if len(got) != 2 || got[0].(map[string]any)["productId"] != "111111" || got[1].(map[string]any)["ok"] != true {
		t.Fatalf("each row says which product it went to: %v", got)
	}
	if p := store.saved["111111"]; p.Link != "https://vt.tiktok.com/a/" || p.Name != "Áo Polo" || p.Image != "https://img/a.webp" {
		t.Fatalf("the link is trimmed and the name and picture come from it: %+v", p)
	}
	if store.saved["222222"].AdsCommissionRate != 12.5 {
		t.Fatalf("the commission typed with the link is saved: %+v", store.saved["222222"])
	}

	_, body = socialCall(app, "GET", "/open-products", "")
	if items := body["data"].([]any); len(items) != 2 {
		t.Fatalf("the list is what was saved: %v", items)
	}
}

func TestOneBadLinkDoesNotStopTheOthers(t *testing.T) {
	store := &fakeOpenProducts{}
	_, body := socialCall(linkApp(store, lookupKnown), "POST", "/open-products", `{"rows":[{"link":"https://vt.tiktok.com/zz/"},{"link":"https://vt.tiktok.com/a/"}]}`)
	got := body["data"].([]any)
	bad, good := got[0].(map[string]any), got[1].(map[string]any)
	if bad["ok"] == true || !strings.Contains(bad["error"].(string), "không dẫn tới") || good["ok"] != true {
		t.Fatalf("the failed row says why and the good one is saved: %v", got)
	}
	if len(store.saved) != 1 {
		t.Fatalf("only the good link is saved: %v", store.saved)
	}
}

func TestAddingALinkAgainKeepsTheCommissionAlreadySaved(t *testing.T) {
	store := &fakeOpenProducts{saved: map[string]domain.OpenProduct{"111111": {ProductID: "111111", Link: "https://vt.tiktok.com/old/", AdsCommissionRate: 9}}}
	socialCall(linkApp(store, lookupKnown), "POST", "/open-products", `{"rows":[{"link":"https://vt.tiktok.com/a/"}]}`)
	if p := store.saved["111111"]; p.Link != "https://vt.tiktok.com/a/" || p.AdsCommissionRate != 9 {
		t.Fatalf("a new link replaces the old one, the commission stays: %+v", p)
	}
}

func TestOpenProductRowsAreChecked(t *testing.T) {
	store := &fakeOpenProducts{}
	app := linkApp(store, lookupKnown)
	if code, _ := socialCall(app, "POST", "/open-products", `{"rows":[]}`); code != 400 {
		t.Fatalf("nothing pasted: %d", code)
	}
	rows := strings.TrimSuffix(strings.Repeat(`{"link":"https://vt.tiktok.com/a/"},`, maxOpenProductRows+1), ",")
	if code, _ := socialCall(app, "POST", "/open-products", `{"rows":[`+rows+`]}`); code != 400 {
		t.Fatalf("too many at once: %d", code)
	}
	_, body := socialCall(app, "POST", "/open-products", `{"rows":[{"link":"https://vt.tiktok.com/a/","adsCommissionRate":95}]}`)
	if row := body["data"].([]any)[0].(map[string]any); row["ok"] == true || len(store.saved) != 0 {
		t.Fatalf("a commission above the ceiling is refused for that row: %v %v", row, store.saved)
	}
}

func TestEveryPastedLinkIsLookedUp(t *testing.T) {
	var calls atomic.Int32
	store := &fakeOpenProducts{}
	app := linkApp(store, func(link string) (service.ProductLinkInfo, error) {
		calls.Add(1)
		return service.ProductLinkInfo{ProductID: strings.TrimPrefix(link, "https://vt.tiktok.com/")}, nil
	})
	var rows []string
	for _, id := range []string{"100001", "100002", "100003", "100004", "100005", "100006", "100007", "100008"} {
		rows = append(rows, `{"link":"https://vt.tiktok.com/`+id+`"}`)
	}
	socialCall(app, "POST", "/open-products", `{"rows":[`+strings.Join(rows, ",")+`]}`)
	if calls.Load() != 8 || len(store.saved) != 8 {
		t.Fatalf("%d lookups, %d saved", calls.Load(), len(store.saved))
	}
}

func TestChangingTheAdsCommissionAndForgettingAProduct(t *testing.T) {
	store := &fakeOpenProducts{saved: map[string]domain.OpenProduct{"111111": {ProductID: "111111", Link: "https://vt.tiktok.com/a/"}}}
	app := linkApp(store, lookupKnown)

	if code, _ := socialCall(app, "PUT", "/open-products/111111", `{"adsCommissionRate":15}`); code != 200 || store.saved["111111"].AdsCommissionRate != 15 {
		t.Fatalf("the commission is changed: %v", store.saved)
	}
	if code, _ := socialCall(app, "PUT", "/open-products/111111", `{"adsCommissionRate":-1}`); code != 400 {
		t.Fatalf("a negative commission: %d", code)
	}
	if code, _ := socialCall(app, "PUT", "/open-products/999999", `{"adsCommissionRate":10}`); code != 404 {
		t.Fatalf("an unknown product: %d", code)
	}
	if code, _ := socialCall(app, "DELETE", "/open-products/111111", ""); code != 200 || len(store.saved) != 0 {
		t.Fatalf("the product is forgotten: %d %v", code, store.saved)
	}
	if code, _ := socialCall(app, "DELETE", "/open-products/111111", ""); code != 404 {
		t.Fatalf("forgetting twice: %d", code)
	}
}
