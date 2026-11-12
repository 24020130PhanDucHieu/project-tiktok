package handler

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
)

// The list of open-collaboration products the team has a link for. It is kept in the database, so a link is added once
// and every later collaboration on that product has it already (see applyOpenProduct).

const (
	maxOpenProductRows = 50
	linkLookups        = 5 // links followed at the same time: each one is a request to TikTok
)

func (h *CollabHandler) requireProducts() error {
	if h.products == nil {
		return httpx.NewError(fiber.StatusServiceUnavailable, "Chưa lưu được thông tin sản phẩm cộng tác mở.")
	}
	return nil
}

func adsRateError() error {
	return httpx.NewError(fiber.StatusBadRequest, fmt.Sprintf("Hoa hồng quảng cáo phải từ 0%% đến %g%%.", float64(maxAdsCommissionPct)))
}

// OpenProducts handles GET /open-products: every product that has a link, most recently changed first.
func (h *CollabHandler) OpenProducts(c *fiber.Ctx) error {
	if err := h.requireProducts(); err != nil {
		return err
	}
	items, err := h.products.List(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, items)
}

type openProductRow struct {
	Link              string   `json:"link"`
	AdsCommissionRate *float64 `json:"adsCommissionRate"` // nil: keep what is saved
}

type openProductResult struct {
	Link      string `json:"link"`
	ProductID string `json:"productId,omitempty"`
	Name      string `json:"name,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// AddOpenProducts handles POST /open-products: files each pasted link under the product it leads to. A row that fails
// (not a TikTok link, not a product) says why and does not stop the others.
func (h *CollabHandler) AddOpenProducts(c *fiber.Ctx) error {
	var req struct {
		Rows []openProductRow `json:"rows"`
	}
	if err := c.BodyParser(&req); err != nil {
		return httpx.NewError(fiber.StatusBadRequest, "Dữ liệu không hợp lệ")
	}
	if len(req.Rows) == 0 {
		return httpx.NewError(fiber.StatusBadRequest, "Hãy dán ít nhất một link.")
	}
	if len(req.Rows) > maxOpenProductRows {
		return httpx.NewError(fiber.StatusBadRequest, fmt.Sprintf("Mỗi lần thêm tối đa %d link.", maxOpenProductRows))
	}
	if err := h.requireProducts(); err != nil {
		return err
	}
	ctx := c.UserContext()

	// find the product of every link first (the slow part, a few at a time), then save in the order they were pasted
	found := make([]domain.OpenProduct, len(req.Rows))
	failed := make([]error, len(req.Rows))
	var wg sync.WaitGroup
	slots := make(chan struct{}, linkLookups)
	for i, row := range req.Rows {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			link := strings.TrimSpace(row.Link)
			info, err := h.linkInfo(ctx, link)
			failed[i] = err
			found[i] = domain.OpenProduct{ProductID: info.ProductID, Name: info.Name, Image: info.Image, Link: link}
		}()
	}
	wg.Wait()

	by := middleware.ActorOf(c).Email
	out := make([]openProductResult, len(req.Rows))
	for i, row := range req.Rows {
		res := openProductResult{Link: strings.TrimSpace(row.Link)}
		out[i] = res
		if failed[i] != nil {
			out[i].Error = failed[i].Error()
			continue
		}
		p := found[i]
		if row.AdsCommissionRate != nil {
			if *row.AdsCommissionRate < 0 || *row.AdsCommissionRate > maxAdsCommissionPct {
				out[i].Error = adsRateError().Error()
				continue
			}
			p.AdsCommissionRate = *row.AdsCommissionRate
		} else if saved, err := h.products.Get(ctx, p.ProductID); err == nil && saved != nil {
			p.AdsCommissionRate = saved.AdsCommissionRate
		}
		p.UpdatedBy = by
		if err := h.products.Upsert(ctx, &p); err != nil {
			return err
		}
		out[i].ProductID, out[i].Name, out[i].OK = p.ProductID, p.Name, true
	}
	return httpx.OK(c, out)
}

// SetOpenProductAds handles PUT /open-products/:productId: the Shop Ads commission of a saved product.
func (h *CollabHandler) SetOpenProductAds(c *fiber.Ctx) error {
	var req struct {
		AdsCommissionRate float64 `json:"adsCommissionRate"`
	}
	if err := c.BodyParser(&req); err != nil {
		return httpx.NewError(fiber.StatusBadRequest, "Dữ liệu không hợp lệ")
	}
	if req.AdsCommissionRate < 0 || req.AdsCommissionRate > maxAdsCommissionPct {
		return adsRateError()
	}
	if err := h.requireProducts(); err != nil {
		return err
	}
	ok, err := h.products.SetAdsRate(c.UserContext(), c.Params("productId"), req.AdsCommissionRate, middleware.ActorOf(c).Email)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.NewError(fiber.StatusNotFound, "Sản phẩm này chưa có link nào được lưu.")
	}
	return httpx.OK(c, fiber.Map{"productId": c.Params("productId"), "adsCommissionRate": req.AdsCommissionRate})
}

// DeleteOpenProduct handles DELETE /open-products/:productId. Collaborations already opened keep the link they have.
func (h *CollabHandler) DeleteOpenProduct(c *fiber.Ctx) error {
	if err := h.requireProducts(); err != nil {
		return err
	}
	ok, err := h.products.Delete(c.UserContext(), c.Params("productId"))
	if err != nil {
		return err
	}
	if !ok {
		return httpx.NewError(fiber.StatusNotFound, "Sản phẩm này chưa có link nào được lưu.")
	}
	return httpx.OK(c, fiber.Map{"deleted": true})
}
