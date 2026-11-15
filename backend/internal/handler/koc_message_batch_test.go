package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
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
