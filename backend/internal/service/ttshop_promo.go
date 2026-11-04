package service

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// Prices that buyers really pay come from the ongoing promotion activities, not from the product list price.
// Everything here is read-only (activity search + get activity + get product) and best-effort: if TikTok refuses
// a call the list simply keeps the list price, it never fails the page.
const (
	ttsPromoSearchPath = "/promotion/202309/activities/search"
	ttsPromoGetPath    = "/promotion/202309/activities/"
	ttsProductGetPath  = "/product/202309/products/"
	ttsPromoTTL        = 60 * time.Second
	ttsSkuTTL          = 5 * time.Minute
	ttsPromoMaxPages   = 3
)

// promoEntry is one ongoing activity's price for a product: per SKU, or one price for the whole product.
type promoEntry struct {
	rank        int // 0 = deal (fixed price), 1 = flash sale; a deal wins over a flash sale for the same SKU
	skus        map[string]float64
	productWide float64
	hasProduct  bool
}

// skuPrices holds the current list price of every SKU of a product.
type skuPrices struct {
	prices map[string]float64
	at     time.Time
}

func ttsAmount(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil && v > 0
}

// promotions returns ongoing activity prices keyed by product id (cached for ttsPromoTTL).
func (s *TTShopService) promotions(ctx context.Context, cipher string) (map[string][]promoEntry, error) {
	s.promoMu.Lock()
	defer s.promoMu.Unlock()
	if s.promo != nil && time.Since(s.promoAt) < ttsPromoTTL {
		return s.promo, nil
	}
	q := url.Values{"shop_cipher": {cipher}}
	type act struct{ id, kind string }
	var acts []act
	token := ""
	for page := 0; page < ttsPromoMaxPages; page++ {
		raw, err := s.postSearch(ctx, ttsPromoSearchPath, q, map[string]any{"status": "ONGOING", "page_size": 50, "page_token": token})
		if err != nil {
			return nil, err
		}
		var out struct {
			NextPageToken string `json:"next_page_token"`
			Activities    []struct {
				ID   string `json:"id"`
				Type string `json:"activity_type"`
			} `json:"activities"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		for _, a := range out.Activities {
			acts = append(acts, act{a.ID, a.Type})
		}
		if out.NextPageToken == "" {
			break
		}
		token = out.NextPageToken
	}

	res := map[string][]promoEntry{}
	for _, a := range acts {
		rank := 0
		switch a.kind {
		case "FIXED_PRICE":
		case "FLASHSALE":
			rank = 1
		default:
			continue // DIRECT_DISCOUNT etc. give a percentage, not a price: not shown rather than guessed
		}
		raw, err := s.get(ctx, ttsPromoGetPath+a.id, q)
		if err != nil {
			continue
		}
		var g struct {
			Products []struct {
				ID            string `json:"id"`
				ActivityPrice struct {
					Amount string `json:"amount"`
				} `json:"activity_price"`
				Skus []struct {
					ID            string `json:"id"`
					ActivityPrice struct {
						Amount string `json:"amount"`
					} `json:"activity_price"`
				} `json:"skus"`
			} `json:"products"`
		}
		if json.Unmarshal(raw, &g) != nil {
			continue
		}
		for _, p := range g.Products {
			e := promoEntry{rank: rank, skus: map[string]float64{}}
			if v, ok := ttsAmount(p.ActivityPrice.Amount); ok {
				e.productWide, e.hasProduct = v, true
			}
			for _, k := range p.Skus {
				if v, ok := ttsAmount(k.ActivityPrice.Amount); ok {
					e.skus[k.ID] = v
				}
			}
			if e.hasProduct || len(e.skus) > 0 {
				res[p.ID] = append(res[p.ID], e)
			}
		}
	}
	for _, es := range res {
		sort.SliceStable(es, func(i, j int) bool { return es[i].rank < es[j].rank })
	}
	s.promo, s.promoAt = res, time.Now()
	return res, nil
}

// productSkuPrices reads the list price of each SKU of a product (cached for ttsSkuTTL).
func (s *TTShopService) productSkuPrices(ctx context.Context, cipher, productID string) (map[string]float64, error) {
	s.skuCacheM.Lock()
	if c, ok := s.skuCache[productID]; ok && time.Since(c.at) < ttsSkuTTL {
		s.skuCacheM.Unlock()
		return c.prices, nil
	}
	s.skuCacheM.Unlock()
	raw, err := s.get(ctx, ttsProductGetPath+productID, url.Values{"shop_cipher": {cipher}})
	if err != nil {
		return nil, err
	}
	var p struct {
		Skus []struct {
			ID    string `json:"id"`
			Price struct {
				SalePrice string `json:"sale_price"`
			} `json:"price"`
		} `json:"skus"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	prices := map[string]float64{}
	for _, k := range p.Skus {
		if v, ok := ttsAmount(k.Price.SalePrice); ok {
			prices[k.ID] = v
		}
	}
	s.skuCacheM.Lock()
	if s.skuCache == nil {
		s.skuCache = map[string]skuPrices{}
	}
	s.skuCache[productID] = skuPrices{prices: prices, at: time.Now()}
	s.skuCacheM.Unlock()
	return prices, nil
}

// effectiveRange is the lowest and highest price a buyer pays across all SKUs: the best-ranked activity that
// prices a SKU wins, otherwise the SKU keeps its list price. ok is false when no SKU is discounted.
func effectiveRange(list map[string]float64, entries []promoEntry) (lo, hi float64, ok bool) {
	if len(list) == 0 {
		return 0, 0, false
	}
	first := true
	for id, base := range list {
		price := base
		for _, e := range entries {
			if v, found := e.skus[id]; found {
				price = v
				break
			}
			if e.hasProduct {
				price = e.productWide
				break
			}
		}
		if price != base {
			ok = true
		}
		if first || price < lo {
			lo = price
		}
		if first || price > hi {
			hi = price
		}
		first = false
	}
	return lo, hi, ok
}

// attachSalePrices fills SalePriceMin/Max for products in an ongoing priced activity. Failures are ignored on purpose.
func (s *TTShopService) attachSalePrices(ctx context.Context, cipher string, items []OpenCollabProduct) {
	promo, err := s.promotions(ctx, cipher)
	if err != nil || len(promo) == 0 {
		return
	}
	for i := range items {
		entries := promo[items[i].ProductID]
		if len(entries) == 0 {
			continue
		}
		list, err := s.productSkuPrices(ctx, cipher, items[i].ProductID)
		if err != nil {
			continue
		}
		if lo, hi, ok := effectiveRange(list, entries); ok {
			items[i].SalePriceMin = strconv.FormatFloat(lo, 'f', -1, 64)
			items[i].SalePriceMax = strconv.FormatFloat(hi, 'f', -1, 64)
		}
	}
}
