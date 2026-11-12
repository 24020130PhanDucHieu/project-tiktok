package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/service"
)

type fakeTargetTTS struct {
	preview    *service.TargetPreview
	previewErr error
	createID   string
	createErr  error
	page       *service.TargetPage
	target     *service.TargetDetail
	targetErr  error

	previews, creates, removes int
	created                    *service.TargetAPIPayload
}

func (f *fakeTargetTTS) SearchCreators(context.Context, string, int) ([]service.TargetCreator, error) {
	return nil, nil
}
func (f *fakeTargetTTS) ListProducts(context.Context, string, int) (*service.ShopProductPage, error) {
	return nil, nil
}
func (f *fakeTargetTTS) ProductImages(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		out[id] = "https://img/" + id + ".jpg"
	}
	return out, nil
}
func (f *fakeTargetTTS) PreviewTarget(context.Context, service.TargetInviteInput, time.Time, int) (*service.TargetPreview, error) {
	f.previews++
	return f.preview, f.previewErr
}
func (f *fakeTargetTTS) CreateTarget(_ context.Context, p *service.TargetAPIPayload) (string, error) {
	f.creates++
	f.created = p
	return f.createID, f.createErr
}
func (f *fakeTargetTTS) SearchTargets(context.Context, service.TargetListParams) (*service.TargetPage, error) {
	return f.page, nil
}
func (f *fakeTargetTTS) GetTarget(context.Context, string) (*service.TargetDetail, error) {
	if f.targetErr != nil {
		return nil, f.targetErr
	}
	if f.target != nil {
		return f.target, nil
	}
	return &service.TargetDetail{TargetSummary: service.TargetSummary{ID: "9", Type: "STANDARD"}}, nil
}
func (f *fakeTargetTTS) TargetLink(context.Context, string) (string, error) {
	return "https://link", nil
}
func (f *fakeTargetTTS) RemoveTarget(context.Context, string) error { f.removes++; return nil }

type fakeTargetStore struct {
	byKey     map[string]*domain.TargetInvite
	results   map[domain.ID]string
	cancelled []string
	local     map[string]domain.TargetInvite
	enabled   bool
	switchErr error
	setCalls  []bool
	contact   domain.ShopContact
	recent    []domain.TargetInvite
}

func newFakeStore() *fakeTargetStore {
	return &fakeTargetStore{byKey: map[string]*domain.TargetInvite{}, results: map[domain.ID]string{}, local: map[string]domain.TargetInvite{}}
}
func (s *fakeTargetStore) Insert(_ context.Context, inv *domain.TargetInvite) (*domain.TargetInvite, error) {
	if ex, ok := s.byKey[inv.RequestKey]; ok {
		return ex, domain.ErrDuplicateRequest
	}
	inv.ID = domain.NewID()
	s.byKey[inv.RequestKey] = inv
	return inv, nil
}
func (s *fakeTargetStore) SetResult(_ context.Context, id domain.ID, status, targetID, _ string) error {
	s.results[id] = status
	for _, inv := range s.byKey {
		if inv.ID == id {
			inv.Status, inv.TargetID = status, targetID
		}
	}
	return nil
}
func (s *fakeTargetStore) RecentSent(context.Context, time.Time) ([]domain.TargetInvite, error) {
	return s.recent, nil
}
func (s *fakeTargetStore) MarkCancelled(_ context.Context, id string) error {
	s.cancelled = append(s.cancelled, id)
	return nil
}
func (s *fakeTargetStore) ByTargetIDs(context.Context, []string) (map[string]domain.TargetInvite, error) {
	return s.local, nil
}
func (s *fakeTargetStore) ByTargetID(context.Context, string) (*domain.TargetInvite, error) {
	return nil, nil
}
func (s *fakeTargetStore) SendingEnabled(context.Context) (bool, error) {
	return s.enabled, s.switchErr
}
func (s *fakeTargetStore) ShopContact(context.Context) (domain.ShopContact, error) {
	return s.contact, nil
}
func (s *fakeTargetStore) SetShopContact(_ context.Context, c domain.ShopContact, _ string) error {
	s.contact = c
	return nil
}
func (s *fakeTargetStore) SetSendingEnabled(_ context.Context, on bool, _ string) error {
	s.setCalls = append(s.setCalls, on)
	s.enabled = on
	return nil
}

type fakeAudit struct{ actions []string }

func (a *fakeAudit) Log(_ context.Context, _, _, action, _, _, _, _ string) {
	a.actions = append(a.actions, action)
}

func readyPreview() *service.TargetPreview {
	p := &service.TargetAPIPayload{Name: "Khăn lạnh", Message: "Chào"}
	p.CreatorUserOpenIDs = []string{"uVERIFIED"}
	return &service.TargetPreview{
		Payload: p, Ready: true,
		Creators: []service.TargetCreator{{OpenID: "uVERIFIED", Username: "trlyng.review", Nickname: "Tủ đồ"}},
		Products: []service.ShopProduct{{ID: "1", Title: "Khăn lạnh", Status: "ACTIVATE"}},
	}
}

type targetEnv struct {
	h     *TargetCollabHandler
	tts   *fakeTargetTTS
	store *fakeTargetStore
	audit *fakeAudit
	app   *fiber.App
}

func newTargetEnv(enabled bool) *targetEnv {
	tts := &fakeTargetTTS{preview: readyPreview(), createID: "7001"}
	st, au := newFakeStore(), &fakeAudit{}
	st.enabled = enabled
	h := NewTargetCollabHandler(tts, st, au, 10)
	app := fiber.New()
	app.Get("/config", h.Config)
	app.Put("/switch", h.SetSwitch)
	app.Post("/create", h.Create)
	app.Post("/cancel/:id", h.Cancel)
	app.Get("/list", h.List)
	return &targetEnv{h, tts, st, au, app}
}

func (e *targetEnv) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func createBody(key string, confirm bool) map[string]any {
	return map[string]any{
		"requestKey": key, "confirm": confirm,
		"invite": map[string]any{
			"name": "Khăn lạnh", "creators": []map[string]string{{"openId": "uFORGED", "username": "someone.else"}},
			"products": []map[string]any{{"id": "1", "commissionPct": 10}},
		},
	}
}

const goodKey = "0123456789abcdef-0123"

func TestCreateIsBlockedWhileTheServerSwitchIsOff(t *testing.T) {
	e := newTargetEnv(false)
	code, body := e.post(t, "/create", createBody(goodKey, true))
	if code != fiber.StatusConflict || !strings.Contains(body["message"].(string), "Cho phép gửi lời mời thật") {
		t.Fatalf("got %d %v", code, body)
	}
	if e.tts.previews != 0 || e.tts.creates != 0 || len(e.store.byKey) != 0 {
		t.Fatal("nothing may be checked, stored or sent while sending is switched off")
	}
}

func TestCreateNeedsExplicitConfirmationAndARequestKey(t *testing.T) {
	e := newTargetEnv(true)
	if code, _ := e.post(t, "/create", createBody(goodKey, false)); code != fiber.StatusBadRequest {
		t.Fatalf("no confirmation must be refused, got %d", code)
	}
	if code, _ := e.post(t, "/create", createBody("short", true)); code != fiber.StatusBadRequest {
		t.Fatalf("a bad request key must be refused, got %d", code)
	}
	if e.tts.creates != 0 || len(e.store.byKey) != 0 {
		t.Fatal("nothing may be sent without confirmation")
	}
}

func TestCreateRefusesWhenTheServerRecheckFindsProblems(t *testing.T) {
	e := newTargetEnv(true)
	e.tts.preview = &service.TargetPreview{Ready: false, Problems: []string{"Không tìm thấy @x trong Creator Marketplace."}}
	code, body := e.post(t, "/create", createBody(goodKey, true))
	if code != fiber.StatusUnprocessableEntity || !strings.Contains(body["message"].(string), "Creator Marketplace") {
		t.Fatalf("got %d %v", code, body)
	}
	if e.tts.creates != 0 || len(e.store.byKey) != 0 {
		t.Fatal("a form with problems must never be stored or sent")
	}
}

func TestCreateSendsOnlyTheServerVerifiedCreatorsAndLogsIt(t *testing.T) {
	e := newTargetEnv(true)
	code, body := e.post(t, "/create", createBody(goodKey, true))
	if code != fiber.StatusCreated {
		t.Fatalf("got %d %v", code, body)
	}
	if e.tts.creates != 1 {
		t.Fatalf("exactly one invitation must be sent, got %d", e.tts.creates)
	}
	if len(e.tts.created.CreatorUserOpenIDs) != 1 || e.tts.created.CreatorUserOpenIDs[0] != "uVERIFIED" {
		t.Fatalf("the creators sent to TikTok must be the verified ones, not the browser's: %v", e.tts.created.CreatorUserOpenIDs)
	}
	rec := e.store.byKey[goodKey]
	if rec.Status != domain.TargetInviteSent || rec.TargetID != "7001" || len(rec.Creators) != 1 || rec.Creators[0].Username != "trlyng.review" {
		t.Fatalf("record: %+v", rec)
	}
	if len(e.audit.actions) != 1 || e.audit.actions[0] != "ttshop.target.create" {
		t.Fatalf("audit: %v", e.audit.actions)
	}
}

func TestCreateTwiceWithTheSameKeySendsOnce(t *testing.T) {
	e := newTargetEnv(true)
	e.post(t, "/create", createBody(goodKey, true))
	code, body := e.post(t, "/create", createBody(goodKey, true))
	if code != fiber.StatusOK || body["data"].(map[string]any)["duplicate"] != true {
		t.Fatalf("a repeated request must return the first result, got %d %v", code, body)
	}
	if e.tts.creates != 1 {
		t.Fatalf("a double click must not send twice, sent %d", e.tts.creates)
	}
}

func TestCreateAfterARejectionIsNotRetriedWithTheSameKey(t *testing.T) {
	e := newTargetEnv(true)
	e.tts.createID, e.tts.createErr = "", &service.TTSRejected{Code: 36009999, Message: "denied"}
	code, _ := e.post(t, "/create", createBody(goodKey, true))
	if code != fiber.StatusBadGateway || e.store.byKey[goodKey].Status != domain.TargetInviteFailed {
		t.Fatalf("a refusal must be reported and recorded as failed, got %d %s", code, e.store.byKey[goodKey].Status)
	}
	if code, _ := e.post(t, "/create", createBody(goodKey, true)); code != fiber.StatusConflict || e.tts.creates != 1 {
		t.Fatalf("the same key must not be sent again (new attempt = new key), got %d creates=%d", code, e.tts.creates)
	}
	if e.audit.actions[0] != "ttshop.target.create_failed" {
		t.Fatalf("audit: %v", e.audit.actions)
	}
}

func TestCreateReportsConflictsAndNeverCallsItSuccess(t *testing.T) {
	e := newTargetEnv(true)
	e.tts.createID = ""
	e.tts.createErr = &service.ErrTargetConflict{Conflicts: []service.TargetConflict{{CreatorOpenID: "uVERIFIED", ProductID: "1"}}}
	code, body := e.post(t, "/create", createBody(goodKey, true))
	if code != fiber.StatusConflict || body["conflicts"] == nil || e.store.byKey[goodKey].Status != domain.TargetInviteFailed {
		t.Fatalf("got %d %v %s", code, body, e.store.byKey[goodKey].Status)
	}
}

func TestCreateAfterABrokenConnectionIsMarkedUnknownNotFailed(t *testing.T) {
	e := newTargetEnv(true)
	e.tts.createID, e.tts.createErr = "", errors.New("không gọi được TikTok Shop: timeout")
	code, body := e.post(t, "/create", createBody(goodKey, true))
	if code != fiber.StatusBadGateway || !strings.Contains(body["message"].(string), "Chưa rõ") {
		t.Fatalf("got %d %v", code, body)
	}
	if e.store.byKey[goodKey].Status != domain.TargetInviteUnknown {
		t.Fatalf("an ambiguous failure must be recorded as unknown, got %s", e.store.byKey[goodKey].Status)
	}
}

func TestCancelNeedsConfirmation(t *testing.T) {
	e := newTargetEnv(true)
	if code, _ := e.post(t, "/cancel/123", map[string]any{"confirm": false}); code != fiber.StatusBadRequest || e.tts.removes != 0 {
		t.Fatalf("no confirmation must not cancel, got %d removes=%d", code, e.tts.removes)
	}
	if code, _ := e.post(t, "/cancel/123", map[string]any{"confirm": true}); code != fiber.StatusOK || e.tts.removes != 1 {
		t.Fatalf("confirmed cancel failed: %d removes=%d", code, e.tts.removes)
	}
	if len(e.store.cancelled) != 1 || e.store.cancelled[0] != "123" || e.audit.actions[len(e.audit.actions)-1] != "ttshop.target.cancel" {
		t.Fatalf("cancel must be recorded and audited: %v %v", e.store.cancelled, e.audit.actions)
	}
}

func TestCancelRefusesInvitationsItMayNotTouch(t *testing.T) {
	e := newTargetEnv(true)
	e.tts.target = &service.TargetDetail{TargetSummary: service.TargetSummary{ID: "9", Type: "TOP_CREATOR_PROGRAM"}}
	if code, _ := e.post(t, "/cancel/9", map[string]any{"confirm": true}); code != fiber.StatusConflict || e.tts.removes != 0 {
		t.Fatalf("a Top Creator Program invitation must not be cancelled here, got %d removes=%d", code, e.tts.removes)
	}
	e.tts.target, e.tts.targetErr = nil, errors.New("không gọi được TikTok Shop: timeout")
	if code, _ := e.post(t, "/cancel/9", map[string]any{"confirm": true}); code != fiber.StatusBadGateway || e.tts.removes != 0 {
		t.Fatalf("when the invitation cannot be read nothing may be cancelled, got %d removes=%d", code, e.tts.removes)
	}
}

func TestCreateRefusesMalformedLocalIDs(t *testing.T) {
	e := newTargetEnv(true)
	body := createBody(goodKey, true)
	body["kocId"] = "not-an-object-id"
	if code, _ := e.post(t, "/create", body); code != fiber.StatusBadRequest {
		t.Fatalf("a malformed kocId must be refused, got %d", code)
	}
	body["kocId"], body["collabId"] = "64b64b64b64b64b64b64b64b", "zzz"
	if code, _ := e.post(t, "/create", body); code != fiber.StatusBadRequest {
		t.Fatalf("a malformed collabId must be refused, got %d", code)
	}
	if e.tts.creates != 0 || len(e.store.byKey) != 0 {
		t.Fatal("nothing may be stored or sent")
	}
}

func TestListAddsWhoSentEachInvitation(t *testing.T) {
	e := newTargetEnv(true)
	e.tts.page = &service.TargetPage{Items: []service.TargetSummary{{ID: "9", Name: "emanhthethao"}, {ID: "10", Name: "other"}}, Total: 2}
	e.store.local["9"] = domain.TargetInvite{SentByEmail: "admin@x.vn", Status: domain.TargetInviteSent, Creators: []domain.TargetInviteCreator{{Username: "trlyng.review"}}}
	resp, err := e.app.Test(httptest.NewRequest("GET", "/list", nil), 10000)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	var out struct {
		Data struct {
			Items []struct {
				ID           string   `json:"id"`
				SentBy       string   `json:"sentBy"`
				CreatorNames []string `json:"creatorNames"`
			} `json:"items"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b, &out)
	if len(out.Data.Items) != 2 || out.Data.Items[0].SentBy != "admin@x.vn" || out.Data.Items[0].CreatorNames[0] != "@trlyng.review" || out.Data.Items[1].SentBy != "" {
		t.Fatalf("got %s", b)
	}
}

// TikTok's list can lag behind a new invitation, or be an old copy while TikTok refuses requests: what was just sent
// from here is on the first page of the plain list, once, and nowhere else.
func TestListCarriesTheInvitationJustSentUntilTikTokListsIt(t *testing.T) {
	sent := domain.TargetInvite{TargetID: "77", Name: "giadinhtraicay", EndDate: "2028-12-31", SentByEmail: "admin@x.vn", Status: domain.TargetInviteSent,
		CreatedAt: time.Now(), Products: []domain.TargetInviteProduct{{ID: "1"}, {ID: "2"}}, Creators: []domain.TargetInviteCreator{{Username: "giadinhtraicay1202"}}}
	ids := func(path string, listed ...string) ([]string, float64) {
		fromHere := true // the row of "77" is the one made from the local record, unless TikTok listed it
		e := newTargetEnv(true)
		items := []service.TargetSummary{}
		for _, id := range listed {
			items = append(items, service.TargetSummary{ID: id})
			if id == "77" {
				fromHere = false
			}
		}
		e.tts.page = &service.TargetPage{Items: items, Total: len(items)}
		e.store.recent = []domain.TargetInvite{sent}
		resp, err := e.app.Test(httptest.NewRequest("GET", path, nil), 10000)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		var out struct {
			Data struct {
				Items []struct {
					ID           string `json:"id"`
					SentBy       string `json:"sentBy"`
					ProductCount int    `json:"productCount"`
					EndTime      int64  `json:"endTime"`
				} `json:"items"`
				Total float64 `json:"total"`
			} `json:"data"`
		}
		_ = json.Unmarshal(b, &out)
		got := []string{}
		for _, it := range out.Data.Items {
			got = append(got, it.ID)
			if it.ID == "77" && fromHere && (it.SentBy != "admin@x.vn" || it.ProductCount != 2 || it.EndTime < time.Now().Unix()) {
				t.Fatalf("the row says who sent it, how many products and when it ends: %s", b)
			}
		}
		return got, out.Data.Total
	}

	if got, total := ids("/list", "9", "10"); len(got) != 3 || got[0] != "77" || total != 3 {
		t.Fatalf("the invitation just sent comes first: %v total %v", got, total)
	}
	if got, total := ids("/list", "9", "77"); len(got) != 2 || total != 2 {
		t.Fatalf("once TikTok lists it, it is not shown twice: %v total %v", got, total)
	}
	for _, path := range []string{"/list?keyword=abc", "/list?pageToken=next", "/list?status=COMPLETED", "/list?acceptStatus=ACCEPT", "/list?freeSample=WITH_FREE_SAMPLE"} {
		if got, _ := ids(path, "9"); len(got) != 1 {
			t.Fatalf("%s is not the plain list, nothing is added: %v", path, got)
		}
	}
}

func TestListHidesWhatWasCancelledFromHereAndFlagsWhatTikTokHasNotListed(t *testing.T) {
	e := newTargetEnv(true)
	e.tts.page = &service.TargetPage{Items: []service.TargetSummary{{ID: "9"}, {ID: "10"}}, Total: 2}
	e.store.local["10"] = domain.TargetInvite{Status: domain.TargetInviteCancelled}
	e.store.recent = []domain.TargetInvite{{TargetID: "77", Name: "new", Status: domain.TargetInviteSent, CreatedAt: time.Now()}}
	resp, err := e.app.Test(httptest.NewRequest("GET", "/list", nil), 10000)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	var out struct {
		Data struct {
			Items []struct {
				ID      string `json:"id"`
				Pending bool   `json:"pending"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b, &out)
	if len(out.Data.Items) != 2 || out.Data.Items[0].ID != "77" || !out.Data.Items[0].Pending || out.Data.Items[1].ID != "9" || out.Data.Items[1].Pending || out.Data.Total != 2 {
		t.Fatalf("the new one first and flagged, the cancelled one gone, the count right: %s", b)
	}
}

func TestListSaysWhenTikTokAnsweredWithAnOldCopy(t *testing.T) {
	e := newTargetEnv(true)
	e.tts.page = &service.TargetPage{Items: []service.TargetSummary{{ID: "9"}}, Total: 1, Stale: true}
	resp, _ := e.app.Test(httptest.NewRequest("GET", "/list", nil), 10000)
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `"stale":true`) {
		t.Fatalf("the screen must be told the list is an old copy: %s", b)
	}
}

func TestConfigExposesTheSwitchAndLimits(t *testing.T) {
	e := newTargetEnv(false)
	resp, _ := e.app.Test(httptest.NewRequest("GET", "/config", nil), 10000)
	b, _ := io.ReadAll(resp.Body)
	for _, want := range []string{`"enabled":false`, `"maxCreators":10`, `"maxProducts":20`, `"minRatePct":10`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("config must contain %s, got %s", want, b)
		}
	}
}

func putJSON(t *testing.T, e *targetEnv, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func TestSwitchStartsOffAndTurningOnNeedsConfirmation(t *testing.T) {
	e := newTargetEnv(false)
	if code, _ := putJSON(t, e, "/switch", map[string]any{"enabled": true}); code != fiber.StatusBadRequest || len(e.store.setCalls) != 0 || e.store.enabled {
		t.Fatalf("turning on without confirmation must be refused and change nothing, got %d calls=%v", code, e.store.setCalls)
	}
	if code, body := putJSON(t, e, "/switch", map[string]any{"enabled": true, "confirm": true}); code != fiber.StatusOK || body["data"].(map[string]any)["enabled"] != true || !e.store.enabled {
		t.Fatalf("confirmed turn on failed: %d %v", code, body)
	}
	if e.audit.actions[len(e.audit.actions)-1] != "ttshop.target.switch_on" {
		t.Fatalf("turning on must be audited: %v", e.audit.actions)
	}
	// once on, sending is allowed to proceed
	if code, _ := e.post(t, "/create", createBody(goodKey, true)); code != fiber.StatusCreated {
		t.Fatalf("with the switch on a valid send must go through, got %d", code)
	}
	// turning off needs no confirmation and takes effect at once
	if code, _ := putJSON(t, e, "/switch", map[string]any{"enabled": false}); code != fiber.StatusOK || e.store.enabled {
		t.Fatalf("turning off failed: %d", code)
	}
	if e.audit.actions[len(e.audit.actions)-1] != "ttshop.target.switch_off" {
		t.Fatalf("turning off must be audited: %v", e.audit.actions)
	}
	if code, _ := e.post(t, "/create", createBody("fedcba9876543210-9999", true)); code != fiber.StatusConflict {
		t.Fatalf("after turning off, sending must be refused, got %d", code)
	}
}

func TestSwitchFailsClosedWhenTheSettingCannotBeRead(t *testing.T) {
	e := newTargetEnv(true)
	e.store.switchErr = errors.New("mongo down")
	if code, _ := e.post(t, "/create", createBody(goodKey, true)); code != fiber.StatusConflict || e.tts.creates != 0 || len(e.store.byKey) != 0 {
		t.Fatalf("an unreadable switch must block sending, got %d creates=%d", code, e.tts.creates)
	}
	resp, _ := e.app.Test(httptest.NewRequest("GET", "/config", nil), 10000)
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `"enabled":false`) {
		t.Fatalf("the config must report off when the switch cannot be read: %s", b)
	}
}

func TestConfigReturnsTheShopContactRememberedFromTheLastInvitation(t *testing.T) {
	e := newTargetEnv(true)
	e.store.contact = domain.ShopContact{Email: "shop@example.com", Zalo: "0901234567", Facebook: "coolshop"}
	resp, _ := e.app.Test(httptest.NewRequest("GET", "/config", nil), 10000)
	b, _ := io.ReadAll(resp.Body)
	for _, want := range []string{`"email":"shop@example.com"`, `"zalo":"0901234567"`, `"facebook":"coolshop"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("config must contain %s, got %s", want, b)
		}
	}
}

func TestASuccessfulInvitationRemembersTheShopContact(t *testing.T) {
	e := newTargetEnv(true)
	body := createBody(goodKey, true)
	body["invite"].(map[string]any)["zalo"] = "0901234567"
	if code, resp := e.post(t, "/create", body); code != fiber.StatusCreated {
		t.Fatalf("got %d %v", code, resp)
	}
	if e.store.contact.Zalo != "0901234567" {
		t.Fatalf("the contact of the invitation must be remembered, got %+v", e.store.contact)
	}
}

func TestAFailedInvitationDoesNotOverwriteTheRememberedContact(t *testing.T) {
	e := newTargetEnv(true)
	e.store.contact = domain.ShopContact{Email: "old@example.com"}
	e.tts.createErr = &service.TTSRejected{Code: 1, Message: "no"}
	e.post(t, "/create", createBody(goodKey, true))
	if e.store.contact.Email != "old@example.com" {
		t.Fatalf("a refused invitation must not change the remembered contact: %+v", e.store.contact)
	}
}

// The picker's product search has no pictures, so the page asks for them separately; it is read-only and open to any
// signed-in member.
func TestTheProductPicturesAreServedForTheIdsAsked(t *testing.T) {
	h := NewTargetCollabHandler(&fakeTargetTTS{}, &fakeTargetStore{}, &fakeAudit{}, 10)
	app := fiber.New()
	app.Get("/images", h.ProductImages)

	req := httptest.NewRequest("GET", "/images?ids=111,%20222,,", nil)
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", err, resp)
	}
	var body struct {
		Data struct {
			Images map[string]string `json:"images"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Images) != 2 || body.Data.Images["111"] != "https://img/111.jpg" || body.Data.Images["222"] == "" {
		t.Fatalf("a picture for each id asked: %v", body.Data.Images)
	}

	resp, _ = app.Test(httptest.NewRequest("GET", "/images", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("no ids is an empty answer, not an error: %d", resp.StatusCode)
	}
}
