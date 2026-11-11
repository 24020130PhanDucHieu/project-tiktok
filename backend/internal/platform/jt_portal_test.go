package platform

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// jtPage wraps a result block the way the public tracking page does: the journey sits in "result-tracking", between
// other tabs whose text (prohibited goods, slogans) must never be read as a status.
func jtPage(result string) string {
	return `<html><body>
<div class="result-prohibited-text">Hàng cấm gửi... Giao hàng thành công là cam kết của chúng tôi</div>
<div class="result-tariff px-4" x-show="openTab=='tariff'"></div>
<div class="result-tracking px-4 lg:px-0" x-show="openTab=='track'">` + result + `</div>
<div class="result-office px-4" x-show="openTab=='office-point'"></div>
</body></html>`
}

// jtRow is one scan of the journey: a clock, a date and what happened.
func jtRow(clock, day, text string) string {
	return `<div class="flex"><div><ion-icon name="time-outline"></ion-icon><span>` + clock + `</span>
<ion-icon name="calendar-outline"></ion-icon><span>` + day + `</span></div>
<div class="flex-1"> ` + text + ` </div></div>`
}

const jtInTransit = "<div>Kết quả tra cứu</div>"

func TestTheJourneyOfAParcelIsReadFromThePortalNewestFirst(t *testing.T) {
	page := jtPage(jtInTransit +
		jtRow("08:12:58", "2026-10-27", `Bưu cục【<a href="#">TTKT NGHỆ AN</a>】đang chuyển hàng đến【(NAA) Tuần】`) +
		jtRow("07:23:46", "2026-10-27", `Hàng đã được chuyển đến【TTKT NGHỆ AN】`) +
		jtRow("16:52:53", "2026-10-26", `Nhân viên【AG-DP Mai Hoàng Dương】của bưu cục【(HNI)Yên Lãng】đã nhận hàng. SĐT nhân viên nhận hàng【+84967801593】`))

	events, found := parseJTPortal(page)
	if !found || len(events) != 3 {
		t.Fatalf("three scans expected, got found=%v %+v", found, events)
	}
	if events[0].Text != "Bưu cục【TTKT NGHỆ AN】đang chuyển hàng đến【(NAA) Tuần】" {
		t.Fatalf("the newest scan comes first, tags stripped: %q", events[0].Text)
	}
	want := time.Date(2026, 10, 27, 8, 12, 58, 0, time.FixedZone("ICT", 7*3600))
	if !events[0].Time.Equal(want) {
		t.Fatalf("the clock is Vietnam time: %v, want %v", events[0].Time, want)
	}
	// the real page pads names inside the brackets, also right after another bracket
	padded, _ := parseJTPortal(jtPage(jtRow("12:27:12", "2026-10-27", "Nhân viên【Phạm Thị Chung】 của bưu cục 【 (NAA) Tuần】đang giao hàng.【 +84900000000】")))
	if len(padded) != 1 || padded[0].Text != "Nhân viên【Phạm Thị Chung】của bưu cục【(NAA) Tuần】đang giao hàng.【+84900000000】" {
		t.Fatalf("no padding inside or around the brackets: %+v", padded)
	}
	if !strings.Contains(events[2].Text, "đã nhận hàng") || events[2].Time.Day() != 26 {
		t.Fatalf("the first scan is last: %+v", events[2])
	}
}

func TestScansAreSortedNewestFirstWhateverOrderThePageUses(t *testing.T) {
	page := jtPage(jtRow("16:52:53", "2026-10-26", "đã nhận hàng") + jtRow("08:12:58", "2026-10-27", "đang chuyển hàng"))
	events, _ := parseJTPortal(page)
	if len(events) != 2 || events[0].Text != "đang chuyển hàng" {
		t.Fatalf("newest first: %+v", events)
	}
}

func TestAnUnknownBillIsReportedAsNotFound(t *testing.T) {
	page := jtPage(`<div class="empty-vandon"><span>Không tìm thấy dữ liệu về vận đơn. Bạn vui lòng kiểm tra lại mã vận đơn !</span></div>`)
	if events, found := parseJTPortal(page); found || len(events) != 0 {
		t.Fatalf("no journey for an unknown bill: found=%v %+v", found, events)
	}
	c := jtServers(t, nil, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(page)) })
	c.cfg.JTAPIAccount = "" // no official account: the portal is the only source
	res, err := c.QueryTracking(context.Background(), "800000000000", "1234")
	if err != nil {
		t.Fatal(err)
	}
	if res.Delivered || len(res.Events) != 0 || !strings.Contains(res.StatusText, "không tìm thấy") {
		t.Fatalf("the booker must be told the bill was not found: %+v", res)
	}
}

func TestThePortalStatusIsTheNewestScanAndDeliveryComesFromItOnly(t *testing.T) {
	serve := func(page string) *JTClient {
		c := jtServers(t, nil, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(page)) })
		c.cfg.JTAPIAccount = ""
		return c
	}
	// in transit: the slogan "Giao hàng thành công" elsewhere on the page must not mark it delivered
	res, err := serve(jtPage(jtRow("08:12:58", "2026-10-27", "Bưu cục【TTKT NGHỆ AN】đang chuyển hàng đến【(NAA) Tuần】"))).
		QueryTracking(context.Background(), "841328496973", "1234")
	if err != nil {
		t.Fatal(err)
	}
	if res.Delivered || res.StatusText != "Bưu cục【TTKT NGHỆ AN】đang chuyển hàng đến【(NAA) Tuần】" || len(res.Events) != 1 || res.Source != "jtexpress.vn" {
		t.Fatalf("status = newest scan, not delivered: %+v", res)
	}
	// delivered: the newest scan says so
	res, err = serve(jtPage(jtRow("10:00:00", "2026-10-28", "Giao hàng thành công. Người ký nhận: Linh")+
		jtRow("08:12:58", "2026-10-27", "đang chuyển hàng"))).QueryTracking(context.Background(), "841328496973", "1234")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Delivered || len(res.Events) != 2 {
		t.Fatalf("a signed parcel is delivered: %+v", res)
	}
}

func TestTheOfficialAPIJourneyIsAlsoGivenAsEvents(t *testing.T) {
	c := jtServers(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"details":[
			{"scanTime":"2026-10-27 08:12:58","desc":"Đang chuyển hàng đến NAA","scanType":"发件"},
			{"scanTime":"2026-10-26 16:52:53","desc":"Đã nhận hàng","scanType":"收件"}]}]}`))
	}, func(w http.ResponseWriter, r *http.Request) {})
	res, err := c.QueryTracking(context.Background(), "841328496973", "1234")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 2 || res.Events[0].Text != "Đang chuyển hàng đến NAA" || res.Events[0].Time.Hour() != 8 {
		t.Fatalf("events from the official details: %+v", res.Events)
	}
}
