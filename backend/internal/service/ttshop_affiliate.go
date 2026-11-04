package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Read-only Affiliate Seller endpoints (see tts-openapi-guide: newest version of each capability).
const (
	ttsOpenCollabSearchPath   = "/affiliate_seller/202412/open_collaborations/search"
	ttsOpenCollabSettingsPath = "/affiliate_seller/202409/open_collaboration_settings"

	ttsCipherTTL = 10 * time.Minute

	// OpenCollabNormal is the status of an open collaboration that is in effect.
	OpenCollabNormal = "NORMAL"
)

// ErrOpenCollabNotFound is returned when a product has no open collaboration in the shop.
var ErrOpenCollabNotFound = errors.New("Sản phẩm này chưa mở cộng tác trên shop")

// OpenCollabProduct is one product the shop opened for open collaboration, with its public commission.
type OpenCollabProduct struct {
	CollaborationID      string  `json:"collaborationId"`
	ProductID            string  `json:"productId"`
	Title                string  `json:"title"`
	ImageURL             string  `json:"imageUrl"`
	Currency             string  `json:"currency"`
	MinPrice             string  `json:"minPrice"`
	MaxPrice             string  `json:"maxPrice"`
	Inventory            int     `json:"inventory"`
	CommissionRate       float64 `json:"commissionRate"` // percent, e.g. 30 means 30%
	Status               string  `json:"status"`         // NORMAL | TERMINATING
	ProductStatus        string  `json:"productStatus"`  // LIVE | OUT_OF_STOCK | SELLER_DEACTIVATE | ...
	Available            bool    `json:"available"`      // false = TikTok flags "Sản phẩm không có sẵn"
	EndTime              int64   `json:"endTime,omitempty"`
	CreatorCount         int     `json:"creatorCount"`
	ShowcaseCreatorCount int     `json:"showcaseCreatorCount"`
}

// OpenCollabPage is one page of open collaborations.
type OpenCollabPage struct {
	Items         []OpenCollabProduct `json:"items"`
	NextPageToken string              `json:"nextPageToken"`
	Total         int                 `json:"total"`
}

// OpenCollabSettings is the shop's open collaboration configuration.
type OpenCollabSettings struct {
	AutoAddEnabled bool    `json:"autoAddEnabled"`
	AutoAddRate    float64 `json:"autoAddRate"` // percent
}

// ttsRatePercent converts a TikTok commission rate (1/10000 increments, 3000 = 30.00%) to a percent.
func ttsRatePercent(rate int64) float64 { return float64(rate) / 100 }

// parseOpenCollabs reads the SearchOpenCollaboration response data.
func parseOpenCollabs(data json.RawMessage) (*OpenCollabPage, error) {
	var out struct {
		NextPageToken      string `json:"next_page_token"`
		TotalCount         int    `json:"total_count"`
		OpenCollaborations []struct {
			ID                   string `json:"id"`
			Status               string `json:"status"`
			ContentCreatorCount  int    `json:"content_creator_count"`
			ShowcaseCreatorCount int    `json:"showcase_creator_count"`
			CurrentCommission    struct {
				Rate    int64 `json:"rate"`
				EndTime int64 `json:"end_time"`
			} `json:"current_commission"`
			Product struct {
				ID            string `json:"id"`
				Title         string `json:"title"`
				MainImageURL  string `json:"main_image_url"`
				Inventory     int    `json:"inventory"`
				Status        string `json:"status"`
				OriginalPrice struct {
					Currency      string `json:"currency"`
					MinimumAmount string `json:"minimum_amount"`
					MaximumAmount string `json:"maximum_amount"`
				} `json:"original_price"`
			} `json:"product"`
		} `json:"open_collaborations"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errors.New("không đọc được danh sách cộng tác mở")
	}
	page := &OpenCollabPage{Items: make([]OpenCollabProduct, 0, len(out.OpenCollaborations)),
		NextPageToken: out.NextPageToken, Total: out.TotalCount}
	for _, oc := range out.OpenCollaborations {
		page.Items = append(page.Items, OpenCollabProduct{
			CollaborationID: oc.ID, ProductID: oc.Product.ID, Title: oc.Product.Title, ImageURL: oc.Product.MainImageURL,
			Currency: oc.Product.OriginalPrice.Currency, MinPrice: oc.Product.OriginalPrice.MinimumAmount,
			MaxPrice: oc.Product.OriginalPrice.MaximumAmount, Inventory: oc.Product.Inventory,
			CommissionRate: ttsRatePercent(oc.CurrentCommission.Rate), Status: oc.Status,
			ProductStatus: oc.Product.Status, Available: oc.Product.Status == "" || oc.Product.Status == "LIVE", EndTime: oc.CurrentCommission.EndTime,
			CreatorCount: oc.ContentCreatorCount, ShowcaseCreatorCount: oc.ShowcaseCreatorCount,
		})
	}
	return page, nil
}

// shopCipher returns the shop_cipher of the authorized shop (cached for a few minutes).
func (s *TTShopService) shopCipher(ctx context.Context) (string, error) {
	s.cipherMu.Lock()
	defer s.cipherMu.Unlock()
	if s.cipher != "" && time.Since(s.cipherAt) < ttsCipherTTL {
		return s.cipher, nil
	}
	shops, err := s.AuthorizedShops(ctx)
	if err != nil {
		return "", err
	}
	for _, sh := range shops {
		if sh.Cipher != "" {
			s.cipher, s.cipherAt = sh.Cipher, time.Now()
			return s.cipher, nil
		}
	}
	return "", ErrTTShopNotConnected
}

// OpenCollaborations lists the products the shop opened for open collaboration (read-only).
// keyword may be a product name or a numeric product id; empty lists everything.
func (s *TTShopService) OpenCollaborations(ctx context.Context, keyword, pageToken string, pageSize int) (*OpenCollabPage, error) {
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, err
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := url.Values{"shop_cipher": {cipher}, "page_size": {strconv.Itoa(pageSize)}}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	body := map[string]string{}
	if kw := strings.TrimSpace(keyword); kw != "" {
		body["keyword"] = kw
		if _, err := strconv.ParseUint(kw, 10, 64); err == nil {
			body["keyword_type"] = "PRODUCT_ID"
		} else {
			body["keyword_type"] = "PRODUCT_NAME"
		}
	}
	data, err := s.postSearch(ctx, ttsOpenCollabSearchPath, q, body)
	if err != nil {
		return nil, err
	}
	return parseOpenCollabs(data)
}

// OpenCollabByProduct returns the open collaboration of one product id, or ErrOpenCollabNotFound.
func (s *TTShopService) OpenCollabByProduct(ctx context.Context, productID string) (*OpenCollabProduct, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return nil, ErrOpenCollabNotFound
	}
	page, err := s.OpenCollaborations(ctx, productID, "", 20)
	if err != nil {
		return nil, err
	}
	for i := range page.Items {
		if page.Items[i].ProductID == productID {
			return &page.Items[i], nil
		}
	}
	return nil, ErrOpenCollabNotFound
}

// OpenCollabSettings reads the shop's open collaboration settings (GET, read-only).
func (s *TTShopService) OpenCollabSettings(ctx context.Context) (*OpenCollabSettings, error) {
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, err
	}
	data, err := s.get(ctx, ttsOpenCollabSettingsPath, url.Values{"shop_cipher": {cipher}})
	if err != nil {
		return nil, err
	}
	var out struct {
		Settings struct {
			AutoAdd struct {
				Rate   int64 `json:"commission_rate"`
				Enable bool  `json:"enable"`
			} `json:"auto_add_product"`
		} `json:"open_collaboration_settings"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errors.New("không đọc được cài đặt cộng tác mở")
	}
	return &OpenCollabSettings{AutoAddEnabled: out.Settings.AutoAdd.Enable, AutoAddRate: ttsRatePercent(out.Settings.AutoAdd.Rate)}, nil
}
