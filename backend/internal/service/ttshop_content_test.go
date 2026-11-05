package service

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"
)

// A page shaped exactly like the live answer of orders/search (settled order, estimated order, a shop-ads order,
// a fully returned order and an ineligible one).
const affOrdersSample = `{"next_page_token":"abc","total_count":5,"orders":[
 {"create_time":1793830351,"delivery_time":0,"id":"o1","skus":[{"actual_commission_base":{"amount":"120000","currency":"VND"},"actual_paid_commission":{"amount":"12000","currency":"VND"},"actual_paid_shop_ads_commission":{},"commission_rate":"1000","content_id":"7689311292443675925","content_type":"VIDEO","creator_username":"ChangDiTap1802","estimated_commission_base":{"amount":"130614","currency":"VND"},"estimated_paid_commission":{"amount":"13061","currency":"VND"},"estimated_paid_shop_ads_commission":{},"fully_return":"No","product_id":"p1","quantity":1,"settlement_status":"SETTLED","open_collaboration_id":"oc1","target_collaboration_id":""}]},
 {"create_time":1793823088,"id":"o2","skus":[{"actual_commission_base":{},"actual_paid_commission":{},"actual_paid_shop_ads_commission":{},"content_id":"7689311292443675925","content_type":"VIDEO","creator_username":"changditap1802","estimated_commission_base":{"amount":"121652","currency":"VND"},"estimated_paid_commission":{"amount":"12165","currency":"VND"},"estimated_paid_shop_ads_commission":{},"fully_return":"No","product_id":"p1","quantity":1,"settlement_status":"To-SETTLE"}]},
 {"create_time":1793812154,"id":"o3","skus":[{"actual_commission_base":{},"content_id":"7689311292443675925","content_type":"VIDEO","creator_username":"changditap1802","estimated_commission_base":{"amount":"130614","currency":"VND"},"estimated_paid_commission":{},"estimated_paid_shop_ads_commission":{"amount":"3918","currency":"VND"},"fully_return":"No","product_id":"p1","quantity":1,"settlement_status":"AWAITING PAYMENT","shop_ads_commission_rate":"300"}]},
 {"create_time":1793812000,"id":"o4","skus":[{"actual_commission_base":{},"content_id":"7689311292443675925","content_type":"VIDEO","creator_username":"changditap1802","estimated_commission_base":{"amount":"99000","currency":"VND"},"estimated_paid_commission":{"amount":"9900","currency":"VND"},"fully_return":"Yes","product_id":"p1","quantity":1,"settlement_status":"To-SETTLE"}]},
 {"create_time":1793811000,"id":"o5","skus":[{"actual_commission_base":{},"content_id":"","content_type":"LIVE","creator_username":"other.kol","estimated_commission_base":{"amount":"50000","currency":"VND"},"estimated_paid_commission":{"amount":"5000","currency":"VND"},"fully_return":"No","product_id":"p2","quantity":2,"settlement_status":"INELIGIBLE"}]}
]}`

func sampleLines(t *testing.T) []AffiliateLine {
	t.Helper()
	lines, next, total, err := parseAffiliateOrders(json.RawMessage(affOrdersSample))
	if err != nil {
		t.Fatal(err)
	}
	if next != "abc" || total != 5 || len(lines) != 5 {
		t.Fatalf("next=%q total=%d lines=%d", next, total, len(lines))
	}
	return lines
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.0001 }

func TestParseAffiliateOrdersUsesActualWhenSettledAndEstimatedOtherwise(t *testing.T) {
	l := sampleLines(t)
	if l[0].Creator != "changditap1802" || !l[0].Confirmed() || !near(l[0].Net, 120000) || !near(l[0].Commission, 12000) {
		t.Fatalf("settled line must use the actual figures: %+v", l[0])
	}
	if l[1].Confirmed() || !near(l[1].Net, 121652) || !near(l[1].Commission, 12165) {
		t.Fatalf("unsettled line must use the estimate: %+v", l[1])
	}
	if !near(l[2].Commission, 3918) {
		t.Fatalf("a Shop Ads order carries its Shop Ads commission: %+v", l[2])
	}
}

func TestAggregateOrdersCountsOnlyRealSalesOncePerOrder(t *testing.T) {
	tot, videos := AggregateOrders(sampleLines(t), OrderFilter{Creator: "@ChangDiTap1802"})
	if tot.Orders != 3 || tot.Items != 3 || tot.Excluded != 1 {
		t.Fatalf("3 valid orders, 1 excluded (fully returned): %+v", tot)
	}
	if !near(tot.GMV, 120000+121652+130614) {
		t.Fatalf("gmv %v", tot.GMV)
	}
	if !near(tot.Commission, 12000+12165+3918) || !near(tot.CommissionConfirmed, 12000) {
		t.Fatalf("commission %v confirmed %v", tot.Commission, tot.CommissionConfirmed)
	}
	if len(videos) != 1 || videos[0].Orders != 3 || tot.Videos != 1 {
		t.Fatalf("one video with 3 orders: %+v", videos)
	}
	if videos[0].ProductIDs[0] != "p1" {
		t.Fatalf("products: %v", videos[0].ProductIDs)
	}
}

func TestAggregateOrdersFiltersByProductAndStart(t *testing.T) {
	lines := sampleLines(t)
	if tot, _ := AggregateOrders(lines, OrderFilter{Creator: "changditap1802", ProductIDs: map[string]bool{"zzz": true}}); tot.Orders != 0 {
		t.Fatalf("another product must not match: %+v", tot)
	}
	from := time.Unix(1793820000, 0) // after o3, o4
	tot, _ := AggregateOrders(lines, OrderFilter{Creator: "changditap1802", From: from})
	if tot.Orders != 2 {
		t.Fatalf("only orders from the start on: %+v", tot)
	}
}

func TestIneligibleAndNonVideoSalesAreNotCountedAsVideoSales(t *testing.T) {
	tot, videos := AggregateOrders(sampleLines(t), OrderFilter{Creator: "other.kol"})
	if tot.Orders != 0 || tot.Excluded != 1 || len(videos) != 0 {
		t.Fatalf("an ineligible live order is excluded: %+v %v", tot, videos)
	}
}

func TestRefundedQuantityLowersTheSalesValueWhenNotSettled(t *testing.T) {
	k := rawAffSKU{Quantity: 4, RefundedQuantity: 1, ReturnedQuantity: 1, EstimatedCommissionBase: tsMoney{Amount: "400000"}}
	l := lineOf("o", 1793830351, k)
	if !near(l.Net, 200000) || !near(l.Gross, 400000) {
		t.Fatalf("half was refunded or returned: %+v", l)
	}
}

func TestVideoPostedAtDecodesTheVideoID(t *testing.T) {
	got := VideoPostedAt("7689311292443675925")
	if got.IsZero() || got.Year() != 2026 {
		t.Fatalf("expected a 2026 time, got %v", got)
	}
	if !VideoPostedAt("123").IsZero() || !VideoPostedAt("abc").IsZero() {
		t.Fatal("ids that are not video ids must give no time")
	}
	if VideoURL("@abc", "77") != "https://www.tiktok.com/@abc/video/77" {
		t.Fatal(VideoURL("@abc", "77"))
	}
}

func TestParseShopVideosAndTraffic(t *testing.T) {
	vids, next, err := parseShopVideos(json.RawMessage(`{"next_page_token":"n","videos":[{"id":"1","title":"Khăn lạnh","username":"Abc","video_post_time":"2026-11-03T05:00:00Z","views":"291","items_sold":2,"products":[{"id":"p1","name":"x"}]}]}`))
	if err != nil || next != "n" || len(vids) != 1 || vids[0].Views != 291 || vids[0].Username != "abc" || vids[0].PostedAt.IsZero() || vids[0].Products[0] != "p1" || vids[0].ItemsSold != 2 {
		t.Fatalf("%+v %v", vids, err)
	}
	tr, err := parseVideoTraffic(json.RawMessage(`{"performance":{"intervals":[{"traffic":{"views":291,"likes":"0","comments":0,"shares":1}},{"traffic":{"views":9,"likes":1}}]}}`))
	if err != nil || tr.Views != 300 || tr.Likes != 1 || tr.Shares != 1 || tr.Comments != 0 {
		t.Fatalf("%+v %v", tr, err)
	}
}

func TestNoScopeRefusalBecomesAnalyticsScopeError(t *testing.T) {
	if !errors.Is(noAnalytics(&TTSRejected{Code: 105005, Message: "Access denied"}), ErrAnalyticsScope) {
		t.Fatal("105005 means the Analytics scope is missing")
	}
	other := &TTSRejected{Code: 36009002, Message: "rate"}
	if errors.Is(noAnalytics(other), ErrAnalyticsScope) {
		t.Fatal("other refusals must stay as they are")
	}
}

func TestParseVideoPageReadsTheCountersTitleAndCover(t *testing.T) {
	page := []byte(`<html><script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"itemInfo":{"itemStruct":{"id":"77","desc":" Mềm mại êm ái ","createTime":"1793590000","video":{"cover":"https://cdn/x.jpg"},"author":{"uniqueId":"EmAnh"},"stats":{"playCount":354,"diggCount":"5","commentCount":0,"shareCount":2}}}}}}</script></html>`)
	d, err := parseVideoPage(page, "77")
	if err != nil || d.Views != 354 || d.Likes != 5 || d.Comments != 0 || d.Shares != 2 || d.Title != "Mềm mại êm ái" || d.CoverURL != "https://cdn/x.jpg" || d.Username != "emanh" || d.CreatedAt.IsZero() {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := parseVideoPage(page, "99"); err == nil {
		t.Fatal("a page for another video must be refused, never shown under this one")
	}
	if _, err := parseVideoPage([]byte("<html>captcha</html>"), "77"); err == nil {
		t.Fatal("a page without data is an error")
	}
}

func TestDayWindowsNeverExceedTheAnalyticsLimit(t *testing.T) {
	loc := time.FixedZone("ICT", 7*3600)
	from := time.Date(2026, 8, 8, 10, 0, 0, 0, loc)
	to := time.Date(2026, 11, 5, 9, 0, 0, 0, loc)
	ws := dayWindows(from, to, 58)
	if len(ws) != 2 || ws[0][0] != "2026-08-08" || ws[len(ws)-1][1] != "2026-11-06" {
		t.Fatalf("90 days need 2 windows ending the day after the last day: %v", ws)
	}
	for i, w := range ws {
		a, _ := time.Parse("2006-01-02", w[0])
		b, _ := time.Parse("2006-01-02", w[1])
		if days := int(b.Sub(a).Hours() / 24); days > 58 || days < 1 {
			t.Fatalf("window %d spans %d days", i, days)
		}
		if i > 0 && ws[i-1][1] != w[0] {
			t.Fatalf("windows must be contiguous: %v", ws)
		}
	}
	if one := dayWindows(to, to, 58); len(one) != 1 || one[0][0] != "2026-11-05" || one[0][1] != "2026-11-06" {
		t.Fatalf("a single day: %v", one)
	}
}

func TestPostTimeFallsBackToTheVideoID(t *testing.T) {
	vids, _, err := parseShopVideos(json.RawMessage(`{"videos":[{"id":"7689311292443675925","username":"a","video_post_time":"not a date","views":1}]}`))
	if err != nil || len(vids) != 1 || vids[0].PostedAt.IsZero() || vids[0].PostedAt.Year() != 2026 {
		t.Fatalf("%+v %v", vids, err)
	}
	for _, in := range []string{"2026-11-03T05:00:00Z", "2026-11-03 12:00:00", "2026-11-03", "1793590000"} {
		if parsePostTime(in).IsZero() {
			t.Fatalf("%q must parse", in)
		}
	}
}

func TestClampToLookbackKeepsTheRangeInsideWhatTikTokReads(t *testing.T) {
	loc := time.FixedZone("ICT", 7*3600)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, loc)
	oldest := time.Date(2026, 5, 9, 0, 0, 0, 0, loc) // 180 days before 2026-11-05

	if got, ok := ClampToLookback(now.AddDate(0, 0, -365), now, now); !ok || !got.Equal(oldest) {
		t.Fatalf("a 365-day range is cut to 180 days: %v %v", got, ok)
	}
	recent := now.AddDate(0, 0, -30)
	if got, ok := ClampToLookback(recent, now, now); !ok || !got.Equal(recent) {
		t.Fatalf("a recent range is untouched: %v %v", got, ok)
	}
	if _, ok := ClampToLookback(now.AddDate(0, 0, -400), now.AddDate(0, 0, -300), now); ok {
		t.Fatal("a period entirely before the limit cannot be read")
	}
}
