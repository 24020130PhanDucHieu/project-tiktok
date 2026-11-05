package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Real sales and content of creators, read from TikTok Shop (read-only):
//   - POST /affiliate_seller/202410/orders/search  every affiliate order with its creator, product, content id
//     (the video) and commission. Verified against the live shop: no order-level status, the state is in
//     settlement_status and fully_return; a query may span at most 3 months.
//   - GET  /analytics/202509/shop_videos/performance and /{video_id}/performance  views, likes, comments, shares.
//     These need the Analytics scope; without it TikTok answers code 105005 and views stay unknown ("—").

const (
	ttsAffOrdersPath   = "/affiliate_seller/202410/orders/search"
	ttsVideoListPath   = "/analytics/202509/shop_videos/performance"
	ttsVideoDetailPath = "/analytics/202509/shop_videos/%s/performance"

	affOrderWindowDays = 88 // the API refuses a range longer than 3 months
	affOrderPageSize   = 100
	affOrderMaxPages   = 40 // 4,000 orders per refresh; the answer says when it was cut
	affOrdersTTL       = 3 * time.Minute
	affOrdersKeep      = 4
	videoListMaxPages  = 10
	videoTrafficTTL    = 10 * time.Minute
	analyticsDenyTTL   = 10 * time.Minute

	ttsCodeNoScope = 105005

	analyticsMaxDays = 58 // the analytics API refuses a date range over 60 days (verified: code 28001022)
)

// dayWindows splits [from, to] (Vietnam days, both inclusive) into windows of at most maxDays days, as the
// start_date_ge / end_date_lt pairs the analytics API wants (end exclusive).
func dayWindows(from, to time.Time, maxDays int) [][2]string {
	day := func(t time.Time) time.Time {
		v := t.In(vnContentZone)
		return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, vnContentZone)
	}
	start, end := day(from), day(to).AddDate(0, 0, 1)
	var out [][2]string
	for start.Before(end) {
		next := start.AddDate(0, 0, maxDays)
		if next.After(end) {
			next = end
		}
		out = append(out, [2]string{start.Format("2006-01-02"), next.Format("2006-01-02")})
		start = next
	}
	return out
}

// ErrAnalyticsScope means the app has not been granted the Analytics scope, so views and likes cannot be read.
var ErrAnalyticsScope = errors.New("ứng dụng chưa được cấp quyền Analytics của TikTok Shop")

// vnZone is the shop's timezone (Vietnam).
var vnContentZone = time.FixedZone("ICT", 7*3600)

// ---------- affiliate orders ----------

// AffiliateLine is one SKU line of an affiliate order.
type AffiliateLine struct {
	OrderID       string
	CreatedAt     time.Time
	Creator       string // TikTok username, lower case
	ContentID     string
	ContentType   string // VIDEO, LIVE, SHOP, LINKSHARE...
	ProductID     string
	Quantity      int
	Gross         float64 // sales value before refunds
	Net           float64 // sales value after refunds/returns (actual when TikTok has settled it)
	Commission    float64 // commission to the creator + Shop Ads commission (actual when settled, else estimated)
	Settlement    string  // SETTLED, To-SETTLE, AWAITING PAYMENT, INELIGIBLE
	FullyReturned bool
	TargetID      string
	OpenID        string
}

// Valid reports whether the line counts as a real sale: it was not fully returned and is not marked ineligible.
func (l AffiliateLine) Valid() bool {
	return !l.FullyReturned && !strings.EqualFold(l.Settlement, "INELIGIBLE")
}

// Confirmed reports whether TikTok has settled the commission (the amount is final).
func (l AffiliateLine) Confirmed() bool { return strings.EqualFold(l.Settlement, "SETTLED") }

type tsMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func (m tsMoney) value() (float64, bool) {
	if strings.TrimSpace(m.Amount) == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(m.Amount), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

type rawAffSKU struct {
	ContentID               string  `json:"content_id"`
	ContentType             string  `json:"content_type"`
	CreatorUsername         string  `json:"creator_username"`
	ProductID               string  `json:"product_id"`
	Quantity                int     `json:"quantity"`
	RefundedQuantity        int     `json:"refunded_quantity"`
	ReturnedQuantity        int     `json:"returned_quantity"`
	FullyReturn             string  `json:"fully_return"`
	SettlementStatus        string  `json:"settlement_status"`
	TargetCollaborationID   string  `json:"target_collaboration_id"`
	OpenCollaborationID     string  `json:"open_collaboration_id"`
	EstimatedCommissionBase tsMoney `json:"estimated_commission_base"`
	ActualCommissionBase    tsMoney `json:"actual_commission_base"`
	EstimatedPaid           tsMoney `json:"estimated_paid_commission"`
	ActualPaid              tsMoney `json:"actual_paid_commission"`
	EstimatedAds            tsMoney `json:"estimated_paid_shop_ads_commission"`
	ActualAds               tsMoney `json:"actual_paid_shop_ads_commission"`
}

func firstMoney(actual, estimated tsMoney) float64 {
	if v, ok := actual.value(); ok {
		return v
	}
	v, _ := estimated.value()
	return v
}

// lineOf turns one SKU of an order into an AffiliateLine.
func lineOf(orderID string, created int64, k rawAffSKU) AffiliateLine {
	gross, _ := k.EstimatedCommissionBase.value()
	net := gross
	if v, ok := k.ActualCommissionBase.value(); ok {
		net = v // already after refunds
	} else if lost := k.RefundedQuantity + k.ReturnedQuantity; lost > 0 && k.Quantity > 0 {
		keep := k.Quantity - lost
		if keep < 0 {
			keep = 0
		}
		net = gross * float64(keep) / float64(k.Quantity)
	}
	return AffiliateLine{
		OrderID: orderID, CreatedAt: time.Unix(created, 0), Creator: strings.ToLower(strings.TrimPrefix(strings.TrimSpace(k.CreatorUsername), "@")),
		ContentID: k.ContentID, ContentType: strings.ToUpper(k.ContentType), ProductID: k.ProductID, Quantity: k.Quantity,
		Gross: gross, Net: net,
		Commission:    firstMoney(k.ActualPaid, k.EstimatedPaid) + firstMoney(k.ActualAds, k.EstimatedAds),
		Settlement:    k.SettlementStatus,
		FullyReturned: strings.EqualFold(k.FullyReturn, "Yes"),
		TargetID:      k.TargetCollaborationID, OpenID: k.OpenCollaborationID,
	}
}

// parseAffiliateOrders reads one page of orders/search.
func parseAffiliateOrders(data json.RawMessage) (lines []AffiliateLine, next string, total int, err error) {
	var d struct {
		Next   string `json:"next_page_token"`
		Total  int    `json:"total_count"`
		Orders []struct {
			ID         string      `json:"id"`
			CreateTime int64       `json:"create_time"`
			SKUs       []rawAffSKU `json:"skus"`
		} `json:"orders"`
	}
	if err = json.Unmarshal(data, &d); err != nil {
		return nil, "", 0, err
	}
	for _, o := range d.Orders {
		for _, k := range o.SKUs {
			lines = append(lines, lineOf(o.ID, o.CreateTime, k))
		}
	}
	return lines, d.Next, d.Total, nil
}

type affCacheEntry struct {
	from, to  time.Time
	at        time.Time
	lines     []AffiliateLine
	truncated bool
}

// AffiliateLines returns every affiliate order line created between from and to, newest data first-page ordering is
// irrelevant. It splits the range into windows of at most 88 days (the API limit), pages through them, and keeps
// the answer for a few minutes so several panels opening together cost one set of calls. truncated is true when the
// page cap cut the list.
func (s *TTShopService) AffiliateLines(ctx context.Context, from, to time.Time) ([]AffiliateLine, bool, error) {
	if !to.After(from) {
		return nil, false, nil
	}
	s.contentMu.Lock()
	defer s.contentMu.Unlock()
	for _, e := range s.affCache {
		if time.Since(e.at) < affOrdersTTL && !e.from.After(from) && !e.to.Before(to.Add(-affOrdersTTL)) {
			return linesSince(e.lines, from), e.truncated, nil
		}
	}
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, false, err
	}
	var all []AffiliateLine
	truncated := false
	pages := 0
	for end := to; end.After(from); {
		start := end.AddDate(0, 0, -affOrderWindowDays)
		if start.Before(from) {
			start = from
		}
		token := ""
		for {
			if pages >= affOrderMaxPages {
				truncated = true
				break
			}
			q := url.Values{"shop_cipher": {cipher}, "page_size": {strconv.Itoa(affOrderPageSize)}}
			if token != "" {
				q.Set("page_token", token)
			}
			raw, err := s.postSearch(ctx, ttsAffOrdersPath, q, map[string]any{"create_time_ge": start.Unix(), "create_time_lt": end.Unix()})
			if err != nil {
				return nil, false, err
			}
			pages++
			lines, next, _, err := parseAffiliateOrders(raw)
			if err != nil {
				return nil, false, errors.New("TikTok trả về danh sách đơn không đọc được")
			}
			all = append(all, lines...)
			if next == "" {
				break
			}
			token = next
		}
		if truncated {
			break
		}
		end = start
	}
	s.affCache = append([]affCacheEntry{{from: from, to: to, at: time.Now(), lines: all, truncated: truncated}}, s.affCache...)
	if len(s.affCache) > affOrdersKeep {
		s.affCache = s.affCache[:affOrdersKeep]
	}
	return all, truncated, nil
}

func linesSince(lines []AffiliateLine, from time.Time) []AffiliateLine {
	out := make([]AffiliateLine, 0, len(lines))
	for _, l := range lines {
		if !l.CreatedAt.Before(from) {
			out = append(out, l)
		}
	}
	return out
}

// OrderFilter narrows lines to one creator, some products and a start time. Empty fields do not filter.
type OrderFilter struct {
	Creator    string
	ProductIDs map[string]bool
	From       time.Time
}

func (f OrderFilter) match(l AffiliateLine) bool {
	if f.Creator != "" && l.Creator != strings.ToLower(strings.TrimPrefix(f.Creator, "@")) {
		return false
	}
	if len(f.ProductIDs) > 0 && !f.ProductIDs[l.ProductID] {
		return false
	}
	return f.From.IsZero() || !l.CreatedAt.Before(f.From)
}

// OrderTotals is the sum of the valid lines that match a filter.
type OrderTotals struct {
	Orders              int     `json:"orders"`
	Items               int     `json:"items"`
	GMV                 float64 `json:"gmv"`
	Commission          float64 `json:"commission"`
	CommissionConfirmed float64 `json:"commissionConfirmed"` // part TikTok has already settled
	Excluded            int     `json:"excluded"`            // orders left out: fully returned or ineligible
	Videos              int     `json:"videos"`
	NonVideoOrders      int     `json:"nonVideoOrders"` // sales through LIVE, shop page or link share, still counted above
}

// VideoSales is the sales of one video.
type VideoSales struct {
	ContentID  string    `json:"id"`
	Creator    string    `json:"username"`
	ProductIDs []string  `json:"productIds"`
	Orders     int       `json:"orders"`
	Items      int       `json:"items"`
	GMV        float64   `json:"gmv"`
	Commission float64   `json:"commission"`
	Confirmed  float64   `json:"commissionConfirmed"`
	FirstOrder time.Time `json:"firstOrderAt"`
	LastOrder  time.Time `json:"lastOrderAt"`
}

// AggregateOrders sums the lines that match the filter. Orders are counted once even when an order has several SKU
// lines; lines that were fully returned or are ineligible are counted in Excluded and never in the money.
func AggregateOrders(lines []AffiliateLine, f OrderFilter) (OrderTotals, []VideoSales) {
	var tot OrderTotals
	orders, nonVideo, excluded := map[string]bool{}, map[string]bool{}, map[string]bool{}
	type acc struct {
		v      VideoSales
		orders map[string]bool
		prods  map[string]bool
	}
	byVideo := map[string]*acc{}
	for _, l := range lines {
		if !f.match(l) {
			continue
		}
		if !l.Valid() {
			excluded[l.OrderID] = true
			continue
		}
		orders[l.OrderID] = true
		tot.Items += l.Quantity
		tot.GMV += l.Net
		tot.Commission += l.Commission
		if l.Confirmed() {
			tot.CommissionConfirmed += l.Commission
		}
		if l.ContentType != "VIDEO" || l.ContentID == "" {
			nonVideo[l.OrderID] = true
			continue
		}
		a := byVideo[l.ContentID]
		if a == nil {
			a = &acc{v: VideoSales{ContentID: l.ContentID, Creator: l.Creator}, orders: map[string]bool{}, prods: map[string]bool{}}
			byVideo[l.ContentID] = a
		}
		a.orders[l.OrderID] = true
		a.prods[l.ProductID] = true
		a.v.Items += l.Quantity
		a.v.GMV += l.Net
		a.v.Commission += l.Commission
		if l.Confirmed() {
			a.v.Confirmed += l.Commission
		}
		if a.v.FirstOrder.IsZero() || l.CreatedAt.Before(a.v.FirstOrder) {
			a.v.FirstOrder = l.CreatedAt
		}
		if l.CreatedAt.After(a.v.LastOrder) {
			a.v.LastOrder = l.CreatedAt
		}
	}
	videos := make([]VideoSales, 0, len(byVideo))
	for _, a := range byVideo {
		a.v.Orders = len(a.orders)
		for p := range a.prods {
			a.v.ProductIDs = append(a.v.ProductIDs, p)
		}
		sort.Strings(a.v.ProductIDs)
		videos = append(videos, a.v)
	}
	sort.Slice(videos, func(i, j int) bool {
		if videos[i].GMV != videos[j].GMV {
			return videos[i].GMV > videos[j].GMV
		}
		return videos[i].ContentID > videos[j].ContentID
	})
	tot.Orders, tot.Excluded, tot.Videos, tot.NonVideoOrders = len(orders), len(excluded), len(videos), len(nonVideo)
	return tot, videos
}

// VideoPostedAt decodes the posting time of a TikTok video from its id (the upper 32 bits are a Unix time).
// It returns the zero time when the id does not look like one.
func VideoPostedAt(id string) time.Time {
	n, err := strconv.ParseUint(strings.TrimSpace(id), 10, 64)
	if err != nil {
		return time.Time{}
	}
	sec := int64(n >> 32)
	if sec < 1_420_070_400 || sec > time.Now().Add(24*time.Hour).Unix() { // 2015-01-01 .. tomorrow
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// VideoURL is the public page of a video.
func VideoURL(username, id string) string {
	return fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", strings.TrimPrefix(username, "@"), id)
}

// ---------- video analytics (needs the Analytics scope) ----------

// AnalyticsVideo is one row of the shop's video performance list.
type AnalyticsVideo struct {
	ID        string
	Title     string
	Username  string
	PostedAt  time.Time
	Views     int64
	ItemsSold int
	Products  []string
}

type tsNumber string

func (f *tsNumber) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "null" {
		s = ""
	}
	*f = tsNumber(s)
	return nil
}

func (f tsNumber) int64() int64 {
	if f == "" {
		return 0
	}
	if v, err := strconv.ParseInt(string(f), 10, 64); err == nil {
		return v
	}
	if v, err := strconv.ParseFloat(string(f), 64); err == nil {
		return int64(v)
	}
	return 0
}

// parsePostTime reads the posting time in the layouts TikTok uses; zero when it is none of them.
func parsePostTime(v string) time.Time {
	v = strings.TrimSpace(v)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, vnContentZone); err == nil {
			return t
		}
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 1_000_000_000 {
		return time.Unix(n, 0)
	}
	return time.Time{}
}

func parseShopVideos(data json.RawMessage) (out []AnalyticsVideo, next string, err error) {
	var d struct {
		Next   string `json:"next_page_token"`
		Videos []struct {
			ID       string   `json:"id"`
			Title    string   `json:"title"`
			Username string   `json:"username"`
			PostTime string   `json:"video_post_time"`
			Views    tsNumber `json:"views"`
			Items    tsNumber `json:"items_sold"`
			Products []struct {
				ID string `json:"id"`
			} `json:"products"`
		} `json:"videos"`
	}
	if err = json.Unmarshal(data, &d); err != nil {
		return nil, "", err
	}
	for _, v := range d.Videos {
		av := AnalyticsVideo{ID: v.ID, Title: v.Title, Username: strings.ToLower(v.Username), Views: v.Views.int64(), ItemsSold: int(v.Items.int64())}
		av.PostedAt = parsePostTime(v.PostTime)
		if av.PostedAt.IsZero() {
			av.PostedAt = VideoPostedAt(v.ID) // the video id carries its posting time
		}
		for _, p := range v.Products {
			av.Products = append(av.Products, p.ID)
		}
		out = append(out, av)
	}
	return out, d.Next, nil
}

// noAnalytics maps TikTok's "no scope" refusal to ErrAnalyticsScope.
func noAnalytics(err error) error {
	var rej *TTSRejected
	if errors.As(err, &rej) && rej.Code == ttsCodeNoScope {
		return ErrAnalyticsScope
	}
	return err
}

// ShopVideos lists the shop's videos (views included) posted between from and to, newest views first. It returns
// ErrAnalyticsScope when the app lacks the Analytics scope; that answer is remembered for a few minutes so the
// refusal is not asked again on every request.
func (s *TTShopService) ShopVideos(ctx context.Context, from, to time.Time) ([]AnalyticsVideo, error) {
	s.contentMu.Lock()
	denied := time.Since(s.analyticsDeniedAt) < analyticsDenyTTL && !s.analyticsDeniedAt.IsZero()
	s.contentMu.Unlock()
	if denied {
		return nil, ErrAnalyticsScope
	}
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, err
	}
	var all []AnalyticsVideo
	index := map[string]int{}
	for _, w := range dayWindows(from, to, analyticsMaxDays) {
		token := ""
		for page := 0; page < videoListMaxPages; page++ {
			q := url.Values{
				"shop_cipher": {cipher}, "page_size": {"100"}, "sort_field": {"views"}, "sort_order": {"DESC"},
				"start_date_ge": {w[0]}, "end_date_lt": {w[1]},
			}
			if token != "" {
				q.Set("page_token", token)
			}
			raw, err := s.get(ctx, ttsVideoListPath, q)
			if err != nil {
				err = noAnalytics(err)
				if errors.Is(err, ErrAnalyticsScope) {
					s.contentMu.Lock()
					s.analyticsDeniedAt = time.Now()
					s.contentMu.Unlock()
				}
				return nil, err
			}
			vids, next, err := parseShopVideos(raw)
			if err != nil {
				return nil, errors.New("TikTok trả về danh sách video không đọc được")
			}
			for _, v := range vids {
				if i, ok := index[v.ID]; ok { // the same video in another window: add its numbers
					all[i].Views += v.Views
					all[i].ItemsSold += v.ItemsSold
					continue
				}
				index[v.ID] = len(all)
				all = append(all, v)
			}
			if next == "" {
				break
			}
			token = next
		}
	}
	return all, nil
}

// VideoTraffic is what viewers did with a video.
type VideoTraffic struct {
	Views, Likes, Comments, Shares int64
}

func parseVideoTraffic(data json.RawMessage) (*VideoTraffic, error) {
	var d struct {
		Performance struct {
			Intervals []struct {
				Traffic struct {
					Views    tsNumber `json:"views"`
					Likes    tsNumber `json:"likes"`
					Comments tsNumber `json:"comments"`
					Shares   tsNumber `json:"shares"`
				} `json:"traffic"`
			} `json:"intervals"`
		} `json:"performance"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	t := &VideoTraffic{}
	for _, in := range d.Performance.Intervals {
		t.Views += in.Traffic.Views.int64()
		t.Likes += in.Traffic.Likes.int64()
		t.Comments += in.Traffic.Comments.int64()
		t.Shares += in.Traffic.Shares.int64()
	}
	return t, nil
}

type trafficEntry struct {
	at time.Time
	t  *VideoTraffic
}

// VideoTraffic reads views, likes, comments and shares of one video between from and to (cached for a few minutes).
func (s *TTShopService) VideoTraffic(ctx context.Context, id string, from, to time.Time) (*VideoTraffic, error) {
	if strings.Trim(id, "0123456789") != "" || id == "" {
		return nil, errors.New("mã video không hợp lệ")
	}
	key := id + "|" + from.In(vnContentZone).Format("2006-01-02")
	s.contentMu.Lock()
	if e, ok := s.trafficCache[key]; ok && time.Since(e.at) < videoTrafficTTL {
		s.contentMu.Unlock()
		return e.t, nil
	}
	s.contentMu.Unlock()
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, err
	}
	t := &VideoTraffic{}
	for _, w := range dayWindows(from, to, analyticsMaxDays) {
		q := url.Values{"shop_cipher": {cipher}, "granularity": {"ALL"}, "start_date_ge": {w[0]}, "end_date_lt": {w[1]}}
		raw, err := s.get(ctx, fmt.Sprintf(ttsVideoDetailPath, id), q)
		if err != nil {
			return nil, noAnalytics(err)
		}
		part, err := parseVideoTraffic(raw)
		if err != nil {
			return nil, errors.New("TikTok trả về chỉ số video không đọc được")
		}
		t.Views += part.Views
		t.Likes += part.Likes
		t.Comments += part.Comments
		t.Shares += part.Shares
	}
	s.contentMu.Lock()
	if s.trafficCache == nil || len(s.trafficCache) > 500 {
		s.trafficCache = map[string]trafficEntry{}
	}
	s.trafficCache[key] = trafficEntry{at: time.Now(), t: t}
	s.contentMu.Unlock()
	return t, nil
}

// contentState is the cache of the content/sales reads, embedded in TTShopService.
type contentState struct {
	contentMu         sync.Mutex
	affCache          []affCacheEntry
	trafficCache      map[string]trafficEntry
	analyticsDeniedAt time.Time
}
