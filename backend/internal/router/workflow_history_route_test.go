package router_test

import (
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestTheHistoryListsPastRunsNewestFirstWithoutTheSheetsOrTheReports(t *testing.T) {
	old := &domain.Workflow{ID: domain.NewID(), Name: "Cũ", Status: domain.WorkflowDone, CreatedAt: time.Now().Add(-48 * time.Hour), Rows: []domain.ImportRow{{Email: "lan@x.vn"}}}
	recent := &domain.Workflow{ID: domain.NewID(), Name: "Mới", Status: domain.WorkflowRunning, CreatedAt: time.Now(), Log: []domain.WorkflowEvent{{NodeID: "s", Kind: domain.EventDid, Kocs: []string{"k"}}}}
	store := &memWorkflowStore{all: []*domain.Workflow{old, recent}}
	app, _, member := newWorkflowApp(t, &fakeWorkflowStarter{}, store)

	code, body := do(t, app, "GET", "/api/v1/workflows", member, "")
	data, _ := body["data"].(map[string]any)
	list, _ := data["workflows"].([]any)
	if code != 200 || len(list) != 2 {
		t.Fatalf("a member may read the history: %d %v", code, body)
	}
	if first, _ := list[0].(map[string]any); first["name"] != "Mới" {
		t.Fatalf("newest first: %v", first["name"])
	}
	for _, item := range list {
		m := item.(map[string]any)
		if _, has := m["rows"]; has {
			t.Fatalf("the list must not carry the sheet: %v", m)
		}
		if _, has := m["log"]; has {
			t.Fatalf("the list is a summary; the report is read one run at a time: %v", m)
		}
	}
}

func TestTheHistoryNeedsALogin(t *testing.T) {
	app, _, _ := newWorkflowApp(t, &fakeWorkflowStarter{}, &memWorkflowStore{})
	if code, _ := do(t, app, "GET", "/api/v1/workflows", "", ""); code != 401 {
		t.Fatalf("login needed: %d", code)
	}
}
