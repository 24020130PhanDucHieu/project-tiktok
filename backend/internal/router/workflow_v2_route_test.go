package router_test

import (
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

const (
	pickNode     = `{"id":"p","type":"pick","x":0,"y":0,"stages":["SHIPPING"],"limit":20}`
	labelNode    = `{"id":"l","type":"label","labelId":"65a1b2c3d4e5f60718293a4b"}`
	stageNode    = `{"id":"g","type":"stage","stage":"NO_REPLY"}`
	sendTextNode = `{"id":"t","type":"send_text","text":"Chào {ten}"}`
	emailNode    = `{"id":"m","type":"email","subject":"Hợp tác","text":"Chào {ten}"}`
	untilNode    = `{"id":"u","type":"wait_until","atMinute":540}`
)

func wire(ids ...string) string {
	out := "["
	for i := 1; i < len(ids); i++ {
		if i > 1 {
			out += ","
		}
		out += `{"id":"e` + ids[i-1] + ids[i] + `","from":"` + ids[i-1] + `","port":"out","to":"` + ids[i] + `"}`
	}
	return out + "]"
}

func bodyNoItems(nodes, edges string) string {
	return `{"confirm":true,"nodes":[` + nodes + `],"edges":` + edges + `}`
}

func TestAWorkflowThatStartsFromPickNeedsNoSheet(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, _, member := newWorkflowApp(t, starter, &memWorkflowStore{})
	code, body := do(t, app, "POST", "/api/v1/workflows", member, bodyNoItems(pickNode+","+labelNode+","+stageNode, wire("p", "l", "g")))
	if code != 202 {
		t.Fatalf("%d %v", code, body)
	}
	if starter.calls != 1 || len(starter.wf.Rows) != 0 || len(starter.wf.Nodes) != 3 || starter.wf.Nodes[0].Limit != 20 || starter.wf.Nodes[0].Stages[0] != "SHIPPING" {
		t.Fatalf("the runner got the pick settings and no rows: %+v", starter.wf)
	}
}

func TestASheetSentWithAPickIsIgnored(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, _, member := newWorkflowApp(t, starter, &memWorkflowStore{})
	b := `{"confirm":true,"nodes":[` + pickNode + `],"edges":[],"items":` + oneRow + `}`
	if code, _ := do(t, app, "POST", "/api/v1/workflows", member, b); code != 202 || len(starter.wf.Rows) != 0 {
		t.Fatalf("the KOCs come from the directory, not from a sheet: %d rows=%d", code, len(starter.wf.Rows))
	}
}

func TestAWorkflowThatStartsFromAnImportStillNeedsRows(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, _, member := newWorkflowApp(t, starter, &memWorkflowStore{})
	for name, b := range map[string]string{
		"no items at all": bodyNoItems(importNode+","+waitNode, importToWait),
		"empty items":     `{"confirm":true,"nodes":[` + importNode + `,` + waitNode + `],"edges":` + importToWait + `,"items":[]}`,
	} {
		if code, _ := do(t, app, "POST", "/api/v1/workflows", member, b); code != 400 {
			t.Errorf("%s must be refused with 400, got %d", name, code)
		}
	}
	if starter.calls != 0 {
		t.Fatal("nothing was started")
	}
}

func TestAMemberMayRunWhatOnlyLabelsMovesAndWaits(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, _, member := newWorkflowApp(t, starter, &memWorkflowStore{})
	if code, body := do(t, app, "POST", "/api/v1/workflows", member, bodyNoItems(pickNode+","+untilNode+","+labelNode+","+stageNode, wire("p", "u", "l", "g"))); code != 202 {
		t.Fatalf("no message is sent by these blocks: %d %v", code, body)
	}
}

func TestOnlyAnAdminStartsAWorkflowThatSendsATextOrAnEmail(t *testing.T) {
	for name, node := range map[string]string{"send_text": sendTextNode, "email": emailNode} {
		t.Run(name, func(t *testing.T) {
			starter := &fakeWorkflowStarter{}
			app, admin, member := newWorkflowApp(t, starter, &memWorkflowStore{})
			b := bodyNoItems(pickNode+","+node, wire("p", node[7:8]))
			if code, _ := do(t, app, "POST", "/api/v1/workflows", member, b); code != 403 {
				t.Fatalf("a member may not start a workflow that messages people: %d", code)
			}
			if starter.calls != 0 {
				t.Fatal("nothing was started")
			}
			if code, body := do(t, app, "POST", "/api/v1/workflows", admin, b); code != 202 {
				t.Fatalf("an admin may: %d %v", code, body)
			}
		})
	}
}

func TestTheSettingsOfAnotherBlockTypeAreDropped(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, _, member := newWorkflowApp(t, starter, &memWorkflowStore{})
	dirty := `{"id":"w","type":"wait","waitMinutes":30,"text":"tin lạ","labelId":"65a1b2c3d4e5f60718293a4b","stage":"REPLIED","campaignId":"c1","steps":[{"kind":"text","text":"hack"}]}`
	if code, body := do(t, app, "POST", "/api/v1/workflows", member, bodyNoItems(pickNode+","+dirty, wire("p", "w"))); code != 202 {
		t.Fatalf("%d %v", code, body)
	}
	var w domain.WorkflowNode
	for _, n := range starter.wf.Nodes {
		if n.ID == "w" {
			w = n
		}
	}
	if w.WaitMinutes != 30 || w.Text != "" || w.LabelID != "" || w.Stage != "" || w.CampaignID != "" || w.Steps != nil {
		t.Fatalf("a wait block keeps only its wait: %+v", w)
	}
}

func TestABlockWithBadSettingsIsRefused(t *testing.T) {
	starter := &fakeWorkflowStarter{}
	app, admin, _ := newWorkflowApp(t, starter, &memWorkflowStore{})
	bad := map[string]string{
		"a text with no content":         `{"id":"t","type":"send_text"}`,
		"an email with no content":       `{"id":"m","type":"email","subject":"x"}`,
		"a label that is not an id":      `{"id":"l","type":"label","labelId":"nhan"}`,
		"a stage that is not offered":    `{"id":"g","type":"stage","stage":"COMPLETED"}`,
		"a time of day that is not one":  `{"id":"u","type":"wait_until","atMinute":2000}`,
		"a pick with no stage or folder": `{"id":"q","type":"pick","limit":10}`,
	}
	for name, node := range bad {
		b := bodyNoItems(pickNode+","+node, wire("p", node[7:8]))
		if code, _ := do(t, app, "POST", "/api/v1/workflows", admin, b); code != 400 {
			t.Errorf("%s must be refused with 400, got %d", name, code)
		}
	}
	if starter.calls != 0 {
		t.Fatalf("nothing was started: %d", starter.calls)
	}
}
