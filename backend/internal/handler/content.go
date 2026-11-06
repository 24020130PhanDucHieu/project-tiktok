package handler

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// ContentTTS is what the content routes need from TikTok Shop (*service.TTShopService). Everything here only reads.
type ContentTTS interface {
	AffiliateLines(ctx context.Context, from, to time.Time) ([]service.AffiliateLine, bool, error)
	ShopVideos(ctx context.Context, from, to time.Time) ([]service.AnalyticsVideo, error)
	VideoTraffic(ctx context.Context, id string, from, to time.Time) (*service.VideoTraffic, error)
	GetTarget(ctx context.Context, id string) (*service.TargetDetail, error)
}

// ContentMeta gives the public page data of a video: title, cover and the public counters (*service.TikTokService).
type ContentMeta interface {
	FetchVideoDetail(ctx context.Context, videoURL string) (*service.TikTokVideoDetail, error)
}

// ContentKocs is the KOC store used by the content routes.
type ContentKocs interface {
	FindActiveByID(ctx context.Context, id string) (*domain.Koc, error)
	FindNotArchived(ctx context.Context) ([]*domain.Koc, error)
	Save(ctx context.Context, koc *domain.Koc) error
}

// ContentCollabs is the collaboration store used by the content routes.
type ContentCollabs interface {
	FindByID(ctx context.Context, id string) (*domain.Collab, error)
	Save(ctx context.Context, c *domain.Collab) error
}

// ContentHandler serves the real posts (videos) and sales of KOCs, read from TikTok Shop.
type ContentHandler struct {
	tts     ContentTTS
	meta    ContentMeta
	kocs    ContentKocs
	collabs ContentCollabs

	metaMu    sync.Mutex
	metaCache map[string]metaEntry
}

type metaEntry struct {
	at time.Time
	d  *service.TikTokVideoDetail
}

const (
	contentDefaultDays = 90
	contentMaxDays     = 365
	contentPageDefault = 50
	contentPageMax     = 100
	contentEnrichLimit = 24
	contentMetaTTL     = 6 * time.Hour
	contentMetaFailTTL = 15 * time.Minute // TikTok's bot check blocks server IPs now and then: do not hammer it
	contentEnrichTime  = 20 * time.Second
)

// NewContentHandler builds a ContentHandler.
func NewContentHandler(tts ContentTTS, meta ContentMeta, kocs ContentKocs, collabs ContentCollabs) *ContentHandler {
	return &ContentHandler{tts: tts, meta: meta, kocs: kocs, collabs: collabs, metaCache: map[string]metaEntry{}}
}

// ---------- response shapes ----------

type videoOut struct {
	ID                  string     `json:"id"`
	Username            string     `json:"username"`
	Title               string     `json:"title"`
	URL                 string     `json:"url"`
	CoverURL            string     `json:"coverUrl"`
	PostedAt            *time.Time `json:"postedAt"`
	ProductIDs          []string   `json:"productIds"`
	Views               *int64     `json:"views"` // null: TikTok has not told us (Analytics scope missing)
	Likes               *int64     `json:"likes"`
	Comments            *int64     `json:"comments"`
	Shares              *int64     `json:"shares"`
	StatsSource         string     `json:"statsSource"` // "analytics" (TikTok Shop API) or "public" (counters shown on TikTok); "" = unknown
	Orders              int        `json:"orders"`
	Items               int        `json:"items"`
	GMV                 float64    `json:"gmv"`
	Commission          float64    `json:"commission"`
	CommissionConfirmed float64    `json:"commissionConfirmed"`
	KocID               string     `json:"kocId,omitempty"`
	KocName             string     `json:"kocName,omitempty"`
	KocAvatar           string     `json:"kocAvatar,omitempty"`
	InApp               bool       `json:"inApp"`
}

type analyticsOut struct {
	Available bool   `json:"available"`
	Note      string `json:"note,omitempty"`
}

type rangeOut struct {
	From string `json:"from"`
	To   string `json:"to"`
	Days int    `json:"days"`
}

type contentOut struct {
	Videos []videoOut          `json:"videos"`
	Total  int                 `json:"total"`
	Totals service.OrderTotals `json:"totals"`
	Views  *int64              `json:"views"`
	// ViewsPublic: Views is the sum of the public counters, not of the Analytics API
	ViewsPublic bool                 `json:"viewsPublic"`
	Analytics   analyticsOut         `json:"analytics"`
	Range       rangeOut             `json:"range"`
	Truncated   bool                 `json:"truncated"`
	AsOf        time.Time            `json:"asOf"`
	Koc         *kocBrief            `json:"koc,omitempty"`
	Note        string               `json:"note,omitempty"`
	byUser      map[string]*kocBrief `json:"-"`
}

type kocBrief struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

const analyticsMissingNote = "Ứng dụng chưa được cấp quyền Analytics của TikTok Shop nên chưa lấy được lượt xem, tim, bình luận bằng API (video chưa có đơn cũng chưa liệt kê được). " +
	"Hệ thống cố đọc số công khai trên trang TikTok của video, nhưng TikTok hay chặn truy cập từ máy chủ nên có thể hiện \"—\". " +
	"Bật quyền Analytics ở Partner Center rồi ủy quyền lại shop để lấy đủ. Số đơn, doanh thu và hoa hồng là số thật từ đơn hàng."

func usernameOf(k *domain.Koc) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(k.TiktokID), "@"))
}

func briefOf(k *domain.Koc) *kocBrief {
	return &kocBrief{ID: k.ID.Hex(), Name: firstText(k.DisplayName, k.TiktokID), Username: usernameOf(k), Avatar: k.AvatarURL}
}

func firstText(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// rangeOf is the period read from TikTok: the last `days` days up to now, from the start of a Vietnam day.
func rangeOf(days int, now time.Time) (time.Time, time.Time) {
	if days < 1 || days > contentMaxDays {
		days = contentDefaultDays
	}
	vn := now.In(time.FixedZone("ICT", 7*3600))
	start := time.Date(vn.Year(), vn.Month(), vn.Day(), 0, 0, 0, 0, vn.Location()).AddDate(0, 0, -(days - 1))
	return start, now
}

func contentFail(c *fiber.Ctx, err error) error {
	var rej *service.TTSRejected
	switch {
	case errors.Is(err, service.ErrTTShopNotConnected):
		return httpx.Fail(c, fiber.StatusConflict, err.Error())
	case errors.As(err, &rej) && rej.Code == 105005:
		return httpx.Fail(c, fiber.StatusBadGateway, "Ứng dụng chưa được cấp quyền đọc đơn affiliate của TikTok Shop. Bật quyền ở Partner Center rồi ủy quyền lại shop.")
	case service.IsRateLimited(err):
		return httpx.Fail(c, fiber.StatusTooManyRequests, "TikTok đang giới hạn tốc độ. Hãy đợi khoảng một phút rồi thử lại.")
	default:
		return httpx.Fail(c, fiber.StatusBadGateway, err.Error())
	}
}

// ---------- building the list ----------

type contentQuery struct {
	from, to time.Time
	users    map[string]*kocBrief // username -> KOC; nil = every creator in the shop
	products map[string]bool      // empty = any product
	since    time.Time            // only sales from here on (zero = whole range)
}

// build reads orders (and, when allowed, video analytics) and returns the videos, totals and the views total.
func (h *ContentHandler) build(ctx context.Context, q contentQuery, limit int) (*contentOut, error) {
	lines, truncated, err := h.tts.AffiliateLines(ctx, q.from, q.to)
	if err != nil {
		return nil, err
	}
	picked := make([]service.AffiliateLine, 0, len(lines))
	for _, l := range lines {
		if q.users != nil && q.users[l.Creator] == nil {
			continue
		}
		picked = append(picked, l)
	}
	totals, sales := service.AggregateOrders(picked, service.OrderFilter{ProductIDs: q.products, From: q.since})

	out := &contentOut{Truncated: truncated, AsOf: time.Now(), Totals: totals,
		Range: rangeOut{From: q.from.In(time.FixedZone("ICT", 7*3600)).Format("2006-01-02"), To: q.to.In(time.FixedZone("ICT", 7*3600)).Format("2006-01-02")}}
	out.Range.Days = int(q.to.Sub(q.from).Hours()/24) + 1

	byID := map[string]*videoOut{}
	order := []string{}
	for _, v := range sales {
		vo := &videoOut{ID: v.ContentID, Username: v.Creator, URL: service.VideoURL(v.Creator, v.ContentID), ProductIDs: v.ProductIDs,
			Orders: v.Orders, Items: v.Items, GMV: v.GMV, Commission: v.Commission, CommissionConfirmed: v.Confirmed}
		if t := service.VideoPostedAt(v.ContentID); !t.IsZero() {
			vo.PostedAt = &t
		}
		byID[v.ContentID] = vo
		order = append(order, v.ContentID)
	}

	// views, likes... only exist behind the Analytics scope
	vids, aerr := h.tts.ShopVideos(ctx, q.from, q.to)
	switch {
	case aerr == nil:
		out.Analytics.Available = true
		if _, whole := service.ClampToLookback(q.from, q.to, time.Now()); whole {
			if cl, _ := service.ClampToLookback(q.from, q.to, time.Now()); cl.After(q.from) {
				out.Analytics.Note = "TikTok chỉ cho đọc lượt xem, tim, bình luận trong " + strconv.Itoa(service.AnalyticsLookbackDays) + " ngày gần nhất, nên video cũ hơn hiện \"—\". Số đơn, doanh thu và hoa hồng vẫn đủ."
			}
		}
		var views int64
		for _, av := range vids {
			if q.users != nil && q.users[av.Username] == nil {
				continue
			}
			if len(q.products) > 0 && !anyIn(av.Products, q.products) {
				continue
			}
			if !q.since.IsZero() && !av.PostedAt.IsZero() && av.PostedAt.Before(q.since) && byID[av.ID] == nil {
				continue
			}
			vo := byID[av.ID]
			if vo == nil {
				vo = &videoOut{ID: av.ID, Username: av.Username, URL: service.VideoURL(av.Username, av.ID), ProductIDs: av.Products}
				byID[av.ID] = vo
				order = append(order, av.ID)
			}
			if !av.PostedAt.IsZero() {
				t := av.PostedAt
				vo.PostedAt = &t
			}
			if vo.Title == "" {
				vo.Title = av.Title
			}
			v := av.Views
			vo.Views = &v
			views += av.Views
		}
		out.Views = &views
	case errors.Is(aerr, service.ErrAnalyticsScope):
		out.Analytics.Note = analyticsMissingNote
	case errors.Is(aerr, service.ErrAnalyticsRange):
		out.Analytics.Note = "Khoảng thời gian này cũ hơn " + strconv.Itoa(service.AnalyticsLookbackDays) + " ngày nên TikTok không cho đọc lượt xem, tim, bình luận. Chọn khoảng gần hơn để xem. Số đơn, doanh thu và hoa hồng vẫn đủ."
	case service.IsRateLimited(aerr):
		out.Analytics.Note = "TikTok đang giới hạn tốc độ gọi nên lượt xem chưa tải được. Đợi khoảng một phút rồi bấm làm mới. Số đơn và doanh thu vẫn là số thật."
	default:
		out.Analytics.Note = "Chưa đọc được lượt xem lúc này (" + aerr.Error() + "). Số đơn và doanh thu vẫn là số thật."
	}

	list := make([]*videoOut, 0, len(order))
	for _, id := range order {
		vo := byID[id]
		if q.users != nil {
			if b := q.users[vo.Username]; b != nil {
				vo.KocID, vo.KocName, vo.KocAvatar, vo.InApp = b.ID, b.Name, b.Avatar, true
			}
		}
		list = append(list, vo)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i].PostedAt, list[j].PostedAt
		switch {
		case a != nil && b != nil && !a.Equal(*b):
			return a.After(*b)
		case (a != nil) != (b != nil):
			return a != nil
		}
		return list[i].GMV > list[j].GMV
	})
	out.Total = len(list)
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	h.enrich(ctx, list, out.Analytics.Available, q.from, q.to)
	if out.Views == nil && len(list) > 0 && len(list) == out.Total { // every video is shown: add up the public counters
		var sum int64
		complete := true
		for _, v := range list {
			if v.Views == nil {
				complete = false
				break
			}
			sum += *v.Views
		}
		if complete {
			out.Views, out.ViewsPublic = &sum, true
		}
	}
	out.Videos = make([]videoOut, len(list))
	for i, v := range list {
		out.Videos[i] = *v
	}
	return out, nil
}

func anyIn(ids []string, set map[string]bool) bool {
	for _, id := range ids {
		if set[id] {
			return true
		}
	}
	return false
}

// enrich adds the public title/cover and, when allowed, likes/comments/shares to the first videos of the list.
// It works for a bounded time; whatever is not ready stays blank instead of holding the page up.
func (h *ContentHandler) enrich(ctx context.Context, list []*videoOut, analytics bool, from, to time.Time) {
	n := len(list)
	if n > contentEnrichLimit {
		n = contentEnrichLimit
	}
	ctx, cancel := context.WithTimeout(ctx, contentEnrichTime)
	defer cancel()
	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup
	var limited atomic.Bool // TikTok asked us to slow down: stop asking for the remaining videos
	for _, v := range list[:n] {
		wg.Add(1)
		sem <- struct{}{}
		go func(v *videoOut) {
			defer wg.Done()
			defer func() { <-sem }()
			d := h.videoMeta(ctx, v)
			if d != nil {
				if v.Title == "" {
					v.Title = d.Title
				}
				v.CoverURL = d.CoverURL
				if v.PostedAt == nil && !d.CreatedAt.IsZero() {
					t := d.CreatedAt
					v.PostedAt = &t
				}
			}
			if analytics && !limited.Load() {
				t, err := h.tts.VideoTraffic(ctx, v.ID, from, to)
				if service.IsRateLimited(err) {
					limited.Store(true)
				}
				if err == nil && t != nil {
					v.Likes, v.Comments, v.Shares = &t.Likes, &t.Comments, &t.Shares
					if v.Views == nil {
						v.Views = &t.Views
					}
					v.StatsSource = "analytics"
					return
				}
			}
			if d != nil && v.Views == nil { // no Analytics: the counters anybody sees on the video's page
				views, likes, comments, shares := d.Views, d.Likes, d.Comments, d.Shares
				v.Views, v.Likes, v.Comments, v.Shares, v.StatsSource = &views, &likes, &comments, &shares, "public"
			}
		}(v)
	}
	wg.Wait()
}

// videoMeta reads the public page of the video (cached for a few hours). nil when TikTok gave nothing usable or the
// page belongs to another creator's video, so nothing is ever shown under the wrong video or KOC.
func (h *ContentHandler) videoMeta(ctx context.Context, v *videoOut) *service.TikTokVideoDetail {
	if h.meta == nil {
		return nil
	}
	h.metaMu.Lock()
	if e, ok := h.metaCache[v.ID]; ok {
		ttl := contentMetaTTL
		if e.d == nil {
			ttl = contentMetaFailTTL
		}
		if time.Since(e.at) < ttl {
			h.metaMu.Unlock()
			return e.d
		}
	}
	h.metaMu.Unlock()
	d, err := h.meta.FetchVideoDetail(ctx, v.URL)
	if err != nil || d == nil || (d.Username != "" && d.Username != v.Username) {
		h.metaMu.Lock()
		h.metaCache[v.ID] = metaEntry{at: time.Now()} // remember the failure for a while
		h.metaMu.Unlock()
		return nil
	}
	h.metaMu.Lock()
	if len(h.metaCache) > 2000 {
		h.metaCache = map[string]metaEntry{}
	}
	h.metaCache[v.ID] = metaEntry{at: time.Now(), d: d}
	h.metaMu.Unlock()
	return d
}

func queryDays(c *fiber.Ctx) int {
	d, _ := strconv.Atoi(c.Query("days", strconv.Itoa(contentDefaultDays)))
	if d < 1 || d > contentMaxDays {
		return contentDefaultDays
	}
	return d
}

func queryLimit(c *fiber.Ctx) int {
	n, _ := strconv.Atoi(c.Query("limit", strconv.Itoa(contentPageDefault)))
	if n < 1 || n > contentPageMax {
		return contentPageDefault
	}
	return n
}

// ---------- routes ----------

// KocVideos handles GET /kocs/:id/videos?days=&productId= : the real videos and sales of ONE KOC in the shop. It does
// not depend on a collaboration having been created in this app, so a KOC who worked with the shop before shows up too.
func (h *ContentHandler) KocVideos(c *fiber.Ctx) error {
	ctx := c.UserContext()
	koc, err := h.kocs.FindActiveByID(ctx, c.Params("id"))
	if err != nil {
		return err
	}
	if koc == nil {
		return httpx.NewError(fiber.StatusNotFound, "Không tìm thấy KOC.")
	}
	user := usernameOf(koc)
	if user == "" {
		return httpx.Fail(c, fiber.StatusUnprocessableEntity, "KOC này chưa có tên TikTok nên chưa tìm được bài đăng.")
	}
	from, to := rangeOf(queryDays(c), time.Now())
	q := contentQuery{from: from, to: to, users: map[string]*kocBrief{user: briefOf(koc)}}
	if pid := strings.TrimSpace(c.Query("productId")); pid != "" {
		if strings.Trim(pid, "0123456789") != "" {
			return httpx.Fail(c, fiber.StatusBadRequest, "Mã sản phẩm không hợp lệ.")
		}
		q.products = map[string]bool{pid: true}
	}
	out, err := h.build(ctx, q, queryLimit(c))
	if err != nil {
		return contentFail(c, err)
	}
	out.Koc = briefOf(koc)
	return httpx.OK(c, out)
}

// Videos handles GET /content/videos?days=&kocId=&all=1&q= : the videos of the KOCs saved in this app (or, with
// all=1, of every creator who sold for the shop), with their real sales.
func (h *ContentHandler) Videos(c *fiber.Ctx) error {
	ctx := c.UserContext()
	from, to := rangeOf(queryDays(c), time.Now())
	q := contentQuery{from: from, to: to}
	kocs, err := h.kocs.FindNotArchived(ctx)
	if err != nil {
		return err
	}
	users := map[string]*kocBrief{}
	for _, k := range kocs {
		if u := usernameOf(k); u != "" {
			users[u] = briefOf(k)
		}
	}
	if id := strings.TrimSpace(c.Query("kocId")); id != "" {
		var only *kocBrief
		for _, b := range users {
			if b.ID == id {
				only = b
			}
		}
		if only == nil {
			return httpx.NewError(fiber.StatusNotFound, "Không tìm thấy KOC.")
		}
		q.users = map[string]*kocBrief{only.Username: only}
	} else if c.Query("all") != "1" {
		q.users = users
	} else {
		q.users = nil
	}
	out, err := h.build(ctx, q, 0) // filter by text first, then cut the page
	if err != nil {
		return contentFail(c, err)
	}
	if q.users == nil {
		for i := range out.Videos {
			if b := users[out.Videos[i].Username]; b != nil {
				out.Videos[i].KocID, out.Videos[i].KocName, out.Videos[i].KocAvatar, out.Videos[i].InApp = b.ID, b.Name, b.Avatar, true
			}
		}
	}
	if kw := strings.ToLower(strings.TrimSpace(c.Query("q"))); kw != "" {
		kept := out.Videos[:0]
		for _, v := range out.Videos {
			if strings.Contains(strings.ToLower(v.Username+" "+v.KocName+" "+v.Title+" "+v.ID), kw) {
				kept = append(kept, v)
			}
		}
		out.Videos = kept
		out.Total = len(kept)
	}
	if limit := queryLimit(c); len(out.Videos) > limit {
		out.Videos = out.Videos[:limit]
	}
	return httpx.OK(c, out)
}

// collabProducts finds the TikTok product ids of a collaboration: the product of an open collaboration, or the
// products of the target collaboration it was invited through. Empty when the collaboration has no product id
// (a pasted link): then every product of the KOC is counted and the answer says so.
func (h *ContentHandler) collabProducts(ctx context.Context, col *domain.Collab) map[string]bool {
	ids := map[string]bool{}
	if p := strings.TrimSpace(col.ProductID); p != "" {
		ids[p] = true
	}
	if t := strings.TrimSpace(col.TargetCollabID); t != "" {
		if d, err := h.tts.GetTarget(ctx, t); err == nil && d != nil {
			for _, p := range d.Products {
				if p.ID != "" {
					ids[p.ID] = true
				}
			}
		}
	}
	return ids
}

// Performance handles GET /collabs/:cid/performance?refresh=1 : the REAL sales of this collaboration (orders of this
// KOC, for this collaboration's products, from the day it started), saved on the collaboration so the "Hiệu quả" step
// and the overview charts use the same numbers.
func (h *ContentHandler) Performance(c *fiber.Ctx) error {
	ctx := c.UserContext()
	col, err := h.collabs.FindByID(ctx, c.Params("cid"))
	if err != nil {
		return err
	}
	if col == nil {
		return httpx.NewError(fiber.StatusNotFound, "Không tìm thấy lần hợp tác.")
	}
	koc, err := h.kocs.FindActiveByID(ctx, col.KocID.Hex())
	if err != nil {
		return err
	}
	if koc == nil {
		return httpx.NewError(fiber.StatusNotFound, "Không tìm thấy KOC của lần hợp tác này.")
	}
	user := usernameOf(koc)
	if user == "" {
		return httpx.Fail(c, fiber.StatusUnprocessableEntity, "KOC này chưa có tên TikTok nên chưa lấy được số liệu thật.")
	}
	if col.Status == domain.CollabClosed && col.Real != nil && c.Query("refresh") != "1" {
		return httpx.OK(c, fiber.Map{"collab": col, "videos": []videoOut{}, "frozen": true})
	}

	start := col.StartedAt
	if start.IsZero() {
		start = col.CreatedAt
	}
	vn := time.FixedZone("ICT", 7*3600)
	s := start.In(vn)
	from := time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, vn)
	to := time.Now()
	if col.EndedAt != nil && col.EndedAt.Before(to) {
		to = *col.EndedAt
	}
	if !to.After(from) {
		to = from.Add(time.Hour)
	}
	q := contentQuery{from: from, to: to, since: start, users: map[string]*kocBrief{user: briefOf(koc)}, products: h.collabProducts(ctx, col)}
	byProduct := len(q.products) > 0
	out, err := h.build(ctx, q, queryLimit(c))
	if err != nil {
		return contentFail(c, err)
	}

	real := &domain.CollabReal{
		SyncedAt: time.Now(), Orders: out.Totals.Orders, ItemsSold: out.Totals.Items, GMV: out.Totals.GMV,
		Commission: out.Totals.Commission, CommissionConfirmed: out.Totals.CommissionConfirmed, Excluded: out.Totals.Excluded,
		Videos: out.Total, ByProduct: byProduct, Truncated: out.Truncated,
	}
	if out.Views != nil {
		real.Views, real.ViewsKnown, real.ViewsPublic = float64(*out.Views), true, out.ViewsPublic
	}
	col.Real = real
	if err := h.collabs.Save(ctx, col); err != nil {
		return err
	}
	if col.Status == domain.CollabActive {
		koc.CollabStep = col.Step
		_ = h.kocs.Save(ctx, koc) // keep the KOC's next-step tag in line; the snapshot is already saved
	}
	if !byProduct {
		out.Note = "Lần hợp tác này chưa gắn mã sản phẩm TikTok nên số liệu tính theo mọi đơn của KOC từ ngày bắt đầu."
	}
	return httpx.OK(c, fiber.Map{"collab": col, "videos": out.Videos, "totals": out.Totals, "analytics": out.Analytics,
		"truncated": out.Truncated, "note": out.Note, "range": out.Range})
}
