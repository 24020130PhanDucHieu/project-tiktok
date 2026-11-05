package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/service"
)

type fakeContentTTS struct {
	mu         sync.Mutex // VideoTraffic is called from several goroutines
	lines      []service.AffiliateLine
	linesErr   error
	videos     []service.AnalyticsVideo
	videosErr  error
	traffic    *service.VideoTraffic
	target     *service.TargetDetail
	lineCalls  int
	fromSeen   time.Time
	trafficIDs []string
}

func (f *fakeContentTTS) AffiliateLines(_ context.Context, from, _ time.Time) ([]service.AffiliateLine, bool, error) {
	f.lineCalls++
	f.fromSeen = from
	return f.lines, false, f.linesErr
}
func (f *fakeContentTTS) ShopVideos(context.Context, time.Time, time.Time) ([]service.AnalyticsVideo, error) {
	return f.videos, f.videosErr
}
func (f *fakeContentTTS) VideoTraffic(_ context.Context, id string, _, _ time.Time) (*service.VideoTraffic, error) {
	f.mu.Lock()
	f.trafficIDs = append(f.trafficIDs, id)
	f.mu.Unlock()
	return f.traffic, nil
}
func (f *fakeContentTTS) GetTarget(context.Context, string) (*service.TargetDetail, error) {
	return f.target, nil
}

type fakeContentKocs struct {
	kocs  map[string]*domain.Koc
	saved int
}

func (f *fakeContentKocs) FindActiveByID(_ context.Context, id string) (*domain.Koc, error) {
	return f.kocs[id], nil
}
func (f *fakeContentKocs) Find(context.Context, bson.M) ([]*domain.Koc, error) {
	out := []*domain.Koc{}
	for _, k := range f.kocs {
		out = append(out, k)
	}
	return out, nil
}
func (f *fakeContentKocs) Save(context.Context, *domain.Koc) error { f.saved++; return nil }

type fakeContentCollabs struct {
	byID  map[string]*domain.Collab
	saves int
}

func (f *fakeContentCollabs) FindByID(_ context.Context, id string) (*domain.Collab, error) {
	return f.byID[id], nil
}
func (f *fakeContentCollabs) Save(_ context.Context, c *domain.Collab) error {
	f.saves++
	c.Recompute()
	return nil
}

type contentEnv struct {
	h      *ContentHandler
	tts    *fakeContentTTS
	kocs   *fakeContentKocs
	collab *fakeContentCollabs
	app    *fiber.App
	koc    *domain.Koc
	col    *domain.Collab
}

func line(order, creator, content, product string, at time.Time, net float64, settle string) service.AffiliateLine {
	return service.AffiliateLine{OrderID: order, CreatedAt: at, Creator: creator, ContentID: content, ContentType: "VIDEO", ProductID: product,
		Quantity: 1, Gross: net, Net: net, Commission: net / 10, Settlement: settle}
}

func newContentEnv() *contentEnv {
	koc := &domain.Koc{ID: primitive.NewObjectID(), TiktokID: "@ChangDiTap1802", DisplayName: "Chang"}
	other := &domain.Koc{ID: primitive.NewObjectID(), TiktokID: "other.kol"}
	started := time.Now().Add(-48 * time.Hour)
	col := &domain.Collab{ID: primitive.NewObjectID(), KocID: koc.ID, Status: domain.CollabActive, FeeSet: true, Mode: "open", ProductID: "111",
		CommissionRate: 10, StartedAt: started, CreatedAt: started}
	now := time.Now()
	tts := &fakeContentTTS{lines: []service.AffiliateLine{
		line("o1", "changditap1802", "7689311292443675925", "111", now.Add(-24*time.Hour), 100000, "SETTLED"),
		line("o2", "changditap1802", "7689311292443675925", "111", now.Add(-23*time.Hour), 50000, "To-SETTLE"),
		line("o3", "changditap1802", "7689311292443675900", "222", now.Add(-22*time.Hour), 70000, "To-SETTLE"),      // another product
		line("o4", "changditap1802", "7689311292443675800", "111", started.Add(-30*24*time.Hour), 90000, "SETTLED"), // before the collaboration
		line("o5", "other.kol", "7689311292443675000", "111", now.Add(-20*time.Hour), 40000, "SETTLED"),             // another creator
	}, videosErr: service.ErrAnalyticsScope}
	ks := &fakeContentKocs{kocs: map[string]*domain.Koc{koc.ID.Hex(): koc, other.ID.Hex(): other}}
	cs := &fakeContentCollabs{byID: map[string]*domain.Collab{col.ID.Hex(): col}}
	h := NewContentHandler(tts, nil, ks, cs)
	app := fiber.New()
	app.Get("/kocs/:id/videos", h.KocVideos)
	app.Get("/content/videos", h.Videos)
	app.Get("/collabs/:cid/performance", h.Performance)
	return &contentEnv{h, tts, ks, cs, app, koc, col}
}

func (e *contentEnv) get(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	resp, err := e.app.Test(httptest.NewRequest("GET", path, nil), 10000)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func dataOf(m map[string]any) map[string]any { d, _ := m["data"].(map[string]any); return d }

func TestKocVideosListsOnlyThatCreatorsRealSalesWithTheVideoLink(t *testing.T) {
	e := newContentEnv()
	code, body := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos?days=90")
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	d := dataOf(body)
	vids := d["videos"].([]any)
	if len(vids) != 3 {
		t.Fatalf("this creator has 3 videos with sales, got %d: %v", len(vids), vids)
	}
	for _, v := range vids {
		m := v.(map[string]any)
		if m["username"] != "changditap1802" {
			t.Fatalf("another creator's video leaked in: %v", m)
		}
		if m["views"] != nil || m["likes"] != nil {
			t.Fatalf("views and likes must stay unknown (null) without the Analytics scope: %v", m)
		}
	}
	first := vids[0].(map[string]any)
	if first["url"] == "" || first["id"] == "" {
		t.Fatalf("every video needs its link: %v", first)
	}
	an := d["analytics"].(map[string]any)
	if an["available"] != false || an["note"] == "" {
		t.Fatalf("the missing scope must be explained: %v", an)
	}
	if d["totals"].(map[string]any)["orders"].(float64) != 4 {
		t.Fatalf("totals: %v", d["totals"])
	}
}

func TestKocVideosFiltersByProductAndRejectsABadProductID(t *testing.T) {
	e := newContentEnv()
	_, body := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos?productId=222")
	if v := dataOf(body)["videos"].([]any); len(v) != 1 {
		t.Fatalf("only the p2 video: %v", v)
	}
	if code, _ := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos?productId=abc%27"); code != 400 {
		t.Fatalf("a malformed product id must be refused, got %d", code)
	}
}

func TestKocVideosAnalyticsFillsViewsAndLikesAndAddsVideosWithoutSales(t *testing.T) {
	e := newContentEnv()
	e.tts.videosErr = nil
	e.tts.videos = []service.AnalyticsVideo{
		{ID: "7689311292443675925", Username: "changditap1802", Views: 291, PostedAt: time.Now().Add(-72 * time.Hour), Products: []string{"111"}},
		{ID: "7689311292443600000", Username: "changditap1802", Views: 371, PostedAt: time.Now().Add(-70 * time.Hour), Products: []string{"111"}}, // no sales
		{ID: "7689311292443611111", Username: "other.kol", Views: 5000, PostedAt: time.Now()},
	}
	e.tts.traffic = &service.VideoTraffic{Views: 291, Likes: 3, Comments: 1, Shares: 2}
	_, body := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos")
	d := dataOf(body)
	if d["analytics"].(map[string]any)["available"] != true {
		t.Fatalf("analytics should be available: %v", d["analytics"])
	}
	found := map[string]map[string]any{}
	for _, v := range d["videos"].([]any) {
		m := v.(map[string]any)
		found[m["id"].(string)] = m
	}
	if found["7689311292443675925"]["views"].(float64) != 291 || found["7689311292443675925"]["likes"].(float64) != 3 {
		t.Fatalf("views and likes: %v", found["7689311292443675925"])
	}
	if nosale, ok := found["7689311292443600000"]; !ok || nosale["orders"].(float64) != 0 {
		t.Fatalf("a video with views but no sale must still be listed: %v", found)
	}
	if _, leaked := found["7689311292443611111"]; leaked {
		t.Fatal("another creator's video must not be listed")
	}
	if v := d["views"].(float64); v != 291+371 {
		t.Fatalf("views total only of this creator: %v", v)
	}
}

func TestKocWithoutATikTokNameCannotBeLookedUp(t *testing.T) {
	e := newContentEnv()
	e.koc.TiktokID = ""
	if code, _ := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos"); code != 422 {
		t.Fatalf("got %d", code)
	}
}

func TestPerformanceCountsOnlyThisKocThisProductAndSinceTheStart(t *testing.T) {
	e := newContentEnv()
	code, body := e.get(t, "/collabs/"+e.col.ID.Hex()+"/performance")
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if e.collab.saves != 1 || e.col.Real == nil {
		t.Fatal("the real figures must be saved on the collaboration")
	}
	r := e.col.Real
	if r.Orders != 2 || r.GMV != 150000 || !r.ByProduct || r.Videos != 1 {
		t.Fatalf("only o1+o2 (this KOC, product p1, after the start): %+v", r)
	}
	if r.Commission != 15000 || r.CommissionConfirmed != 10000 {
		t.Fatalf("commission %v confirmed %v", r.Commission, r.CommissionConfirmed)
	}
	if e.col.Perf.Source != "tiktok" || e.col.Perf.Revenue != 150000 {
		t.Fatalf("perf must come from TikTok now: %+v", e.col.Perf)
	}
	if e.col.Step != 4 {
		t.Fatalf("a video with real sales means the result step: %d", e.col.Step)
	}
	if e.kocs.saved != 1 {
		t.Fatal("the KOC's step tag must follow")
	}
}

func TestPerformanceUsesTheTargetInvitationProducts(t *testing.T) {
	e := newContentEnv()
	e.col.ProductID, e.col.TargetCollabID, e.col.Mode = "", "7001", "targeted"
	e.tts.target = &service.TargetDetail{Products: []service.TargetDetailProduct{{ID: "222"}}}
	e.get(t, "/collabs/"+e.col.ID.Hex()+"/performance")
	if r := e.col.Real; r == nil || r.Orders != 1 || r.GMV != 70000 {
		t.Fatalf("the products of the invitation decide: %+v", r)
	}
}

func TestPerformanceWithoutAProductIDCountsEverythingTheKocSoldAndSaysSo(t *testing.T) {
	e := newContentEnv()
	e.col.ProductID, e.col.Mode = "", "product"
	_, body := e.get(t, "/collabs/"+e.col.ID.Hex()+"/performance")
	if r := e.col.Real; r == nil || r.ByProduct || r.Orders != 3 {
		t.Fatalf("%+v", r)
	}
	if dataOf(body)["note"] == "" {
		t.Fatal("the user must be told the figures are not limited to a product")
	}
}

func TestPerformanceOfAClosedCollaborationIsFrozenUnlessRefreshed(t *testing.T) {
	e := newContentEnv()
	e.col.Status = domain.CollabClosed
	e.col.Real = &domain.CollabReal{Orders: 9}
	e.get(t, "/collabs/"+e.col.ID.Hex()+"/performance")
	if e.tts.lineCalls != 0 || e.col.Real.Orders != 9 {
		t.Fatal("a closed collaboration keeps its final figures")
	}
	e.get(t, "/collabs/"+e.col.ID.Hex()+"/performance?refresh=1")
	if e.tts.lineCalls != 1 {
		t.Fatal("refresh=1 must read TikTok again")
	}
}

func TestContentVideosListsTheKocsOfTheAppAndOnlyThemUnlessAskedForAll(t *testing.T) {
	e := newContentEnv()
	e.kocs.kocs = map[string]*domain.Koc{e.koc.ID.Hex(): e.koc} // "other.kol" is not in the app
	_, body := e.get(t, "/content/videos")
	for _, v := range dataOf(body)["videos"].([]any) {
		if v.(map[string]any)["username"] != "changditap1802" {
			t.Fatalf("only KOCs saved in the app: %v", v)
		}
	}
	_, body = e.get(t, "/content/videos?all=1")
	seen := map[string]bool{}
	for _, v := range dataOf(body)["videos"].([]any) {
		seen[v.(map[string]any)["username"].(string)] = true
	}
	if !seen["other.kol"] || !seen["changditap1802"] {
		t.Fatalf("all=1 shows every creator of the shop: %v", seen)
	}
}

func TestContentFailureMessagesAreFriendly(t *testing.T) {
	e := newContentEnv()
	e.tts.linesErr = &service.TTSRejected{Code: 36009002, Message: "rate"}
	if code, _ := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos"); code != 429 {
		t.Fatalf("got %d", code)
	}
	e.tts.linesErr = service.ErrTTShopNotConnected
	if code, _ := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos"); code != 409 {
		t.Fatalf("got %d", code)
	}
}

type fakeMeta struct {
	mu    sync.Mutex
	byID  map[string]*service.TikTokVideoDetail
	calls int
}

func (f *fakeMeta) FetchVideoDetail(_ context.Context, videoURL string) (*service.TikTokVideoDetail, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	for id, d := range f.byID {
		if strings.HasSuffix(videoURL, "/"+id) {
			return d, nil
		}
	}
	return nil, errors.New("not found")
}

func TestWithoutAnalyticsTheViewsComeFromTheVideoPageAndAreMarkedPublic(t *testing.T) {
	e := newContentEnv()
	meta := &fakeMeta{byID: map[string]*service.TikTokVideoDetail{
		"7689311292443675925": {ID: "7689311292443675925", Username: "changditap1802", Title: "Khăn lạnh", CoverURL: "https://cdn/c.jpg", Views: 354, Likes: 5, Comments: 1, Shares: 2},
		"7689311292443675900": {ID: "7689311292443675900", Username: "changditap1802", Views: 100},
		"7689311292443675800": {ID: "7689311292443675800", Username: "changditap1802", Views: 46},
	}}
	e.h.meta = meta
	_, body := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos")
	d := dataOf(body)
	found := map[string]map[string]any{}
	for _, v := range d["videos"].([]any) {
		m := v.(map[string]any)
		found[m["id"].(string)] = m
	}
	first := found["7689311292443675925"]
	if first["views"].(float64) != 354 || first["likes"].(float64) != 5 || first["statsSource"] != "public" || first["coverUrl"] != "https://cdn/c.jpg" || first["title"] != "Khăn lạnh" {
		t.Fatalf("public counters, title and cover expected: %v", first)
	}
	if d["views"].(float64) != 500 || d["viewsPublic"] != true {
		t.Fatalf("views total of the public counters, marked as such: %v %v", d["views"], d["viewsPublic"])
	}
}

func TestAPageOfAnotherCreatorIsNeverShownUnderThisVideo(t *testing.T) {
	e := newContentEnv()
	e.h.meta = &fakeMeta{byID: map[string]*service.TikTokVideoDetail{
		"7689311292443675925": {ID: "7689311292443675925", Username: "someone.else", Title: "wrong", Views: 999999},
	}}
	_, body := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos")
	for _, v := range dataOf(body)["videos"].([]any) {
		m := v.(map[string]any)
		if m["id"] == "7689311292443675925" && (m["views"] != nil || m["title"] != "") {
			t.Fatalf("a page of another creator must be ignored: %v", m)
		}
	}
}

func TestAnalyticsNumbersWinOverThePublicOnes(t *testing.T) {
	e := newContentEnv()
	e.tts.videosErr = nil
	e.tts.videos = []service.AnalyticsVideo{{ID: "7689311292443675925", Username: "changditap1802", Views: 291, PostedAt: time.Now().Add(-72 * time.Hour)}}
	e.tts.traffic = &service.VideoTraffic{Views: 291, Likes: 3}
	e.h.meta = &fakeMeta{byID: map[string]*service.TikTokVideoDetail{"7689311292443675925": {ID: "7689311292443675925", Username: "changditap1802", Views: 354, Likes: 5}}}
	_, body := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos")
	for _, v := range dataOf(body)["videos"].([]any) {
		m := v.(map[string]any)
		if m["id"] == "7689311292443675925" && (m["views"].(float64) != 291 || m["statsSource"] != "analytics") {
			t.Fatalf("%v", m)
		}
	}
}

func TestAFailedPageReadIsNotRepeatedRightAway(t *testing.T) {
	e := newContentEnv()
	meta := &fakeMeta{byID: map[string]*service.TikTokVideoDetail{}} // every read fails, like TikTok's bot check
	e.h.meta = meta
	e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos")
	first := meta.calls
	e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos")
	if first == 0 || meta.calls != first {
		t.Fatalf("a refused page must not be asked again at once: %d then %d", first, meta.calls)
	}
}

func TestAPeriodOlderThanTheLookbackExplainsItAndKeepsTheSales(t *testing.T) {
	e := newContentEnv()
	e.tts.videosErr = service.ErrAnalyticsRange
	code, body := e.get(t, "/kocs/"+e.koc.ID.Hex()+"/videos?days=365")
	if code != 200 {
		t.Fatalf("an old period must not be an error: %d %v", code, body)
	}
	d := dataOf(body)
	if d["analytics"].(map[string]any)["note"] == "" || d["totals"].(map[string]any)["orders"].(float64) == 0 {
		t.Fatalf("the note must explain and the sales must stay: %v", d)
	}
}
