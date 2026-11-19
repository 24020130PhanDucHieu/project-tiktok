package router_test

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/service"
)

type wfKocs struct{ list []*domain.Koc }

func (w *wfKocs) FindByIDs(_ context.Context, ids []domain.ID) ([]*domain.Koc, error) {
	var out []*domain.Koc
	for _, k := range w.list {
		for _, id := range ids {
			if k.ID == id {
				out = append(out, k)
			}
		}
	}
	return out, nil
}

func TestTheReportOfAWorkflowComesWithTheNamesOfItsKocsAndNothingElseAboutThem(t *testing.T) {
	k1 := &domain.Koc{ID: domain.NewID(), TiktokID: "lan.beauty", DisplayName: "Lan Beauty", Email: "lan@x.vn", Phone: "0900000001"}
	k2 := &domain.Koc{ID: domain.NewID(), TiktokID: "minh.review"}
	wf := &domain.Workflow{
		ID: domain.NewID(), Name: "Thử", Status: domain.WorkflowRunning,
		Log: []domain.WorkflowEvent{{At: time.Now(), NodeID: "s", Kind: domain.EventDid, Kocs: []string{k1.ID.Hex(), k2.ID.Hex(), "not-an-id"}}},
	}
	store := &memWorkflowStore{wf: wf}
	app, admin, _ := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.Workflow = handler.NewWorkflowHandler(&fakeWorkflowStarter{}, store, newWfCampaigns(), service.NewAuditService(nil)).WithKocs(&wfKocs{list: []*domain.Koc{k1, k2}})
	})
	code, body := do(t, app, "GET", "/api/v1/workflows/"+wf.ID.Hex(), admin, "")
	data, _ := body["data"].(map[string]any)
	names, _ := data["kocNames"].(map[string]any)
	if code != 200 || names[k1.ID.Hex()] != "Lan Beauty" || names[k2.ID.Hex()] != "@minh.review" {
		t.Fatalf("names by id, the TikTok handle when a KOC has no display name: %d %v", code, data["kocNames"])
	}
	if len(names) != 2 {
		t.Fatalf("only real KOCs are named: %v", names)
	}
	if s := body; s != nil {
		for _, secret := range []string{"lan@x.vn", "0900000001"} {
			if containsText(body, secret) {
				t.Fatalf("the report carries names only, never %s", secret)
			}
		}
	}
}

func containsText(v any, needle string) bool {
	switch x := v.(type) {
	case string:
		return x == needle
	case map[string]any:
		for _, e := range x {
			if containsText(e, needle) {
				return true
			}
		}
	case []any:
		for _, e := range x {
			if containsText(e, needle) {
				return true
			}
		}
	}
	return false
}
