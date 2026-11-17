package router_test

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// fakeImportStarter records what the handler hands to the worker and answers with a canned error.
type fakeImportStarter struct {
	mu     sync.Mutex
	calls  int
	by     string
	source string
	rows   []domain.ImportRow
	err    error
}

func (f *fakeImportStarter) Start(_ context.Context, by, source string, rows []domain.ImportRow) (*domain.ImportJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	f.by, f.source, f.rows = by, source, rows
	return &domain.ImportJob{ID: domain.NewID(), Source: source, Rows: rows, Status: domain.ImportRunning, By: by}, nil
}

// memImportJobs is a job store with at most one job.
type memImportJobs struct {
	job      *domain.ImportJob
	stopFor  string
	stopSeen bool
}

func (m *memImportJobs) Running(context.Context) (*domain.ImportJob, error) { return m.job, nil }
func (m *memImportJobs) Find(_ context.Context, id string) (*domain.ImportJob, error) {
	if m.job == nil || m.job.ID.Hex() != id {
		return nil, domain.NotFound("Không tìm thấy đợt nhập.")
	}
	return m.job, nil
}
func (m *memImportJobs) RequestStop(_ context.Context, id string) (bool, error) {
	if m.job == nil || m.job.ID.Hex() != id || m.job.Status != domain.ImportRunning {
		return false, nil
	}
	m.stopSeen = true
	return true, nil
}

func importBody(items string) string { return `{"items":` + items + `}` }

const twoRows = `[{"tiktokId":"lan.beauty","email":"lan@x.vn"},{"tiktokId":"minh.rv","line":9}]`

func TestImportJobRoutesNeedALogin(t *testing.T) {
	app, _, _ := newApp(t)
	for _, r := range [][2]string{{"POST", "/api/v1/import-jobs"}, {"GET", "/api/v1/import-jobs/current"}, {"GET", "/api/v1/import-jobs/x"}, {"PUT", "/api/v1/import-jobs/x/stop"}} {
		if code, _ := do(t, app, r[0], r[1], "", `{}`); code != 401 {
			t.Errorf("%s %s needs a login: %d", r[0], r[1], code)
		}
	}
}

func TestStartingAnImportAnswers202WithTheJobAndHandsTheRowsToTheWorker(t *testing.T) {
	starter := &fakeImportStarter{}
	app, _, member := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.ImportJob = handler.NewImportJobHandler(starter, &memImportJobs{}, service.NewAuditService(nil))
	})
	code, body := do(t, app, "POST", "/api/v1/import-jobs", member, importBody(twoRows))
	data, _ := body["data"].(map[string]any)
	if code != 202 || data["_id"] == nil || data["status"] != "running" {
		t.Fatalf("202 with the job: %d %v", code, body)
	}
	if starter.calls != 1 || len(starter.rows) != 2 || starter.rows[0].Line != 2 || starter.rows[1].Line != 9 || starter.rows[0].Email != "lan@x.vn" || starter.source != "TikToday" {
		t.Fatalf("the worker got the rows, with the sheet line (the header is row 1) and the default source: %+v %q", starter.rows, starter.source)
	}
}

func TestTheRowsAreCheckedBeforeAnyJobIsMade(t *testing.T) {
	starter := &fakeImportStarter{}
	app, _, member := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.ImportJob = handler.NewImportJobHandler(starter, &memImportJobs{}, service.NewAuditService(nil))
	})
	many := "[" + strings.Repeat(`{"tiktokId":"abc"},`, 200) + `{"tiktokId":"abc"}]` // 201 rows
	bad := map[string]string{
		"no rows":         importBody(`[]`),
		"too many rows":   importBody(many),
		"no TikTok id":    importBody(`[{"tiktokId":""}]`),
		"a bad email":     importBody(`[{"tiktokId":"abc","email":"not-an-email"}]`),
		"a negative fee":  importBody(`[{"tiktokId":"abc","castFee":-5}]`),
		"a bad source":    `{"source":"Facebook","items":[{"tiktokId":"abc"}]}`,
		"a row line of 0": importBody(`[{"tiktokId":"abc","line":-1}]`),
	}
	for name, b := range bad {
		if code, _ := do(t, app, "POST", "/api/v1/import-jobs", member, b); code != 400 {
			t.Errorf("%s must be refused with 400, got %d", name, code)
		}
	}
	if starter.calls != 0 {
		t.Fatalf("no job is made for bad data: %d", starter.calls)
	}
}

func TestAnImportIsRefusedWithAConflictWhenOneIsRunning(t *testing.T) {
	starter := &fakeImportStarter{err: service.ErrImportInProgress}
	app, _, member := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.ImportJob = handler.NewImportJobHandler(starter, &memImportJobs{}, service.NewAuditService(nil))
	})
	if code, _ := do(t, app, "POST", "/api/v1/import-jobs", member, importBody(twoRows)); code != 409 {
		t.Fatalf("one at a time: %d", code)
	}
}

func TestTheCurrentJobIsNullWhenNothingRunsAndTheJobWhenSomethingDoes(t *testing.T) {
	jobs := &memImportJobs{}
	app, _, member := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.ImportJob = handler.NewImportJobHandler(&fakeImportStarter{}, jobs, service.NewAuditService(nil))
	})
	_, body := do(t, app, "GET", "/api/v1/import-jobs/current", member, "")
	if data, _ := body["data"].(map[string]any); data == nil || data["job"] != nil {
		t.Fatalf("job: null: %v", body)
	}
	jobs.job = &domain.ImportJob{ID: domain.NewID(), Status: domain.ImportRunning, Rows: []domain.ImportRow{{Line: 2, TiktokID: "a", Status: domain.RowAdded}}}
	_, body = do(t, app, "GET", "/api/v1/import-jobs/current", member, "")
	data, _ := body["data"].(map[string]any)
	job, _ := data["job"].(map[string]any)
	if job["status"] != "running" || job["rows"] == nil {
		t.Fatalf("the running job with its rows: %v", body)
	}
}

func TestAJobIsReadByIdAndAnUnknownOneIsNotFound(t *testing.T) {
	jobs := &memImportJobs{job: &domain.ImportJob{ID: domain.NewID(), Status: domain.ImportDone}}
	app, _, member := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.ImportJob = handler.NewImportJobHandler(&fakeImportStarter{}, jobs, service.NewAuditService(nil))
	})
	if code, _ := do(t, app, "GET", "/api/v1/import-jobs/"+jobs.job.ID.Hex(), member, ""); code != 200 {
		t.Fatalf("read: %d", code)
	}
	if code, _ := do(t, app, "GET", "/api/v1/import-jobs/"+domain.NewID().Hex(), member, ""); code != 404 {
		t.Fatalf("unknown: %d", code)
	}
}

func TestStoppingARunningJobIsAskedForAndAnEndedOneIsAConflict(t *testing.T) {
	jobs := &memImportJobs{job: &domain.ImportJob{ID: domain.NewID(), Status: domain.ImportRunning}}
	app, _, member := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.ImportJob = handler.NewImportJobHandler(&fakeImportStarter{}, jobs, service.NewAuditService(nil))
	})
	path := "/api/v1/import-jobs/" + jobs.job.ID.Hex() + "/stop"
	if code, _ := do(t, app, "PUT", path, member, ""); code != 200 || !jobs.stopSeen {
		t.Fatalf("stop: %d seen=%v", code, jobs.stopSeen)
	}
	jobs.job.Status = domain.ImportDone
	if code, _ := do(t, app, "PUT", path, member, ""); code != 409 {
		t.Fatalf("a job that ended: %d", code)
	}
}

func TestTheStartOfAnImportIsRateLimited(t *testing.T) {
	app, _, member := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.ImportJob = handler.NewImportJobHandler(&fakeImportStarter{}, &memImportJobs{}, service.NewAuditService(nil))
	})
	limited := false
	for i := 0; i < 25; i++ {
		if code, _ := do(t, app, "POST", "/api/v1/import-jobs", member, importBody(twoRows)); code == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("starting an import is limited to 20 an hour: " + strconv.Itoa(25) + " calls were not refused")
	}
}

func TestEveryFieldOfARowReachesTheWorker(t *testing.T) {
	starter := &fakeImportStarter{}
	app, _, member := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.ImportJob = handler.NewImportJobHandler(starter, &memImportJobs{}, service.NewAuditService(nil))
	})
	body := `{"source":"TikTok","items":[{"tiktokId":"abc","email":"a@x.vn","recipient":"Lan","shippingAddress":"1 A, Hà Nội","phone":"0976451571","castFee":500000,"category":"Làm đẹp"}]}`
	if code, _ := do(t, app, "POST", "/api/v1/import-jobs", member, body); code != 202 || len(starter.rows) != 1 {
		t.Fatalf("one row goes through: %d %v", code, starter.rows)
	}
	r := starter.rows[0]
	if r.Email != "a@x.vn" || r.Recipient != "Lan" || r.ShippingAddress != "1 A, Hà Nội" || r.Phone != "0976451571" || r.Category != "Làm đẹp" || r.CastFee == nil || *r.CastFee != 500000 || starter.source != "TikTok" {
		t.Fatalf("the worker gets every field of the row: %+v source=%q", r, starter.source)
	}
}
