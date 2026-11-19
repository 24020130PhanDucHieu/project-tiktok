package router_test

import (
	"context"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/service"
)

type wfPicker struct {
	got  domain.PickSpec
	list []domain.TokenKoc
}

func (w *wfPicker) PickBy(_ context.Context, spec domain.PickSpec) ([]domain.TokenKoc, error) {
	w.got = spec
	return w.list, nil
}

func previewApp(t *testing.T, picker *wfPicker, kocs ...*domain.Koc) (app interface{}, member string, run func(method, path, token, body string) (int, map[string]any)) {
	t.Helper()
	a, _, m := newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.Workflow = handler.NewWorkflowHandler(&fakeWorkflowStarter{}, &memWorkflowStore{}, newWfCampaigns(), service.NewAuditService(nil)).
			WithKocs(&wfKocs{list: kocs}).WithPicker(picker)
	})
	return a, m, func(method, path, token, body string) (int, map[string]any) { return do(t, a, method, path, token, body) }
}

const pickNodeJSON = `{"id":"p","type":"pick","x":0,"y":0,"stages":["DORMANT"],"limit":5}`

func TestThePreviewNamesTheKocsAPickBlockWouldTakeAndAnyoneSignedInMayAskForIt(t *testing.T) {
	k1 := &domain.Koc{ID: domain.NewID(), TiktokID: "xuka", DisplayName: "Thơm Xuka review"}
	k2 := &domain.Koc{ID: domain.NewID(), TiktokID: "lan.beauty"}
	picker := &wfPicker{list: []domain.TokenKoc{{KocID: k1.ID.Hex()}, {KocID: k2.ID.Hex()}}}
	_, member, run := previewApp(t, picker, k1, k2)

	code, body := run("POST", "/api/v1/workflows/preview", member, `{"nodes":[`+pickNodeJSON+`],"edges":[]}`)
	data, _ := body["data"].(map[string]any)
	list, _ := data["kocs"].([]any)
	if code != 200 || data["total"] != float64(2) || len(list) != 2 {
		t.Fatalf("a member may preview: %d %v", code, body)
	}
	first, _ := list[0].(map[string]any)
	second, _ := list[1].(map[string]any)
	if first["name"] != "Thơm Xuka review" || second["name"] != "@lan.beauty" {
		t.Fatalf("names in the order the block would take them: %v %v", first, second)
	}
	if len(picker.got.Stages) != 1 || picker.got.Stages[0] != "DORMANT" || picker.got.Limit != 5 {
		t.Fatalf("the preview asks the picker what the block says: %+v", picker.got)
	}
}

func TestThePreviewRefusesAnInvalidGraphAndAnImportStartAndNeedsALogin(t *testing.T) {
	_, member, run := previewApp(t, &wfPicker{})
	if code, _ := run("POST", "/api/v1/workflows/preview", "", `{"nodes":[`+pickNodeJSON+`],"edges":[]}`); code != 401 {
		t.Fatalf("login needed: %d", code)
	}
	if code, _ := run("POST", "/api/v1/workflows/preview", member, `{"nodes":[{"id":"p","type":"pick","x":0,"y":0,"limit":5}],"edges":[]}`); code != 400 {
		t.Fatalf("a pick that picks nothing is refused: %d", code)
	}
	if code, _ := run("POST", "/api/v1/workflows/preview", member, `{"nodes":[`+importNode+`],"edges":[]}`); code != 400 {
		t.Fatalf("an import start has no directory list to preview: %d", code)
	}
}
