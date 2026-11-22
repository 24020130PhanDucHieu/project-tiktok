package router_test

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// accountChecker says which Zalo accounts exist for a workflow block, and notes what it was asked.
type accountChecker struct {
	ok    map[string]bool // account id -> may be used
	asked []string
}

func (a *accountChecker) ResolveAccount(_ context.Context, id string) (string, error) {
	a.asked = append(a.asked, id)
	if id == "" {
		return "", nil
	}
	if !a.ok[id] {
		return "", domain.Invalid("Không tìm thấy tài khoản Zalo để gửi.")
	}
	return id, nil
}

func newAccountWorkflowApp(t *testing.T, starter *fakeWorkflowStarter, checker handler.WorkflowAccounts) (*fiber.App, string) {
	t.Helper()
	app, admin, _ := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.Workflow = handler.NewWorkflowHandler(starter, &memWorkflowStore{}, newWfCampaigns(), service.NewAuditService(nil)).WithAccounts(checker)
	})
	return app, admin
}

const acctB = "bbbbbbbbbbbbbbbbbbbbbbbb"

func TestASendBlockCarriesTheAccountItWasGiven(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin := newAccountWorkflowApp(t, starter, &accountChecker{ok: map[string]bool{acctB: true}})
	send := `{"id":"s","type":"send","x":200,"y":0,"campaignId":"c1","zaloAccountId":"` + acctB + `"}`
	if code, body := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+send, importToSend, oneRow)); code != 202 {
		t.Fatalf("%d %v", code, body)
	}
	var got string
	for _, n := range starter.wf.Nodes {
		if n.ID == "s" {
			got = n.ZaloAccountID
		}
	}
	if got != acctB {
		t.Fatalf("the send block reached the runner with account %q, want %s", got, acctB)
	}
}

func TestASendTextBlockCarriesItsAccountToo(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin := newAccountWorkflowApp(t, starter, &accountChecker{ok: map[string]bool{acctB: true}})
	text := `{"id":"t","type":"send_text","x":200,"y":0,"text":"Chào {ten}","zaloAccountId":"` + acctB + `"}`
	edges := `[{"id":"e1","from":"i","port":"out","to":"t"}]`
	if code, body := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+text, edges, oneRow)); code != 202 {
		t.Fatalf("%d %v", code, body)
	}
	if n := starter.wf.Nodes[len(starter.wf.Nodes)-1]; n.ID != "t" || n.ZaloAccountID != acctB {
		t.Fatalf("send_text block: %+v", n)
	}
}

func TestAnAccountThatCannotBeUsedStopsTheWorkflowBeforeItStarts(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin := newAccountWorkflowApp(t, starter, &accountChecker{ok: map[string]bool{}})
	send := `{"id":"s","type":"send","x":200,"y":0,"campaignId":"c1","zaloAccountId":"` + acctB + `"}`
	code, body := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+send, importToSend, oneRow))
	if code != 400 {
		t.Fatalf("%d %v, want 400", code, body)
	}
	if starter.calls != 0 {
		t.Fatal("a workflow with an unusable account was started")
	}
}

func TestOnlyASendBlockIsAskedAboutAnAccount(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	checker := &accountChecker{ok: map[string]bool{}}
	app, admin := newAccountWorkflowApp(t, starter, checker)
	// a wait block that tries to carry an account: the block is cleaned, so nobody is asked and nothing is kept
	wait := `{"id":"w","type":"wait","x":100,"y":0,"waitMinutes":60,"zaloAccountId":"` + acctB + `"}`
	if code, body := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+wait, importToWait, oneRow)); code != 202 {
		t.Fatalf("%d %v", code, body)
	}
	if len(checker.asked) != 0 {
		t.Fatalf("asked about %v for a block that sends nothing", checker.asked)
	}
	for _, n := range starter.wf.Nodes {
		if n.ZaloAccountID != "" {
			t.Fatalf("block %s kept an account: %+v", n.ID, n)
		}
	}
}

func TestWithoutAnAccountCheckerAWorkflowStartsAsBefore(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	send := `{"id":"s","type":"send","x":200,"y":0,"campaignId":"c1","zaloAccountId":"` + acctB + `"}`
	if code, body := do(t, app, "POST", "/api/v1/workflows", admin, workflowBody(true, importNode+","+send, importToSend, oneRow)); code != 202 {
		t.Fatalf("%d %v", code, body)
	}
}
