package router_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// fakeWorkflowStarter records what the handler hands to the runner and answers with a canned error.
type fakeWorkflowStarter struct {
	mu    sync.Mutex
	calls int
	by    string
	wf    *domain.Workflow
	err   error
}

func (f *fakeWorkflowStarter) Start(_ context.Context, by string, wf *domain.Workflow) (*domain.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	f.by, f.wf = by, wf
	cp := *wf
	cp.ID, cp.Status = domain.NewID(), domain.WorkflowRunning
	return &cp, nil
}

// memWorkflows is a store with at most one workflow.
type memWorkflowStore struct {
	wf       *domain.Workflow
	stopSeen bool
}

func (m *memWorkflowStore) Latest(context.Context) (*domain.Workflow, error) { return m.wf, nil }
func (m *memWorkflowStore) Find(_ context.Context, id string) (*domain.Workflow, error) {
	if m.wf == nil || m.wf.ID.Hex() != id {
		return nil, domain.NotFound("Không tìm thấy quy trình.")
	}
	return m.wf, nil
}
func (m *memWorkflowStore) RequestStop(_ context.Context, id string) (bool, error) {
	if m.wf == nil || m.wf.ID.Hex() != id || m.wf.Status != domain.WorkflowRunning {
		return false, nil
	}
	m.stopSeen = true
	return true, nil
}

// wfCampaigns holds the saved campaigns the send blocks may choose.
type wfCampaigns struct{ byID map[string]*domain.Campaign }

func (w *wfCampaigns) Find(_ context.Context, id string) (*domain.Campaign, error) {
	if c := w.byID[id]; c != nil {
		return c, nil
	}
	return nil, domain.NotFound("Không tìm thấy chiến dịch.")
}
func (w *wfCampaigns) ImagesExist(context.Context, []domain.ID) (bool, error) { return true, nil }

func newWfCampaigns() *wfCampaigns {
	return &wfCampaigns{byID: map[string]*domain.Campaign{
		"c1": {ID: domain.NewID(), Name: "Mời hợp tác", Steps: []domain.CampaignStep{{Kind: domain.StepText, Text: "Chào {ten}"}, {Kind: domain.StepText, Text: "Anh chị xem giúp em nhé"}}},
		"c0": {ID: domain.NewID(), Name: "Trống"},
	}}
}

const (
	importNode   = `{"id":"i","type":"import","x":0,"y":0}`
	waitNode     = `{"id":"w","type":"wait","x":100,"y":0,"waitMinutes":60}`
	sendNode     = `{"id":"s","type":"send","x":200,"y":0,"campaignId":"c1"}`
	oneRow       = `[{"tiktokId":"lan.beauty","email":"lan@x.vn"}]`
	importToSend = `[{"id":"e1","from":"i","port":"out","to":"s"}]`
	importToWait = `[{"id":"e1","from":"i","port":"out","to":"w"}]`
)

func workflowBody(confirm bool, nodes, edges, items string) string {
	c := "false"
	if confirm {
		c = "true"
	}
	return `{"name":"Mời rồi nhắc","confirm":` + c + `,"nodes":[` + nodes + `],"edges":` + edges + `,"items":` + items + `}`
}

func newWorkflowApp(t *testing.T, starter *fakeWorkflowStarter, store *memWorkflowStore) (app *fiber.App, admin, member string) {
	t.Helper()
	return newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.Workflow = handler.NewWorkflowHandler(starter, store, newWfCampaigns(), service.NewAuditService(nil))
	})
}

func TestWorkflowRoutesNeedALogin(t *testing.T) {
	app, _, _ := newWorkflowApp(t, &fakeWorkflowStarter{}, &memWorkflowStore{})
	for _, r := range [][2]string{{"POST", "/api/v1/workflows"}, {"GET", "/api/v1/workflows/latest"}, {"GET", "/api/v1/workflows/x"}, {"PUT", "/api/v1/workflows/x/stop"}} {
		if code, _ := do(t, app, r[0], r[1], "", `{}`); code != 401 {
			t.Errorf("%s %s needs a login: %d", r[0], r[1], code)
		}
	}
}

func TestStartingAWorkflowNeedsTheConfirmation(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	if code, _ := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(false, importNode+","+sendNode, importToSend, oneRow)); code != 400 {
		t.Fatalf("a workflow that sends needs confirm=true: %d", code)
	}
	if starter.calls != 0 {
		t.Fatal("nothing was started")
	}
}

func TestAnAdminStartsAWorkflowAndGetsA202WithoutTheSheet(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	code, body := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+sendNode, importToSend, oneRow))
	data, _ := body["data"].(map[string]any)
	if code != 202 || data["_id"] == nil || data["status"] != "running" {
		t.Fatalf("202 with the workflow: %d %v", code, body)
	}
	if _, has := data["rows"]; has {
		t.Fatalf("the answer must not carry the rows of the sheet (phones, emails, addresses): %v", data["rows"])
	}
	if starter.calls != 1 || len(starter.wf.Rows) != 1 || starter.wf.Rows[0].Email != "lan@x.vn" || starter.wf.Rows[0].Line != 2 || starter.wf.Source != "TikToday" {
		t.Fatalf("the runner got the rows with their sheet line and the default source: %+v", starter.wf)
	}
}

func TestOnlyAnAdminStartsAWorkflowThatSends(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, _, member := newWorkflowApp(t, starter, &memWorkflowStore{})
	if code, _ := do(t, app, "POST", "/api/v1/workflows", member, workflowBody(true, importNode+","+sendNode, importToSend, oneRow)); code != 403 {
		t.Fatalf("a member may not start a workflow with a send block: %d", code)
	}
	if starter.calls != 0 {
		t.Fatal("nothing was started")
	}
	if code, _ := do(t, app, "POST", "/api/v1/workflows", member, workflowBody(true, importNode+","+waitNode, importToWait, oneRow)); code != 202 {
		t.Fatalf("a workflow that sends nothing is open to a member: %d", code)
	}
}

func TestAnInvalidGraphIsRefusedBeforeAnythingIsStarted(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	two := `[{"id":"e1","from":"i","port":"out","to":"s"},{"id":"e2","from":"i","port":"out","to":"w"}]`
	bad := map[string]string{
		"no import block":  workflowBody(true, sendNode, `[]`, oneRow),
		"two wires out":    workflowBody(true, importNode+","+sendNode+","+waitNode, two, oneRow),
		"an unknown block": workflowBody(true, importNode+`,{"id":"x","type":"email"}`, `[{"id":"e1","from":"i","port":"out","to":"x"}]`, oneRow),
		"no blocks":        `{"confirm":true,"nodes":[],"edges":[],"items":` + oneRow + `}`,
		"a loose block":    workflowBody(true, importNode+","+waitNode, `[]`, oneRow),
	}
	for name, b := range bad {
		if code, _ := do(t, app, "POST", "/api/v1/workflows", admin, b); code != 400 {
			t.Errorf("%s must be refused with 400, got %d", name, code)
		}
	}
	if starter.calls != 0 {
		t.Fatalf("no workflow is made for a bad graph: %d", starter.calls)
	}
}

func TestTheStepsOfTheCampaignAreCopiedIntoTheSendBlock(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	cheat := `{"id":"s","type":"send","campaignId":"c1","campaignName":"Của tôi","steps":[{"kind":"text","text":"tin do trinh duyet tu viet"}]}`
	if code, body := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+cheat, importToSend, oneRow)); code != 202 {
		t.Fatalf("%d %v", code, body)
	}
	var send domain.WorkflowNode
	for _, n := range starter.wf.Nodes {
		if n.Type == domain.NodeSend {
			send = n
		}
	}
	if send.CampaignName != "Mời hợp tác" || len(send.Steps) != 2 || send.Steps[0].Text != "Chào {ten}" {
		t.Fatalf("the steps come from the saved campaign at this moment, never from the browser: %+v", send)
	}
}

func TestACampaignThatIsMissingOrEmptyIsRefused(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	for name, id := range map[string]string{"missing": "nope", "without steps": "c0"} {
		node := `{"id":"s","type":"send","campaignId":"` + id + `"}`
		if code, _ := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+node, importToSend, oneRow)); code != 400 {
			t.Errorf("a %s campaign must be refused with 400, got %d", name, code)
		}
	}
	if starter.calls != 0 {
		t.Fatal("nothing was started")
	}
}

func TestRowsThatBreakARuleAreKeptAsInvalidAndTheOthersGoOn(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	rows := `[{"tiktokId":"lan.beauty"},{"tiktokId":"minh.rv","email":"not-an-email"}]`
	if code, body := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+waitNode, importToWait, rows)); code != 202 {
		t.Fatalf("%d %v", code, body)
	}
	got := starter.wf.Rows
	if len(got) != 2 || got[0].Status == domain.RowInvalid || got[1].Status != domain.RowInvalid || got[1].Reason == "" {
		t.Fatalf("the bad row is kept with its reason: %+v", got)
	}
	allBad := `[{"tiktokId":"x","email":"nope"}]`
	if code, _ := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+waitNode, importToWait, allBad)); code != 400 {
		t.Fatalf("a sheet where every row is refused is refused: %d", code)
	}
	many := "[" + strings.Repeat(`{"tiktokId":"abc"},`, 200) + `{"tiktokId":"abc"}]` // 201 rows
	if code, _ := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+waitNode, importToWait, many)); code != 400 {
		t.Fatalf("more than 200 rows is refused: %d", code)
	}
}

func TestOneWorkflowAtATimeAnswersConflict(t *testing.T) {
	starter := &fakeWorkflowStarter{err: service.ErrWorkflowInProgress}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	if code, _ := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+waitNode, importToWait, oneRow)); code != 409 {
		t.Fatalf("one at a time: %d", code)
	}
}

func TestReadingAWorkflowHidesTheRowsOfTheSheet(t *testing.T) {
	wf := &domain.Workflow{ID: domain.NewID(), Name: "w", Status: domain.WorkflowRunning,
		Rows:   []domain.ImportRow{{Line: 2, TiktokID: "lan", Phone: "0976451571", ShippingAddress: "12 Lê Lợi"}},
		States: []domain.NodeState{{NodeID: "i", Status: domain.NodeRunning, Count: 1}}}
	app, _, member := newWorkflowApp(t, &fakeWorkflowStarter{}, &memWorkflowStore{wf: wf})
	code, body := do(t, app, "GET", "/api/v1/workflows/latest", member, ``)
	data, _ := body["data"].(map[string]any)
	w, _ := data["workflow"].(map[string]any)
	if code != 200 || w["_id"] != wf.ID.Hex() || w["states"] == nil {
		t.Fatalf("latest shows the workflow and its states: %d %v", code, body)
	}
	if _, has := w["rows"]; has {
		t.Fatalf("no phone or address of the sheet goes to the browser: %v", w["rows"])
	}
	code, body = do(t, app, "GET", "/api/v1/workflows/"+wf.ID.Hex(), member, ``)
	one, _ := body["data"].(map[string]any)
	if code != 200 || one["_id"] != wf.ID.Hex() || one["rows"] != nil {
		t.Fatalf("by id: %d %v", code, body)
	}
	if code, _ := do(t, app, "GET", "/api/v1/workflows/"+domain.NewID().Hex(), member, ``); code != 404 {
		t.Fatalf("an unknown workflow is 404: %d", code)
	}
}

func TestLatestIsNullWhenThereIsNoWorkflow(t *testing.T) {
	app, _, member := newWorkflowApp(t, &fakeWorkflowStarter{}, &memWorkflowStore{})
	code, body := do(t, app, "GET", "/api/v1/workflows/latest", member, ``)
	data, _ := body["data"].(map[string]any)
	if code != 200 || data["workflow"] != nil {
		t.Fatalf("%d %v", code, body)
	}
}

func TestStoppingNeedsAnAdminAndRefusesAnEndedWorkflow(t *testing.T) {
	wf := &domain.Workflow{ID: domain.NewID(), Status: domain.WorkflowRunning}
	store := &memWorkflowStore{wf: wf}
	app, admin, member := newWorkflowApp(t, &fakeWorkflowStarter{}, store)
	path := "/api/v1/workflows/" + wf.ID.Hex() + "/stop"
	if code, _ := do(t, app, "PUT", path, member, ``); code != 403 {
		t.Fatalf("a member may not stop it: %d", code)
	}
	if code, _ := do(t, app, "PUT", path, admin, ``); code != 200 || !store.stopSeen {
		t.Fatalf("an admin stops it: %d seen=%v", code, store.stopSeen)
	}
	wf.Status = domain.WorkflowDone
	if code, _ := do(t, app, "PUT", path, admin, ``); code != 409 {
		t.Fatalf("nothing to stop in an ended workflow: %d", code)
	}
}

func TestStartingWorkflowsIsRateLimited(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	body := workflowBody(true, importNode+","+waitNode, importToWait, oneRow)
	for i := 1; i <= 10; i++ {
		if code, _ := do(t, app, "POST", "/api/v1/workflows", admin, body); code != 202 {
			t.Fatalf("start %d: %d", i, code)
		}
	}
	if code, _ := do(t, app, "POST", "/api/v1/workflows", admin, body); code != 429 {
		t.Fatalf("the eleventh start in an hour is refused: %d", code)
	}
}
