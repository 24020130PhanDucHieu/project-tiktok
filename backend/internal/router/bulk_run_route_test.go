package router_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// fakeRunner records what the handler hands to Start and answers with a canned error.
type fakeRunner struct {
	mu      sync.Mutex
	calls   int
	steps   []domain.CampaignStep
	targets []domain.RunTarget
	err     error
}

func (f *fakeRunner) Start(_ context.Context, by, name string, steps []domain.CampaignStep, targets []domain.RunTarget) (*domain.BulkRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	f.steps, f.targets = steps, targets
	return &domain.BulkRun{ID: domain.NewID(), CampaignName: name, Steps: steps, Targets: targets, Status: domain.RunRunning, By: by}, nil
}
func (f *fakeRunner) Estimate(kocs, steps int) time.Duration {
	return time.Duration(kocs*steps) * time.Minute
}

type fakeRunReader struct{ running *domain.BulkRun }

func (f fakeRunReader) Running(context.Context) (*domain.BulkRun, error) { return f.running, nil }
func (f fakeRunReader) Recent(context.Context, int64) ([]domain.BulkRun, error) {
	return []domain.BulkRun{}, nil
}

type memLimit struct{ value int }

func (m *memLimit) Get(context.Context) int { return m.value }
func (m *memLimit) Set(_ context.Context, n int) error {
	if n != 10 && n != 20 && n != 30 && n != 50 {
		return domain.Invalid("Hạn mức chỉ có thể là 10, 20, 30 hoặc 50 KOC mỗi ngày.")
	}
	m.value = n
	return nil
}

// kocFinder finds a KOC for every id except "gone".
type kocFinder struct{}

func (kocFinder) FindActive(_ context.Context, id string) (*domain.Koc, error) {
	if id == "gone" {
		return nil, domain.NotFound("Không tìm thấy KOC đang hoạt động")
	}
	return &domain.Koc{DisplayName: "KOC " + id, TiktokID: "u" + id}, nil
}

func runBody(steps, ids string, confirm bool) string {
	c := "false"
	if confirm {
		c = "true"
	}
	return `{"campaignName":"c","steps":` + steps + `,"kocIds":` + ids + `,"confirm":` + c + `}`
}

const oneText = `[{"kind":"text","text":"Chào {ten}"}]`

func TestBulkRunRoutesNeedALogin(t *testing.T) {
	app, _, _ := newApp(t)
	for _, r := range [][2]string{{"POST", "/api/v1/bulk-runs"}, {"GET", "/api/v1/bulk-runs"}, {"GET", "/api/v1/bulk-runs/current"}, {"GET", "/api/v1/bulk-limit"}, {"PUT", "/api/v1/bulk-limit"}} {
		if code, _ := do(t, app, r[0], r[1], "", `{}`); code != 401 {
			t.Errorf("%s %s needs a login: %d", r[0], r[1], code)
		}
	}
}

func TestAMemberStartsARunOnlyWithConfirmAndValidInput(t *testing.T) {
	app, _, member := newApp(t)
	post := func(body string) int {
		code, _ := do(t, app, "POST", "/api/v1/bulk-runs", member, body)
		return code
	}
	if code := post(runBody(oneText, `["a"]`, false)); code != 400 {
		t.Fatalf("no confirm: %d", code)
	}
	if code := post(runBody(`[]`, `["a"]`, true)); code != 400 {
		t.Fatalf("no steps: %d", code)
	}
	if code := post(runBody(`[{"kind":"text","text":"  "}]`, `["a"]`, true)); code != 400 {
		t.Fatalf("a blank step: %d", code)
	}
	if code := post(runBody(`[{"kind":"image","imageId":"`+domain.NewID().Hex()+`"}]`, `["a"]`, true)); code != 400 {
		t.Fatalf("a picture that is not stored: %d", code)
	}
	if code := post(runBody(oneText, `[]`, true)); code != 400 {
		t.Fatalf("no KOCs: %d", code)
	}
	many := `["` + strings.Repeat(`k","`, 50) + `k"]` // 52 ids
	if code := post(runBody(oneText, many, true)); code != 400 {
		t.Fatalf("more than 50 KOCs: %d", code)
	}
}

func TestAValidRunReachesTheRunnerWithDedupedKocsAndNoConfirmationOnTheRun(t *testing.T) {
	runner := &fakeRunner{}
	app, _, member := newAppWithRuns(t, runner, fakeRunReader{}, &memLimit{value: 30})
	code, body := do(t, app, "POST", "/api/v1/bulk-runs", member, runBody(oneText, `["a","b","a","gone"]`, true))
	if code != 201 || runner.calls != 1 {
		t.Fatalf("started: %d %v", code, body)
	}
	if len(runner.targets) != 3 || runner.targets[0].KocID != "a" || runner.targets[0].Handle != "@ua" || runner.targets[2].KocID != "gone" || runner.targets[2].KocName != "" {
		t.Fatalf("a repeated id once, an unknown id stays in the run: %+v", runner.targets)
	}
	data, _ := body["data"].(map[string]any)
	if data["estimateMinutes"] != float64(3) {
		t.Fatalf("estimate: %v", data)
	}
}

func TestARunIsRefusedWithAConflictWhenTheSwitchIsOffOrOneIsRunning(t *testing.T) {
	for _, err := range []error{service.ErrBulkOff, service.ErrRunInProgress} {
		runner := &fakeRunner{err: err}
		app, _, member := newAppWithRuns(t, runner, fakeRunReader{}, &memLimit{value: 30})
		if code, _ := do(t, app, "POST", "/api/v1/bulk-runs", member, runBody(oneText, `["a"]`, true)); code != 409 {
			t.Fatalf("%v must be a 409: %d", err, code)
		}
	}
}

func TestTheDailyLimitIsReadByMembersAndChangedOnlyByAnAdminToAnOfferedValue(t *testing.T) {
	app, admin, member := newApp(t)
	_, body := do(t, app, "GET", "/api/v1/bulk-limit", member, "")
	if data, _ := body["data"].(map[string]any); data["value"] != float64(30) {
		t.Fatalf("30 by default: %v", body)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/bulk-limit", member, `{"value":50}`); code != 403 {
		t.Fatalf("a member cannot change it: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/bulk-limit", admin, `{"value":25}`); code != 400 {
		t.Fatalf("25 is not offered: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/bulk-limit", admin, `{"value":50}`); code != 200 {
		t.Fatalf("an admin sets 50: %d", code)
	}
	_, body = do(t, app, "GET", "/api/v1/bulk-limit", member, "")
	if data, _ := body["data"].(map[string]any); data["value"] != float64(50) {
		t.Fatalf("50 is kept: %v", body)
	}
}

func TestTheCurrentRunIsNullWhenNothingRuns(t *testing.T) {
	app, _, member := newApp(t)
	code, body := do(t, app, "GET", "/api/v1/bulk-runs/current", member, "")
	data, _ := body["data"].(map[string]any)
	if code != 200 || data == nil || data["run"] != nil {
		t.Fatalf("run: null: %d %v", code, body)
	}
}

func TestTheOldBatchRouteIsGone(t *testing.T) {
	app, _, member := newApp(t)
	if code, _ := do(t, app, "POST", "/api/v1/kocs/message-batch", member, `{}`); code != 404 && code != 405 { // "/message-batch" now falls under "/:id", which only answers GET
		t.Fatalf("the batch-of-five route is removed: %d", code)
	}
}

func TestTheBulkSwitchIsReadByMembersChangedOnlyByAnAdminAndOffByDefault(t *testing.T) {
	app, admin, member := newApp(t)

	if code, _ := do(t, app, "GET", "/api/v1/bulk-switch", "", ""); code != 401 {
		t.Fatalf("the switch is not public: %d", code)
	}
	code, body := do(t, app, "GET", "/api/v1/bulk-switch", member, "")
	if data, _ := body["data"].(map[string]any); code != 200 || data["enabled"] != false {
		t.Fatalf("a member sees it and it starts off: %d %v", code, body)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/bulk-switch", member, `{"enabled":true,"confirm":true}`); code != 403 {
		t.Fatalf("a member cannot turn it on: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/bulk-switch", admin, `{"enabled":true}`); code != 400 {
		t.Fatalf("turning on needs a confirmation: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/bulk-switch", admin, `{"enabled":true,"confirm":true}`); code != 200 {
		t.Fatalf("an admin turns it on: %d", code)
	}
	_, body = do(t, app, "GET", "/api/v1/automation", member, "")
	if data, _ := body["data"].(map[string]any); data["enabled"] != false {
		t.Fatalf("the master switch of the chatbot is untouched: %v", body)
	}
}
