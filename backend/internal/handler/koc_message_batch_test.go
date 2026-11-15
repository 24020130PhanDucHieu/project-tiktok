package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/repository"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// batchKocAPI finds an active KOC for every id except the ones listed in missing.
type batchKocAPI struct {
	KocAPI
	missing map[string]bool
}

func (b *batchKocAPI) FindActive(_ context.Context, id string) (*domain.Koc, error) {
	if b.missing[id] {
		return nil, domain.NotFound("Không tìm thấy KOC đang hoạt động")
	}
	return &domain.Koc{DisplayName: "KOC " + id}, nil
}

// batchOutreach records what the handler hands to the service and answers with a canned report or error.
type batchOutreach struct {
	OutreachAPI
	calls   int
	body    string
	targets []service.BatchTarget
	report  *service.BatchReport
	err     error
}

func (b *batchOutreach) SendTemplateBatch(_ context.Context, body string, targets []service.BatchTarget) (*service.BatchReport, error) {
	b.calls++
	b.body, b.targets = body, targets
	return b.report, b.err
}

// detailAudit keeps the action and the detail of every audit line.
type detailAudit struct{ actions, details []string }

func (a *detailAudit) Log(_ context.Context, _, _, action, _, _, detail, _ string) {
	a.actions = append(a.actions, action)
	a.details = append(a.details, detail)
}

func batchHandlerApp(api *batchKocAPI, out *batchOutreach, au *detailAudit) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/kocs/message-batch", NewKocHandler(api, out, nil, au).SendMessageBatch)
	return app
}

func newBatchFixtures() (*batchKocAPI, *batchOutreach, *detailAudit) {
	return &batchKocAPI{missing: map[string]bool{}}, &batchOutreach{report: &service.BatchReport{}}, &detailAudit{}
}

func TestMessageBatchNeedsConfirmAndABody(t *testing.T) {
	api, out, au := newBatchFixtures()
	app := batchHandlerApp(api, out, au)

	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"Chào","kocIds":["a"]}`); code != 400 {
		t.Fatalf("a request that is not confirmed must be refused: %d", code)
	}
	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"   ","kocIds":["a"],"confirm":true}`); code != 400 {
		t.Fatalf("a body of spaces must be refused: %d", code)
	}
	if out.calls != 0 {
		t.Fatalf("the service must not be called, got %d calls", out.calls)
	}
}

func TestMessageBatchRefusesMoreThanFiveKocsOrNone(t *testing.T) {
	api, out, au := newBatchFixtures()
	app := batchHandlerApp(api, out, au)

	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"Chào","kocIds":["a","b","c","d","e","f"],"confirm":true}`); code != 400 {
		t.Fatalf("6 KOCs in one request: want 400, got %d", code)
	}
	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"Chào","kocIds":[],"confirm":true}`); code != 400 {
		t.Fatalf("no KOC: want 400, got %d", code)
	}
	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"Chào","kocIds":["a","b","c","d","e"],"confirm":true}`); code != 200 {
		t.Fatalf("5 KOCs are fine: %d", code)
	}
}

func TestMessageBatchRefusesABodyOverTwoThousandCharacters(t *testing.T) {
	api, out, au := newBatchFixtures()
	app := batchHandlerApp(api, out, au)

	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"`+strings.Repeat("ạ", 2001)+`","kocIds":["a"],"confirm":true}`); code != 400 {
		t.Fatalf("2001 characters: want 400, got %d", code)
	}
	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"`+strings.Repeat("ạ", 2000)+`","kocIds":["a"],"confirm":true}`); code != 200 {
		t.Fatalf("2000 characters are fine: %d", code)
	}
}

func TestMessageBatchHandsTheLoadedKocsToTheService(t *testing.T) {
	api, out, au := newBatchFixtures()
	api.missing["b"] = true
	app := batchHandlerApp(api, out, au)

	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"  Chào {ten}  ","kocIds":["a","b"],"confirm":true}`); code != 200 {
		t.Fatalf("want 200, got %d", code)
	}
	if out.body != "Chào {ten}" {
		t.Fatalf("the body is trimmed, got %q", out.body)
	}
	if len(out.targets) != 2 || out.targets[0].ID != "a" || out.targets[0].Koc == nil || out.targets[1].ID != "b" || out.targets[1].Koc != nil {
		t.Fatalf("a found KOC is handed over, a missing one as nil: %+v", out.targets)
	}
}

func TestMessageBatchSendsEachKocOnlyOnceEvenIfListedTwice(t *testing.T) {
	api, out, au := newBatchFixtures()
	app := batchHandlerApp(api, out, au)

	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"Chào","kocIds":["a","a","b"],"confirm":true}`); code != 200 {
		t.Fatalf("want 200, got %d", code)
	}
	if len(out.targets) != 2 || out.targets[0].ID != "a" || out.targets[1].ID != "b" {
		t.Fatalf("a repeated id is sent once, in the order given: %+v", out.targets)
	}
}

func TestMessageBatchAnswersWithTheServicesReport(t *testing.T) {
	api, out, au := newBatchFixtures()
	out.report = &service.BatchReport{
		Results: []service.BatchResult{{KocID: "a", Status: service.BatchSent}},
		Stop:    true, StopReason: "daily_quota_reached",
	}
	app := batchHandlerApp(api, out, au)

	code, body := postJSON(app, "/kocs/message-batch", `{"body":"Chào","kocIds":["a"],"confirm":true}`)
	data, _ := body["data"].(map[string]any)
	results, _ := data["results"].([]any)
	if code != 200 || data["stop"] != true || data["stopReason"] != "daily_quota_reached" || len(results) != 1 {
		t.Fatalf("the report goes out as it is: %d %v", code, body)
	}
	if first, _ := results[0].(map[string]any); first["kocId"] != "a" || first["status"] != "sent" {
		t.Fatalf("the result keeps its fields: %v", results[0])
	}
}

func TestMessageBatchPassesTheSwitchOffErrorThrough(t *testing.T) {
	api, out, au := newBatchFixtures()
	out.report, out.err = nil, service.ErrAutomationOff
	app := batchHandlerApp(api, out, au)

	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"Chào","kocIds":["a"],"confirm":true}`); code != 409 {
		t.Fatalf("the switch being off is a 409: %d", code)
	}
}

func TestMessageBatchLogsOneAuditLineWithTheCountsNotTheText(t *testing.T) {
	api, out, au := newBatchFixtures()
	out.report = &service.BatchReport{Results: []service.BatchResult{
		{KocID: "a", Status: service.BatchSent}, {KocID: "b", Status: service.BatchSkipped, Reason: "no_phone"},
	}}
	app := batchHandlerApp(api, out, au)

	postJSON(app, "/kocs/message-batch", `{"body":"Bí mật nội dung","kocIds":["a","b"],"confirm":true}`)
	if len(au.actions) != 1 || au.actions[0] != "outreach.message_batch" {
		t.Fatalf("one audit line named outreach.message_batch, got %v", au.actions)
	}
	if d := au.details[0]; !strings.Contains(d, "sent=1") || !strings.Contains(d, "skipped=1") || strings.Contains(d, "Bí mật nội dung") {
		t.Fatalf("the detail holds the counts and never the text: %q", d)
	}
}

// ---- the log of bulk sends ("theo dõi") and the switch of the bulk send ----

type memSendLog struct {
	rows    []domain.BulkSendLog
	failing bool
}

func (m *memSendLog) Record(_ context.Context, rows []domain.BulkSendLog) error {
	if m.failing {
		return errors.New("db down")
	}
	m.rows = append(m.rows, rows...)
	return nil
}

func (m *memSendLog) Recent(_ context.Context, limit int64) ([]domain.BulkSendLog, error) {
	out := []domain.BulkSendLog{}
	for i := len(m.rows) - 1; i >= 0 && int64(len(out)) < limit; i-- {
		out = append(out, m.rows[i])
	}
	return out, nil
}

func batchAppWithLog(api *batchKocAPI, out *batchOutreach, au *detailAudit, log SendLog) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	h := NewKocHandler(api, out, nil, au).WithSendLog(log)
	app.Post("/kocs/message-batch", h.SendMessageBatch)
	app.Get("/kocs/message-log", h.MessageLog)
	return app
}

func TestMessageBatchRecordsOneLogRowPerKocAndNeverTheWholeText(t *testing.T) {
	api, out, au := newBatchFixtures()
	out.report = &service.BatchReport{Results: []service.BatchResult{
		{KocID: "a", Status: service.BatchSent}, {KocID: "b", Status: service.BatchSkipped, Reason: "no_phone"},
	}}
	log := &memSendLog{}
	app := batchAppWithLog(api, out, au, log)
	long := strings.Repeat("x", 300)

	postJSON(app, "/kocs/message-batch", `{"body":"Chào {ten} `+long+`","kocIds":["a","b"],"confirm":true,"runId":"run-1"}`)
	if len(log.rows) != 2 || log.rows[0].RunID != "run-1" || log.rows[0].KocID != "a" || log.rows[0].Status != "sent" || log.rows[1].Reason != "no_phone" {
		t.Fatalf("one row per KOC with its outcome: %+v", log.rows)
	}
	if log.rows[0].KocName != "KOC a" || log.rows[0].At.IsZero() {
		t.Fatalf("the KOC's name and the time are kept: %+v", log.rows[0])
	}
	if n := len([]rune(log.rows[0].Preview)); n == 0 || n > 80 {
		t.Fatalf("only the beginning of the text is kept, at most 80 characters, got %d", n)
	}
}

func TestMessageBatchGivesTheRunAnIdWhenTheClientSentNone(t *testing.T) {
	api, out, au := newBatchFixtures()
	out.report = &service.BatchReport{Results: []service.BatchResult{{KocID: "a", Status: service.BatchSent}}}
	log := &memSendLog{}
	postJSON(batchAppWithLog(api, out, au, log), "/kocs/message-batch", `{"body":"Chào","kocIds":["a"],"confirm":true}`)
	if len(log.rows) != 1 || log.rows[0].RunID == "" {
		t.Fatalf("a run without an id still gets one: %+v", log.rows)
	}
}

func TestMessageBatchStillAnswersWhenTheLogCannotBeWritten(t *testing.T) {
	api, out, au := newBatchFixtures()
	out.report = &service.BatchReport{Results: []service.BatchResult{{KocID: "a", Status: service.BatchSent}}}
	app := batchAppWithLog(api, out, au, &memSendLog{failing: true})
	if code, _ := postJSON(app, "/kocs/message-batch", `{"body":"Chào","kocIds":["a"],"confirm":true}`); code != 200 {
		t.Fatalf("the messages went out: the answer must not become an error because the log failed: %d", code)
	}
}

func TestMessageLogListsTheRecentRowsNewestFirstAndIsEmptyWithoutAStore(t *testing.T) {
	api, out, au := newBatchFixtures()
	log := &memSendLog{rows: []domain.BulkSendLog{{RunID: "old", Status: "sent"}, {RunID: "new", Status: "skipped"}}}
	code, body := getJSON(batchAppWithLog(api, out, au, log), "/kocs/message-log?limit=1")
	rows, _ := body["data"].([]any)
	if code != 200 || len(rows) != 1 || rows[0].(map[string]any)["runId"] != "new" {
		t.Fatalf("newest first and limited: %d %v", code, body)
	}
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Get("/kocs/message-log", NewKocHandler(api, out, nil, au).MessageLog)
	code, body = getJSON(app, "/kocs/message-log")
	if empty, ok := body["data"].([]any); code != 200 || !ok || len(empty) != 0 {
		t.Fatalf("no store: an empty list, not null: %d %v", code, body)
	}
}

type memBulkSwitch struct{ on bool }

func (m *memBulkSwitch) State(context.Context) (repository.AutomationState, error) {
	return repository.AutomationState{Enabled: m.on}, nil
}
func (m *memBulkSwitch) SetEnabled(_ context.Context, enabled bool, _ string) error {
	m.on = enabled
	return nil
}

func TestTheBulkSwitchNeedsAConfirmationToTurnOnAndLogsItsOwnActions(t *testing.T) {
	au, sw := &detailAudit{}, &memBulkSwitch{}
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	h := NewBulkSwitchHandler(sw, au)
	app.Get("/bulk-switch", h.Get)
	app.Put("/bulk-switch", h.Set)

	if code := putStatus(app, "/bulk-switch", `{"enabled":true}`); code != 400 || sw.on {
		t.Fatalf("turning on without confirm is refused: %d on=%v", code, sw.on)
	}
	if code := putStatus(app, "/bulk-switch", `{"enabled":true,"confirm":true}`); code != 200 || !sw.on {
		t.Fatalf("confirmed: %d on=%v", code, sw.on)
	}
	if code := putStatus(app, "/bulk-switch", `{"enabled":false}`); code != 200 || sw.on {
		t.Fatalf("turning off needs no confirmation: %d on=%v", code, sw.on)
	}
	if len(au.actions) != 2 || au.actions[0] != "bulk_switch.on" || au.actions[1] != "bulk_switch.off" {
		t.Fatalf("its own audit actions, not the master switch's: %v", au.actions)
	}
	_, body := getJSON(app, "/bulk-switch")
	if data, _ := body["data"].(map[string]any); data["enabled"] != false {
		t.Fatalf("GET reports the state: %v", body)
	}
}

func getJSON(app *fiber.App, path string) (int, map[string]any) {
	resp, _ := app.Test(httptest.NewRequest("GET", path, nil), 10000)
	b, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func putStatus(app *fiber.App, path, body string) int {
	req := httptest.NewRequest("PUT", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, 10000)
	return resp.StatusCode
}
