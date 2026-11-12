package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

type fakeOpenLookup struct {
	item *service.OpenCollabProduct
	err  error
}

func (f fakeOpenLookup) ProductImages(_ context.Context, ids []string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]string{}
	for _, id := range ids {
		out[id] = "https://img/" + id + ".jpg"
	}
	return out, nil
}

func (f fakeOpenLookup) OpenCollabByProduct(_ context.Context, id string) (*service.OpenCollabProduct, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.item, nil
}

// fakeOpenProducts is the per-product store of the open-collaboration link and Shop Ads commission.
type fakeOpenProducts struct{ saved map[string]domain.OpenProduct }

func (f *fakeOpenProducts) Get(_ context.Context, productID string) (*domain.OpenProduct, error) {
	if p, ok := f.saved[productID]; ok {
		return &p, nil
	}
	return nil, nil
}

func (f *fakeOpenProducts) Upsert(_ context.Context, p *domain.OpenProduct) error {
	if f.saved == nil {
		f.saved = map[string]domain.OpenProduct{}
	}
	if old, ok := f.saved[p.ProductID]; ok { // like the real store: a name or picture not given is kept
		if p.Name == "" {
			p.Name = old.Name
		}
		if p.Image == "" {
			p.Image = old.Image
		}
	}
	f.saved[p.ProductID] = *p
	return nil
}

func (f *fakeOpenProducts) List(context.Context) ([]domain.OpenProduct, error) {
	out := []domain.OpenProduct{}
	for _, p := range f.saved {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeOpenProducts) SetAdsRate(_ context.Context, id string, rate float64, by string) (bool, error) {
	p, ok := f.saved[id]
	if !ok {
		return false, nil
	}
	p.AdsCommissionRate, p.UpdatedBy = rate, by
	f.saved[id] = p
	return true, nil
}

func (f *fakeOpenProducts) Delete(_ context.Context, id string) (bool, error) {
	_, ok := f.saved[id]
	delete(f.saved, id)
	return ok, nil
}

func str(s string) *string { return &s }

func statusOf(err error) int {
	var ae *httpx.AppError
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

func TestApplyOpenProductTakesNameAndRateFromTheShop(t *testing.T) {
	h := (&CollabHandler{}).WithOpenCollabs(fakeOpenLookup{item: &service.OpenCollabProduct{
		ProductID: "p1", Title: "Quạt mini", CommissionRate: 30, Status: service.OpenCollabNormal}})
	col := &domain.Collab{Mode: domain.ModeOpen, ProductName: "typed by client", CommissionRate: 99}
	if err := h.applyOpenProduct(context.Background(), col, str("p1"), true); err != nil {
		t.Fatal(err)
	}
	if col.ProductID != "p1" || col.ProductName != "Quạt mini" || col.CommissionRate != 30 {
		t.Fatalf("name, id and rate must come from the shop: %+v", col)
	}
}

func TestApplyOpenProductRules(t *testing.T) {
	ok := fakeOpenLookup{item: &service.OpenCollabProduct{ProductID: "p1", Title: "X", CommissionRate: 10, Status: service.OpenCollabNormal}}
	ctx := context.Background()

	// choosing the open mode without a product is rejected
	if err := (&CollabHandler{}).WithOpenCollabs(ok).applyOpenProduct(ctx, &domain.Collab{Mode: domain.ModeOpen}, nil, true); statusOf(err) != fiber.StatusBadRequest {
		t.Fatalf("missing product: %v", err)
	}
	// a product on a non-open collaboration is rejected
	if err := (&CollabHandler{}).WithOpenCollabs(ok).applyOpenProduct(ctx, &domain.Collab{Mode: domain.ModeTargeted}, str("p1"), false); statusOf(err) != fiber.StatusBadRequest {
		t.Fatalf("product on targeted mode: %v", err)
	}
	// an unrelated patch on an existing open collaboration is not blocked
	if err := (&CollabHandler{}).WithOpenCollabs(ok).applyOpenProduct(ctx, &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1"}, nil, false); err != nil {
		t.Fatalf("patch without product must pass: %v", err)
	}
	// no TikTok Shop wired
	if err := (&CollabHandler{}).applyOpenProduct(ctx, &domain.Collab{Mode: domain.ModeOpen}, str("p1"), true); statusOf(err) != fiber.StatusServiceUnavailable {
		t.Fatalf("no lookup: %v", err)
	}
	// product not opened, or opened but about to expire
	if err := (&CollabHandler{}).WithOpenCollabs(fakeOpenLookup{err: service.ErrOpenCollabNotFound}).applyOpenProduct(ctx, &domain.Collab{Mode: domain.ModeOpen}, str("zz"), true); statusOf(err) != fiber.StatusNotFound {
		t.Fatalf("unknown product: %v", err)
	}
	ending := fakeOpenLookup{item: &service.OpenCollabProduct{ProductID: "p1", Status: "TERMINATING"}}
	if err := (&CollabHandler{}).WithOpenCollabs(ending).applyOpenProduct(ctx, &domain.Collab{Mode: domain.ModeOpen}, str("p1"), true); statusOf(err) != fiber.StatusConflict {
		t.Fatalf("terminating product: %v", err)
	}
}

func TestApplyOpenProductKeepsThePictureAndDropsTheOldLinkWhenTheProductChanges(t *testing.T) {
	h := (&CollabHandler{}).WithOpenCollabs(fakeOpenLookup{item: &service.OpenCollabProduct{
		ProductID: "p2", Title: "Áo polo", ImageURL: "https://img/p2.jpg", CommissionRate: 10, Status: service.OpenCollabNormal}})
	col := &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1", ProductImage: "https://img/p1.jpg", ProductLink: "https://link/p1"}
	if err := h.applyOpenProduct(context.Background(), col, str("p2"), false); err != nil {
		t.Fatal(err)
	}
	if col.ProductID != "p2" || col.ProductImage != "https://img/p2.jpg" || col.ProductLink != "" {
		t.Fatalf("a new product must bring its picture and drop the previous product's link: %+v", col)
	}
	// choosing the same product again must not throw the existing link away
	same := &domain.Collab{Mode: domain.ModeOpen, ProductID: "p2", ProductLink: "https://link/p2"}
	if err := h.applyOpenProduct(context.Background(), same, str("p2"), false); err != nil || same.ProductLink != "https://link/p2" {
		t.Fatalf("the same product keeps its link: %+v %v", same, err)
	}
}

func TestOverviewItemFilters(t *testing.T) {
	it := overviewItem{
		Collab: domain.Collab{Status: domain.CollabActive, Mode: domain.ModeOpen, ProductName: "Khăn lạnh Cooltech", Name: "Đợt 11"},
		Koc:    overviewKoc{Name: "Tủ đồ da ngăm", Handle: "@trlyng.review"},
	}
	cases := []struct {
		status, mode, q string
		want            bool
	}{
		{"", "", "", true},
		{"active", "open", "", true},
		{"closed", "", "", false},
		{"", "targeted", "", false},
		{"", "none", "", false},
		{"", "", "TRLYNG", true},
		{"", "", "khăn", true},
		{"", "", "đợt 11", true},
		{"", "", "không có", false},
	}
	for _, c := range cases {
		if got := it.matches(c.status, c.mode, c.q); got != c.want {
			t.Fatalf("matches(%q,%q,%q) = %v, want %v", c.status, c.mode, c.q, got, c.want)
		}
	}
	if !(overviewItem{}).matches("", "none", "") {
		t.Fatal("a collaboration without a mode counts as mode none")
	}
}

// An open collaboration has one link per product (TikTok makes it, nobody creates a new one per KOC), so what a
// booker pasted once for a product is there for every later KOC.
func TestTheLinkPastedOnceForAProductIsThereForTheNextKoc(t *testing.T) {
	products := &fakeOpenProducts{}
	h := (&CollabHandler{}).WithOpenCollabs(fakeOpenLookup{item: &service.OpenCollabProduct{
		ProductID: "p1", Title: "Áo polo", CommissionRate: 10, Status: service.OpenCollabNormal}}).WithOpenProducts(products)
	ctx := context.Background()

	first := &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1"}
	if err := h.setOpenProduct(ctx, first, " https://vt.tiktok.com/ZSabc/ ", 12.5, "booker@shop.vn"); err != nil {
		t.Fatal(err)
	}
	if first.ProductLink != "https://vt.tiktok.com/ZSabc/" || first.AdsCommissionRate != 12.5 {
		t.Fatalf("the collaboration takes the link and the ads rate: %+v", first)
	}
	if got := products.saved["p1"]; got.Link != "https://vt.tiktok.com/ZSabc/" || got.AdsCommissionRate != 12.5 || got.UpdatedBy != "booker@shop.vn" {
		t.Fatalf("and they are remembered for the product: %+v", got)
	}

	next := &domain.Collab{Mode: domain.ModeOpen} // another KOC choosing the same product
	if err := h.applyOpenProduct(ctx, next, str("p1"), true); err != nil {
		t.Fatal(err)
	}
	if next.ProductLink != "https://vt.tiktok.com/ZSabc/" || next.AdsCommissionRate != 12.5 || next.CommissionRate != 10 {
		t.Fatalf("the next KOC gets the product's link and ads rate, the organic rate still from the shop: %+v", next)
	}
}

// A product can be picked before its link is added to the list; choosing it again then takes the link that is there now.
func TestPickingTheSameProductAgainTakesALinkSavedAfterwards(t *testing.T) {
	products := &fakeOpenProducts{saved: map[string]domain.OpenProduct{"p1": {ProductID: "p1", Link: "https://vt.tiktok.com/ZSnew/", AdsCommissionRate: 6}}}
	h := (&CollabHandler{}).WithOpenCollabs(fakeOpenLookup{item: &service.OpenCollabProduct{
		ProductID: "p1", Title: "Áo polo", CommissionRate: 10, Status: service.OpenCollabNormal}}).WithOpenProducts(products)
	col := &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1"}
	if err := h.applyOpenProduct(context.Background(), col, str("p1"), false); err != nil {
		t.Fatal(err)
	}
	if col.ProductLink != "https://vt.tiktok.com/ZSnew/" || col.AdsCommissionRate != 6 {
		t.Fatalf("the link added later is taken: %+v", col)
	}
	// a link the collaboration already has is not replaced
	has := &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1", ProductLink: "https://vt.tiktok.com/mine/"}
	if err := h.applyOpenProduct(context.Background(), has, str("p1"), false); err != nil || has.ProductLink != "https://vt.tiktok.com/mine/" {
		t.Fatalf("%+v %v", has, err)
	}
}

func TestChangingToAProductWithNothingSavedClearsTheLinkAndTheAdsRate(t *testing.T) {
	h := (&CollabHandler{}).WithOpenCollabs(fakeOpenLookup{item: &service.OpenCollabProduct{
		ProductID: "p2", Title: "Quạt", CommissionRate: 10, Status: service.OpenCollabNormal}}).WithOpenProducts(&fakeOpenProducts{})
	col := &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1", ProductLink: "https://vt.tiktok.com/old", AdsCommissionRate: 8}
	if err := h.applyOpenProduct(context.Background(), col, str("p2"), false); err != nil {
		t.Fatal(err)
	}
	if col.ProductLink != "" || col.AdsCommissionRate != 0 {
		t.Fatalf("the previous product's link and ads rate must not stay: %+v", col)
	}
}

func TestTheOpenProductLinkMustBeATikTokLinkAndTheAdsRateSane(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []string{
		"http://vt.tiktok.com/x",             // not https
		"https://evil.example/vt.tiktok.com", // another site
		"https://tiktok.com.evil.example/x",  // look-alike host
		"javascript:alert(1)",
		"vt.tiktok.com/x",
	} {
		h := (&CollabHandler{}).WithOpenProducts(&fakeOpenProducts{})
		col := &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1"}
		if err := h.setOpenProduct(ctx, col, bad, 0, "a"); statusOf(err) != fiber.StatusBadRequest || col.ProductLink != "" {
			t.Fatalf("%q must be refused, got %v %q", bad, err, col.ProductLink)
		}
	}
	for _, good := range []string{"https://vt.tiktok.com/ZS1/", "https://www.tiktok.com/view/product/1", "https://shop.tiktok.com/view/product/1?region=VN", "https://affiliate.tiktok.com/api/v1/share/AJ1"} {
		h := (&CollabHandler{}).WithOpenProducts(&fakeOpenProducts{})
		if err := h.setOpenProduct(ctx, &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1"}, good, 0, "a"); err != nil {
			t.Fatalf("%q is a TikTok link: %v", good, err)
		}
	}
	h := (&CollabHandler{}).WithOpenProducts(&fakeOpenProducts{})
	for _, rate := range []float64{-1, 80.5, 1000} {
		if err := h.setOpenProduct(ctx, &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1"}, "", rate, "a"); statusOf(err) != fiber.StatusBadRequest {
			t.Fatalf("ads rate %v must be refused: %v", rate, err)
		}
	}
	// only the ads rate, no link yet: allowed (the link can be pasted later)
	col := &domain.Collab{Mode: domain.ModeOpen, ProductID: "p1"}
	if err := h.setOpenProduct(ctx, col, "", 15, "a"); err != nil || col.AdsCommissionRate != 15 {
		t.Fatalf("the ads rate alone can be saved: %v %+v", err, col)
	}
	// without a chosen open product there is nothing to attach them to
	if err := h.setOpenProduct(ctx, &domain.Collab{Mode: domain.ModeTargeted}, "https://vt.tiktok.com/x", 0, "a"); statusOf(err) != fiber.StatusBadRequest {
		t.Fatalf("not an open collaboration: %v", err)
	}
}

// A targeted collaboration is invited through the TikTok invitation, which keeps no picture. The collaboration card
// takes the product's picture from the shop, so it is not an empty grey square.
func TestATargetedCollaborationTakesTheProductPictureFromTheShop(t *testing.T) {
	ctx := context.Background()
	h := (&CollabHandler{}).WithOpenCollabs(fakeOpenLookup{})
	col := &domain.Collab{Mode: domain.ModeTargeted}
	if err := h.applyTargetProduct(ctx, col, str(" 1736294693654136704 ")); err != nil {
		t.Fatal(err)
	}
	if col.ProductID != "1736294693654136704" || col.ProductImage != "https://img/1736294693654136704.jpg" {
		t.Fatalf("the product and its picture: %+v", col)
	}

	// a shop that cannot be reached: the invitation is still recorded, the picture is just missing
	off := (&CollabHandler{}).WithOpenCollabs(fakeOpenLookup{err: errors.New("tiktok down")})
	col = &domain.Collab{Mode: domain.ModeTargeted}
	if err := off.applyTargetProduct(ctx, col, str("1736294693654136704")); err != nil || col.ProductID == "" || col.ProductImage != "" {
		t.Fatalf("a missing picture must not fail the request: %v %+v", err, col)
	}

	if err := h.applyTargetProduct(ctx, &domain.Collab{Mode: domain.ModeOpen}, str("1736294693654136704")); statusOf(err) != fiber.StatusBadRequest {
		t.Fatalf("only for a targeted collaboration: %v", err)
	}
	if err := h.applyTargetProduct(ctx, &domain.Collab{Mode: domain.ModeTargeted}, str("12ab")); statusOf(err) != fiber.StatusBadRequest {
		t.Fatalf("a product id is a number: %v", err)
	}
	if err := h.applyTargetProduct(ctx, &domain.Collab{Mode: domain.ModeTargeted}, nil); err != nil {
		t.Fatalf("no product given, nothing to do: %v", err)
	}
}
