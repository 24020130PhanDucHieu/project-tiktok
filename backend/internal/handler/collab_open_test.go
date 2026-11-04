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

func (f fakeOpenLookup) OpenCollabByProduct(_ context.Context, id string) (*service.OpenCollabProduct, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.item, nil
}

func (f fakeOpenLookup) PromotionLink(_ context.Context, id string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "https://vt.tiktok.com/" + id, nil
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
