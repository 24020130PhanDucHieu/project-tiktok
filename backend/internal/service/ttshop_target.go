package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Target collaboration ("Cộng tác mục tiêu"): the seller invites chosen creators to promote chosen products at a
// private commission. Creating one REACHES REAL CREATORS and changes the shop, so everything that can be checked
// without calling TikTok (BuildTargetPayload) is checked first, creators and products are re-verified against
// TikTok right before sending, and only the routes in ttsWriteRoutes may write.
//
// API versions follow the newest one in the bundled OpenAPI reference (tts-openapi-guide):
//
//	create  POST   /affiliate_seller/202508/target_collaborations
//	search  POST   /affiliate_seller/202508/target_collaborations/search
//	detail  GET    /affiliate_seller/202508/target_collaborations/{id}
//	link    POST   /affiliate_seller/202509/target_collaboration/{id}/link
//	cancel  DELETE /affiliate_seller/202409/target_collaborations/{id}
//	people  POST   /affiliate_seller/202508/marketplace_creators/search
//	items   POST   /product/202502/products/search  and  GET /product/202309/products/{id}
const (
	ttsCreatorSearchPath = "/affiliate_seller/202508/marketplace_creators/search"
	ttsProductSearchPath = "/product/202502/products/search"
	ttsTargetCreatePath  = "/affiliate_seller/202508/target_collaborations"
	ttsTargetSearchPath  = "/affiliate_seller/202508/target_collaborations/search"
	ttsTargetGetPath     = "/affiliate_seller/202508/target_collaborations/%s"
	ttsTargetLinkPath    = "/affiliate_seller/202509/target_collaboration/%s/link"
	ttsTargetRemovePath  = "/affiliate_seller/202409/target_collaborations/%s"

	// Limits. The first four come from TikTok's documentation; the rest are this app's own safety limits.
	TargetMaxName         = 30
	TargetMaxMessage      = 500
	TargetMinRatePct      = 10.0 // target_commission_rate "must be a minimum of 1000" (create docs)
	TargetMaxRatePct      = 80.0 // commission range is [100, 8000] (detail/update docs)
	TargetMinAdsRatePct   = 1.0
	TargetMaxProducts     = 20 // TikTok allows 100; kept low so every product can be re-verified quickly
	targetVerifyParallel  = 2  // creator search is rate-limited by TikTok
	targetCreatorCacheTTL = 10 * time.Minute
)

// ErrCreatorNotFound is returned when no creator in the Creator Marketplace has exactly that username.
var ErrCreatorNotFound = errors.New("Không tìm thấy nhà sáng tạo này trong Creator Marketplace của TikTok Shop")

// ---------- shapes sent to the browser ----------

// TargetCreator is a creator found in the Creator Marketplace.
type TargetCreator struct {
	OpenID    string `json:"openId"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
	Followers int    `json:"followers"`
}

// ShopProduct is one product of the shop for the picker.
type ShopProduct struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Currency  string `json:"currency"`
	MinPrice  string `json:"minPrice"`
	MaxPrice  string `json:"maxPrice"`
	Inventory int    `json:"inventory"`
}

// ShopProductPage is one page of ShopProduct.
type ShopProductPage struct {
	Items         []ShopProduct `json:"items"`
	NextPageToken string        `json:"nextPageToken"`
	Total         int           `json:"total"`
}

// ---------- input from the browser ----------

// TargetProductInput is a product of the invitation with its commissions in percent.
type TargetProductInput struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	CommissionPct    float64 `json:"commissionPct"`
	AdsCommissionPct float64 `json:"adsCommissionPct"` // 0 = not set
}

// TargetCreatorInput names a creator the way TikTok's search returned it.
type TargetCreatorInput struct {
	OpenID   string `json:"openId"`
	Username string `json:"username"`
}

// TargetInviteInput is everything the invitation form collects.
type TargetInviteInput struct {
	Name          string               `json:"name"`
	EndDate       string               `json:"endDate"` // YYYY-MM-DD, valid through the end of that day (Vietnam time)
	Email         string               `json:"email"`
	Zalo          string               `json:"zalo"`
	Facebook      string               `json:"facebook"`
	ContentPref   string               `json:"contentPref"` // "video" | "live" | ""
	Message       string               `json:"message"`
	HasFreeSample bool                 `json:"hasFreeSample"`
	SampleExempt  bool                 `json:"sampleApprovalExempt"`
	Products      []TargetProductInput `json:"products"`
	Creators      []TargetCreatorInput `json:"creators"`
}

// ---------- body sent to TikTok (exact field names of CreateTargetCollaboration 202508) ----------

type targetAPIProduct struct {
	ID                    string `json:"id"`
	TargetCommissionRate  int    `json:"target_commission_rate"`
	ShopAdsCommissionRate int    `json:"shop_ads_commission_rate,omitempty"`
}

// TargetAPIPayload is the exact request body of CreateTargetCollaboration. The preview shows it unchanged.
type TargetAPIPayload struct {
	Name    string `json:"name"`
	EndTime string `json:"end_time"`
	Message string `json:"message"`
	// SellerContactInfo is REQUIRED by the live API even though the OAS does not say so (code 36009004,
	// "SellerContactInfo is a required field and has not been provided").
	SellerContactInfo struct {
		Email string `json:"email"`
	} `json:"seller_contact_info"`
	FreeSampleRule struct {
		HasFreeSample          bool `json:"has_free_sample"`
		IsSampleApprovalExempt bool `json:"is_sample_approval_exempt"`
	} `json:"free_sample_rule"`
	Products           []targetAPIProduct `json:"products"`
	CreatorUserOpenIDs []string           `json:"creator_user_open_ids"`
}

var (
	reEmail  = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	rePhone  = regexp.MustCompile(`^[0-9+() .-]{6,20}$`)
	reDigits = regexp.MustCompile(`^[0-9]{1,30}$`)
)

// rateUnits converts a percent to TikTok's unit (1/10,000: 10% = 1000).
func rateUnits(pct float64) int { return int(math.Round(pct * 100)) }

// composeInviteMessage is what creators read: the owner's text plus a footer with the contact details and content
// preference that TikTok's API has no field for.
func composeInviteMessage(in TargetInviteInput) string {
	msg := strings.TrimSpace(in.Message)
	var foot []string
	if z := strings.TrimSpace(in.Zalo); z != "" {
		foot = append(foot, "Zalo: "+z)
	}
	if f := strings.TrimSpace(in.Facebook); f != "" {
		foot = append(foot, "Facebook: "+f)
	}
	// The email is NOT repeated here: it goes through seller_contact_info.email, and TikTok rejects messages that
	// carry an address ("request parameters contain sensitive words", code 50001703).
	if len(foot) > 0 {
		msg += "\n\nLiên hệ — " + strings.Join(foot, " · ")
	}
	switch in.ContentPref {
	case "video":
		msg += "\nƯu tiên: video giới thiệu sản phẩm"
	case "live":
		msg += "\nƯu tiên: LIVE giới thiệu sản phẩm"
	}
	return msg
}

// BuildTargetPayload checks the form and builds the exact TikTok request body. It makes no network call.
// problems lists everything wrong in Vietnamese; the payload is only usable when problems is empty.
func BuildTargetPayload(in TargetInviteInput, now time.Time, maxCreators int) (*TargetAPIPayload, []string) {
	var problems []string
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	name := strings.TrimSpace(in.Name)
	switch n := utf8.RuneCountInString(name); {
	case n == 0:
		bad("Hãy đặt tên cho lời mời.")
	case n > TargetMaxName:
		bad("Tên lời mời tối đa %d ký tự (đang %d).", TargetMaxName, n)
	}

	var endTime string
	if day, err := ParseOverviewDay(strings.TrimSpace(in.EndDate)); err != nil {
		bad("Hãy chọn ngày hết hạn của lời mời.")
	} else {
		today := dayStart(now)
		switch {
		case day.Before(today.AddDate(0, 0, 1)):
			bad("Ngày hết hạn phải từ ngày mai trở đi.")
		case day.After(targetLatestEnd):
			bad("Ngày hết hạn không quá ngày %s.", targetLatestEnd.Format("02/01/2006"))
		default:
			endTime = strconv.FormatInt(day.Add(24*time.Hour-time.Second).Unix(), 10) // 23:59:59 that day
		}
	}

	email := strings.TrimSpace(in.Email)
	if !reEmail.MatchString(email) {
		bad("Email liên hệ của shop là bắt buộc (TikTok yêu cầu), hãy nhập email hợp lệ.")
	}
	if z := strings.TrimSpace(in.Zalo); z != "" && !rePhone.MatchString(z) {
		bad("Số Zalo chưa hợp lệ.")
	}
	if utf8.RuneCountInString(strings.TrimSpace(in.Facebook)) > 100 {
		bad("Tài khoản Facebook tối đa 100 ký tự.")
	}
	if in.ContentPref != "" && in.ContentPref != "video" && in.ContentPref != "live" {
		bad("Loại nội dung ưu tiên không hợp lệ.")
	}

	if strings.TrimSpace(in.Message) == "" {
		bad("Hãy nhập nội dung lời mời.")
	}
	message := composeInviteMessage(in)
	if n := utf8.RuneCountInString(message); n > TargetMaxMessage {
		bad("Lời nhắn (kèm thông tin liên hệ) tối đa %d ký tự, đang %d.", TargetMaxMessage, n)
	}

	p := &TargetAPIPayload{Name: name, EndTime: endTime, Message: message}
	p.SellerContactInfo.Email = email
	p.FreeSampleRule.HasFreeSample = in.HasFreeSample
	p.FreeSampleRule.IsSampleApprovalExempt = in.HasFreeSample && in.SampleExempt // exempt has no effect without samples

	if len(in.Products) == 0 {
		bad("Hãy chọn ít nhất một sản phẩm.")
	}
	if len(in.Products) > TargetMaxProducts {
		bad("Mỗi lời mời tối đa %d sản phẩm.", TargetMaxProducts)
	}
	seenProduct := map[string]bool{}
	p.Products = make([]targetAPIProduct, 0, len(in.Products))
	for _, pr := range in.Products {
		id := strings.TrimSpace(pr.ID)
		label := pr.Title
		if label == "" {
			label = id
		}
		switch {
		case !reDigits.MatchString(id):
			bad("Mã sản phẩm %q không hợp lệ.", id)
			continue
		case seenProduct[id]:
			bad("Sản phẩm %q bị chọn hai lần.", label)
			continue
		}
		seenProduct[id] = true
		units := rateUnits(pr.CommissionPct)
		if units < rateUnits(TargetMinRatePct) || units > rateUnits(TargetMaxRatePct) {
			bad("Hoa hồng của %q phải từ %g%% đến %g%%.", label, TargetMinRatePct, TargetMaxRatePct)
			continue
		}
		item := targetAPIProduct{ID: id, TargetCommissionRate: units}
		if pr.AdsCommissionPct != 0 {
			ads := rateUnits(pr.AdsCommissionPct)
			if ads < rateUnits(TargetMinAdsRatePct) || ads > rateUnits(TargetMaxRatePct) {
				bad("Hoa hồng quảng cáo của %q phải từ %g%% đến %g%% (hoặc để trống).", label, TargetMinAdsRatePct, TargetMaxRatePct)
				continue
			}
			item.ShopAdsCommissionRate = ads
		}
		p.Products = append(p.Products, item)
	}

	if maxCreators < 1 {
		maxCreators = 1
	}
	if len(in.Creators) == 0 {
		bad("Hãy chọn ít nhất một nhà sáng tạo.")
	}
	if len(in.Creators) > maxCreators {
		bad("Mỗi lời mời gửi tối đa %d nhà sáng tạo (giới hạn an toàn của ứng dụng).", maxCreators)
	}
	seenCreator := map[string]bool{}
	p.CreatorUserOpenIDs = make([]string, 0, len(in.Creators))
	for _, c := range in.Creators {
		id := strings.TrimSpace(c.OpenID)
		switch {
		case id == "" || strings.TrimSpace(c.Username) == "":
			bad("Thiếu thông tin nhà sáng tạo (cần chọn từ kết quả tìm kiếm).")
		case seenCreator[id]:
			bad("Nhà sáng tạo @%s bị chọn hai lần.", c.Username)
		default:
			seenCreator[id] = true
			p.CreatorUserOpenIDs = append(p.CreatorUserOpenIDs, id)
		}
	}
	return p, problems
}

// ---------- reads ----------

// SearchCreators looks creators up in the Creator Marketplace by username or nickname (read-only).
func (s *TTShopService) SearchCreators(ctx context.Context, keyword string, size int) ([]TargetCreator, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []TargetCreator{}, nil
	}
	if size != 12 { // TikTok accepts only 12 or 20 (checked against the real API)
		size = 20
	}
	// TikTok rate-limits this endpoint hard: the same keyword is answered from the cache, also after a restart.
	return s.creators.do(ctx, strings.ToLower(keyword)+"|"+strconv.Itoa(size), targetCreatorCacheTTL, func(ctx context.Context) ([]TargetCreator, error) {
		cipher, err := s.shopCipher(ctx)
		if err != nil {
			return nil, err
		}
		q := url.Values{"shop_cipher": {cipher}, "page_size": {strconv.Itoa(size)}}
		data, err := s.postSearch(ctx, ttsCreatorSearchPath, q, map[string]string{"keyword": keyword})
		if err != nil {
			return nil, err
		}
		return parseCreators(data)
	})
}

func parseCreators(data json.RawMessage) ([]TargetCreator, error) {
	var out struct {
		Creators []struct {
			OpenID    string `json:"creator_open_id"`
			Username  string `json:"username"`
			Nickname  string `json:"nickname"`
			Followers int    `json:"follower_count"`
			Avatar    struct {
				URL string `json:"url"`
			} `json:"avatar"`
		} `json:"creators"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errors.New("không đọc được danh sách nhà sáng tạo")
	}
	list := make([]TargetCreator, 0, len(out.Creators))
	for _, c := range out.Creators {
		if c.OpenID == "" {
			continue
		}
		list = append(list, TargetCreator{OpenID: c.OpenID, Username: c.Username, Nickname: c.Nickname, AvatarURL: c.Avatar.URL, Followers: c.Followers})
	}
	return list, nil
}

// ResolveCreator finds the creator whose username is EXACTLY the given one (case-insensitive). Searching by a
// near-match would risk inviting somebody else.
func (s *TTShopService) ResolveCreator(ctx context.Context, username string) (*TargetCreator, error) {
	want := strings.TrimPrefix(strings.TrimSpace(username), "@")
	if want == "" {
		return nil, ErrCreatorNotFound
	}
	list, err := s.SearchCreators(ctx, want, 20)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if strings.EqualFold(list[i].Username, want) {
			return &list[i], nil
		}
	}
	return nil, ErrCreatorNotFound
}

// ListProducts lists the shop's active products for the picker (read-only; TikTok has no title filter, so the
// browser filters the loaded page).
func (s *TTShopService) ListProducts(ctx context.Context, pageToken string, size int) (*ShopProductPage, error) {
	if size < 1 || size > 100 {
		size = 50
	}
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{"shop_cipher": {cipher}, "page_size": {strconv.Itoa(size)}}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	data, err := s.postSearch(ctx, ttsProductSearchPath, q, map[string]string{"status": "ACTIVATE"})
	if err != nil {
		return nil, err
	}
	return parseShopProducts(data)
}

func parseShopProducts(data json.RawMessage) (*ShopProductPage, error) {
	var out struct {
		NextPageToken string `json:"next_page_token"`
		TotalCount    int    `json:"total_count"`
		Products      []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
			Skus   []struct {
				Price struct {
					Currency          string `json:"currency"`
					SalePrice         string `json:"sale_price"`
					TaxExclusivePrice string `json:"tax_exclusive_price"`
				} `json:"price"`
				Inventory []struct {
					Quantity int `json:"quantity"`
				} `json:"inventory"`
			} `json:"skus"`
		} `json:"products"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errors.New("không đọc được danh sách sản phẩm")
	}
	page := &ShopProductPage{Items: make([]ShopProduct, 0, len(out.Products)), NextPageToken: out.NextPageToken, Total: out.TotalCount}
	for _, p := range out.Products {
		it := ShopProduct{ID: p.ID, Title: p.Title, Status: p.Status}
		lo, hi := math.MaxFloat64, 0.0
		for _, k := range p.Skus {
			amount := k.Price.SalePrice
			if amount == "" {
				amount = k.Price.TaxExclusivePrice // local sellers get this field instead of sale_price
			}
			if v, ok := ttsAmount(amount); ok {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
				it.Currency = k.Price.Currency
			}
			for _, inv := range k.Inventory {
				it.Inventory += inv.Quantity
			}
		}
		if hi > 0 {
			it.MinPrice, it.MaxPrice = strconv.FormatFloat(lo, 'f', -1, 64), strconv.FormatFloat(hi, 'f', -1, 64)
		}
		page.Items = append(page.Items, it)
	}
	return page, nil
}

// productState reads a product's current title and status (read-only).
func (s *TTShopService) productState(ctx context.Context, cipher, id string) (title, status string, err error) {
	raw, err := s.get(ctx, ttsProductGetPath+id, url.Values{"shop_cipher": {cipher}})
	if err != nil {
		return "", "", err
	}
	var p struct {
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", "", errors.New("không đọc được sản phẩm")
	}
	return p.Title, p.Status, nil
}

// TargetPreview is what the form shows before anything is sent.
type TargetPreview struct {
	Payload  *TargetAPIPayload `json:"payload"`
	Problems []string          `json:"problems"`
	Creators []TargetCreator   `json:"creators"`
	Products []ShopProduct     `json:"products"`
	Ready    bool              `json:"ready"`
}

// PreviewTarget validates the form, then re-checks with TikTok that every creator really exists with that exact
// username and open id and that every product is on sale. It sends nothing.
func (s *TTShopService) PreviewTarget(ctx context.Context, in TargetInviteInput, now time.Time, maxCreators int) (*TargetPreview, error) {
	payload, problems := BuildTargetPayload(in, now, maxCreators)
	if problems == nil {
		problems = []string{} // JSON must say [] (not null): the page reads problems.length
	}
	pv := &TargetPreview{Payload: payload, Problems: problems, Creators: []TargetCreator{}, Products: []ShopProduct{}}
	if len(problems) > 0 {
		return pv, nil
	}
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, err
	}

	type cres struct {
		c   *TargetCreator
		err error
	}
	type pres struct {
		title, status string
		err           error
	}
	creators := make([]cres, len(in.Creators))
	products := make([]pres, len(in.Products))
	sem := make(chan struct{}, targetVerifyParallel)
	var wg sync.WaitGroup
	for i := range in.Creators {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c, err := s.ResolveCreator(ctx, in.Creators[i].Username)
			creators[i] = cres{c, err}
		}(i)
	}
	for i := range in.Products {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			t, st, err := s.productState(ctx, cipher, strings.TrimSpace(in.Products[i].ID))
			products[i] = pres{t, st, err}
		}(i)
	}
	wg.Wait()

	for i, r := range creators {
		in := in.Creators[i]
		switch {
		case errors.Is(r.err, ErrCreatorNotFound):
			pv.Problems = append(pv.Problems, fmt.Sprintf("Không tìm thấy @%s trong Creator Marketplace.", strings.TrimPrefix(in.Username, "@")))
		case r.err != nil:
			return nil, r.err
		case r.c.OpenID != strings.TrimSpace(in.OpenID):
			pv.Problems = append(pv.Problems, fmt.Sprintf("Mã nhà sáng tạo của @%s không khớp với TikTok, hãy chọn lại từ kết quả tìm kiếm.", r.c.Username))
		default:
			pv.Creators = append(pv.Creators, *r.c)
		}
	}
	for i, r := range products {
		in := in.Products[i]
		label := in.Title
		if label == "" {
			label = in.ID
		}
		switch {
		case r.err != nil:
			return nil, r.err
		case r.status != "ACTIVATE":
			pv.Problems = append(pv.Problems, fmt.Sprintf("Sản phẩm %q không đang bán (trạng thái %s).", label, r.status))
		default:
			pv.Products = append(pv.Products, ShopProduct{ID: strings.TrimSpace(in.ID), Title: r.title, Status: r.status})
		}
	}
	pv.Ready = len(pv.Problems) == 0
	return pv, nil
}

// ---------- the write ----------

// TargetConflict is a creator/product pair TikTok refused because it already sits in another invitation.
type TargetConflict struct {
	CreatorOpenID string `json:"creatorOpenId"`
	ProductID     string `json:"productId"`
}

// ErrTargetConflict is returned (with the pairs) when TikTok created nothing because of conflicts.
type ErrTargetConflict struct{ Conflicts []TargetConflict }

func (e *ErrTargetConflict) Error() string {
	return fmt.Sprintf("TikTok không tạo lời mời vì %d cặp nhà sáng tạo/sản phẩm đã nằm trong lời mời khác", len(e.Conflicts))
}

// CreateTarget sends the invitation. It must only be called with a payload from BuildTargetPayload whose creators
// and products were verified by PreviewTarget in the same request. Returns the new target collaboration id.
func (s *TTShopService) CreateTarget(ctx context.Context, payload *TargetAPIPayload) (string, error) {
	if payload == nil {
		return "", errors.New("thiếu nội dung lời mời")
	}
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return "", err
	}
	data, err := s.callWrite(ctx, "POST", ttsTargetCreatePath, url.Values{"shop_cipher": {cipher}}, payload)
	if err != nil {
		return "", err
	}
	// The cached lists are kept on purpose: the screen adds the invitation from our own record (see the list handler),
	// so no extra request goes to TikTok right after the one that just created it. Calls here are what hit TikTok's
	// shared rate limit, and a list asked for in that moment is the one that gets refused.
	return parseTargetCreated(data)
}

func parseTargetCreated(data json.RawMessage) (string, error) {
	var out struct {
		Target struct {
			ID string `json:"id"`
		} `json:"target_collaboration"`
		Conflicts []struct {
			CreatorOpenID string `json:"creator_user_open_id"`
			ProductID     string `json:"product_id"`
		} `json:"target_collaboration_conflicts"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", errors.New("không đọc được phản hồi tạo lời mời")
	}
	if out.Target.ID == "" {
		if len(out.Conflicts) > 0 {
			e := &ErrTargetConflict{}
			for _, c := range out.Conflicts {
				e.Conflicts = append(e.Conflicts, TargetConflict{CreatorOpenID: c.CreatorOpenID, ProductID: c.ProductID})
			}
			return "", e
		}
		return "", errors.New("TikTok Shop không trả về mã lời mời")
	}
	return out.Target.ID, nil
}

// TargetLink creates the link a creator opens to review and accept an invitation.
func (s *TTShopService) TargetLink(ctx context.Context, id string) (string, error) {
	if !reDigits.MatchString(id) {
		return "", ErrBadProductID
	}
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return "", err
	}
	data, err := s.callWrite(ctx, "POST", fmt.Sprintf(ttsTargetLinkPath, id), url.Values{"shop_cipher": {cipher}}, map[string]string{})
	if err != nil {
		return "", err
	}
	var out struct {
		Link string `json:"link"`
	}
	if json.Unmarshal(data, &out) != nil || strings.TrimSpace(out.Link) == "" {
		return "", errors.New("TikTok Shop không trả về link lời mời")
	}
	return strings.TrimSpace(out.Link), nil
}

// ttsOperationNotAllowed is the code TikTok answers when what is asked cannot be done in the invitation's state.
const ttsOperationNotAllowed = 98001004

// IsInvitationEnded reports whether TikTok refused because the invitation is already terminated or being terminated.
func IsInvitationEnded(err error) bool {
	var rej *TTSRejected
	return errors.As(err, &rej) && rej.Code == ttsOperationNotAllowed && strings.Contains(strings.ToLower(rej.Message), "terminat")
}

// RemoveTarget cancels an invitation.
func (s *TTShopService) RemoveTarget(ctx context.Context, id string) error {
	if !reDigits.MatchString(id) {
		return ErrBadProductID
	}
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return err
	}
	// The cached lists are kept: the list handler hides an invitation cancelled from here, and TikTok's next list drops it.
	_, err = s.callWrite(ctx, "DELETE", fmt.Sprintf(ttsTargetRemovePath, id), url.Values{"shop_cipher": {cipher}}, nil)
	return err
}

// ---------- list and detail ----------

// TargetSummary is one invitation of the management table.
type TargetSummary struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Type                 string `json:"type"`
	Message              string `json:"message"`
	StartTime            int64  `json:"startTime"`
	EndTime              int64  `json:"endTime"`
	UpdateTime           int64  `json:"updateTime"`
	ProductCount         int    `json:"productCount"`
	InvitedCount         int    `json:"invitedCount"`
	ShowcaseCreatorCount int    `json:"showcaseCreatorCount"` // creators who added the product to their showcase
	ContentCreatorCount  int    `json:"contentCreatorCount"`  // creators who posted content
	HasFreeSample        bool   `json:"hasFreeSample"`
	SampleExempt         bool   `json:"sampleApprovalExempt"`
}

// TargetPage is one page of invitations.
type TargetPage struct {
	Items         []TargetSummary `json:"items"`
	NextPageToken string          `json:"nextPageToken"`
	Total         int             `json:"total"`
	Stale         bool            `json:"stale,omitempty"` // TikTok refused the request, this is the last list it gave
}

// TargetListParams are the filters of SearchTargetCollaborations.
type TargetListParams struct {
	Status        string // ONGOING | EXPIRING | VALID | CANCELING | COMPLETED; "" means VALID (TikTok requires one)
	AcceptStatus  string // ACCEPT | ALL | ""
	SampleSetting string // WITH_FREE_SAMPLE | ALL; "" means ALL
	Keyword       string
	KeywordType   string // PRODUCT_ID | PRODUCT_NAME | TARGET_COLLABORATION_ID | TARGET_COLLABORATION_NAME
	PageToken     string
	PageSize      int
	Fresh         bool // skip the short cache and ask TikTok now (the "Làm mới" button)
}

var (
	targetStatuses     = map[string]bool{"ONGOING": true, "EXPIRING": true, "VALID": true, "CANCELING": true, "COMPLETED": true}
	targetAccept       = map[string]bool{"ACCEPT": true, "ALL": true}
	targetSample       = map[string]bool{"WITH_FREE_SAMPLE": true, "ALL": true}
	targetKeywordTypes = map[string]bool{"PRODUCT_ID": true, "PRODUCT_NAME": true, "TARGET_COLLABORATION_ID": true, "TARGET_COLLABORATION_NAME": true}
)

// buildTargetSearchBody builds the SearchTargetCollaborations request body from the filters.
func buildTargetSearchBody(p TargetListParams) (map[string]any, error) {
	// TikTok rejects the search ("invalid params" / "is a required field") unless all three filters are present.
	if p.Status == "" {
		p.Status = "VALID"
	}
	if p.AcceptStatus == "" {
		p.AcceptStatus = "ALL"
	}
	if p.SampleSetting == "" {
		p.SampleSetting = "ALL"
	}
	if !targetStatuses[p.Status] || !targetAccept[p.AcceptStatus] || !targetSample[p.SampleSetting] {
		return nil, errors.New("bộ lọc không hợp lệ")
	}
	body := map[string]any{"collaboration_status": p.Status, "creator_accept_status": p.AcceptStatus, "free_sample_setting": p.SampleSetting}
	if kw := strings.TrimSpace(p.Keyword); kw != "" {
		kt := p.KeywordType
		if kt == "" {
			kt = "TARGET_COLLABORATION_NAME"
		}
		if !targetKeywordTypes[kt] {
			return nil, errors.New("kiểu tìm kiếm không hợp lệ")
		}
		body["search_param"] = map[string]string{"keyword": kw, "keyword_type": kt}
	}
	return body, nil
}

// searchTargetsLive asks TikTok for one page of the invitations (read-only); SearchTargets adds the cache.
// targetLatestEnd is the last day an invitation may end on. TikTok documents no upper limit, and the shop's own
// invitations made in Seller Center end as late as 31/12/2050; the app used to cap it at one year.
var targetLatestEnd = time.Date(2050, 12, 31, 0, 0, 0, 0, vnZone)

func (s *TTShopService) searchTargetsLive(ctx context.Context, p TargetListParams) (*TargetPage, error) {
	body, err := buildTargetSearchBody(p)
	if err != nil {
		return nil, err
	}
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, err
	}
	// TikTok answers "invalid params" to unusual page sizes (3 and 5 were refused, 20 works): always ask for 20.
	q := url.Values{"shop_cipher": {cipher}, "page_size": {"20"}}
	if p.PageToken != "" {
		q.Set("page_token", p.PageToken)
	}
	data, err := s.postSearch(ctx, ttsTargetSearchPath, q, body)
	if err != nil {
		return nil, err
	}
	return parseTargetPage(data)
}

func parseTargetPage(data json.RawMessage) (*TargetPage, error) {
	var out struct {
		NextPageToken string `json:"next_page_token"`
		TotalCount    int    `json:"total_count"`
		Items         []struct {
			ID                   string `json:"id"`
			Name                 string `json:"name"`
			Type                 string `json:"type"`
			Message              string `json:"message"`
			StartTime            int64  `json:"start_time"`
			EndTime              int64  `json:"end_time"`
			UpdateTime           int64  `json:"update_time"`
			ProductCount         int    `json:"product_count"`
			InvitedCount         int    `json:"creator_inivited_count"` // sic: TikTok's field name has this typo
			ShowcaseCreatorCount int    `json:"showcase_creator_count"`
			ContentCreatorCount  int    `json:"content_creator_count"`
			FreeSampleRule       struct {
				Has    bool `json:"has_free_sample"`
				Exempt bool `json:"is_sample_approval_exempt"`
			} `json:"free_sample_rule"`
		} `json:"target_collaborations"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errors.New("không đọc được danh sách lời mời")
	}
	page := &TargetPage{Items: make([]TargetSummary, 0, len(out.Items)), NextPageToken: out.NextPageToken, Total: out.TotalCount}
	for _, t := range out.Items {
		page.Items = append(page.Items, TargetSummary{ID: t.ID, Name: t.Name, Type: t.Type, Message: t.Message, StartTime: t.StartTime, EndTime: t.EndTime,
			UpdateTime: t.UpdateTime, ProductCount: t.ProductCount, InvitedCount: t.InvitedCount, ShowcaseCreatorCount: t.ShowcaseCreatorCount,
			ContentCreatorCount: t.ContentCreatorCount, HasFreeSample: t.FreeSampleRule.Has, SampleExempt: t.FreeSampleRule.Exempt})
	}
	return page, nil
}

// TargetDetailCreator is a creator inside an invitation.
type TargetDetailCreator struct {
	OpenID          string `json:"openId"`
	Username        string `json:"username"`
	Nickname        string `json:"nickname"`
	AvatarURL       string `json:"avatarUrl"`
	Status          string `json:"status"` // NORMAL | DELETING | DELETED
	ShowcaseProduct int    `json:"showcaseProductCount"`
	ContentProduct  int    `json:"contentProductCount"`
}

// TargetDetailProduct is a product inside an invitation.
type TargetDetailProduct struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	ImageURL      string  `json:"imageUrl"`
	Status        string  `json:"status"`
	Currency      string  `json:"currency"`
	MinPrice      string  `json:"minPrice"`
	MaxPrice      string  `json:"maxPrice"`
	CommissionPct float64 `json:"commissionPct"`
	AdsPct        float64 `json:"adsCommissionPct"`
}

// TargetDetail is the full invitation.
type TargetDetail struct {
	TargetSummary
	Email    string                `json:"email"`
	Creators []TargetDetailCreator `json:"creators"`
	Products []TargetDetailProduct `json:"products"`
}

// GetTarget reads one invitation (read-only).
func (s *TTShopService) GetTarget(ctx context.Context, id string) (*TargetDetail, error) {
	if !reDigits.MatchString(id) {
		return nil, ErrBadProductID
	}
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, err
	}
	data, err := s.get(ctx, fmt.Sprintf(ttsTargetGetPath, id), url.Values{"shop_cipher": {cipher}})
	if err != nil {
		return nil, err
	}
	return parseTargetDetail(data)
}

func parseTargetDetail(data json.RawMessage) (*TargetDetail, error) {
	var out struct {
		T struct {
			ID                   string `json:"id"`
			Name                 string `json:"name"`
			Type                 string `json:"type"`
			Message              string `json:"message"`
			StartTime            int64  `json:"start_time"`
			EndTime              int64  `json:"end_time"`
			UpdateTime           int64  `json:"update_time"`
			ProductCount         int    `json:"product_count"`
			InvitedCount         int    `json:"creator_invited_count"`
			ShowcaseCreatorCount int    `json:"showcase_creator_count"`
			ContentCreatorCount  int    `json:"content_creator_count"`
			FreeSampleRule       struct {
				Has    bool `json:"has_free_sample"`
				Exempt bool `json:"is_sample_approval_exempt"`
			} `json:"free_sample_rule"`
			SellerContactInfo struct {
				Email string `json:"email"`
			} `json:"seller_contact_info"`
			Creators []struct {
				OpenID    string `json:"creator_open_id"`
				Username  string `json:"username"`
				Nickname  string `json:"nickname"`
				Status    string `json:"collaboration_status"`
				Showcase  int    `json:"showcase_product_count"`
				Content   int    `json:"content_product_count"`
				AvatarURL struct {
					URL string `json:"url"`
				} `json:"avatar"`
			} `json:"creators"`
			Products []struct {
				ID     string `json:"id"`
				Title  string `json:"title"`
				Image  string `json:"main_image_url"`
				Status string `json:"status"`
				Price  struct {
					Currency string `json:"currency"`
					Min      string `json:"minimum_amount"`
					Max      string `json:"maximum_amount"`
				} `json:"original_price"`
				Commission struct {
					Rate int `json:"rate"`
					Ads  int `json:"shop_ads_commission_rate"`
				} `json:"commission"`
			} `json:"products"`
		} `json:"target_collaboration"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errors.New("không đọc được chi tiết lời mời")
	}
	t := out.T
	d := &TargetDetail{
		TargetSummary: TargetSummary{ID: t.ID, Name: t.Name, Type: t.Type, Message: t.Message, StartTime: t.StartTime, EndTime: t.EndTime, UpdateTime: t.UpdateTime,
			ProductCount: t.ProductCount, InvitedCount: t.InvitedCount, ShowcaseCreatorCount: t.ShowcaseCreatorCount, ContentCreatorCount: t.ContentCreatorCount,
			HasFreeSample: t.FreeSampleRule.Has, SampleExempt: t.FreeSampleRule.Exempt},
		Email: t.SellerContactInfo.Email, Creators: []TargetDetailCreator{}, Products: []TargetDetailProduct{},
	}
	for _, c := range t.Creators {
		d.Creators = append(d.Creators, TargetDetailCreator{OpenID: c.OpenID, Username: c.Username, Nickname: c.Nickname, AvatarURL: c.AvatarURL.URL,
			Status: c.Status, ShowcaseProduct: c.Showcase, ContentProduct: c.Content})
	}
	for _, p := range t.Products {
		d.Products = append(d.Products, TargetDetailProduct{ID: p.ID, Title: p.Title, ImageURL: p.Image, Status: p.Status, Currency: p.Price.Currency,
			MinPrice: p.Price.Min, MaxPrice: p.Price.Max, CommissionPct: ttsRatePercent(int64(p.Commission.Rate)), AdsPct: ttsRatePercent(int64(p.Commission.Ads))})
	}
	return d, nil
}
