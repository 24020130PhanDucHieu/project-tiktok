package router_test

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// memTemplates is the store of saved workflows, with the limit of the real one.
type memTemplates struct {
	byID  map[string]*domain.WorkflowTemplate
	limit int
}

func newMemTemplates() *memTemplates {
	return &memTemplates{byID: map[string]*domain.WorkflowTemplate{}, limit: domain.MaxWorkflowTemplates}
}

func (m *memTemplates) List(context.Context) ([]domain.WorkflowTemplate, error) {
	out := []domain.WorkflowTemplate{}
	for _, t := range m.byID {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (m *memTemplates) Find(_ context.Context, id string) (*domain.WorkflowTemplate, error) {
	if t := m.byID[id]; t != nil {
		return t, nil
	}
	return nil, domain.NotFound("Không tìm thấy quy trình đã lưu.")
}
func (m *memTemplates) Create(_ context.Context, t *domain.WorkflowTemplate) error {
	if len(m.byID) >= m.limit {
		return domain.Conflict("Đã có tối đa quy trình đã lưu. Hãy xóa bớt.")
	}
	t.ID, t.CreatedAt, t.UpdatedAt = domain.NewID(), time.Now(), time.Now()
	cp := *t
	m.byID[t.ID.Hex()] = &cp
	return nil
}
func (m *memTemplates) Update(_ context.Context, id, name string, nodes []domain.WorkflowNode, edges []domain.WorkflowEdge, at time.Time) error {
	t, ok := m.byID[id]
	if !ok {
		return domain.NotFound("Không tìm thấy quy trình đã lưu.")
	}
	t.Name, t.Nodes, t.Edges, t.UpdatedAt = name, nodes, edges, at
	return nil
}
func (m *memTemplates) Delete(_ context.Context, id string) error {
	if _, ok := m.byID[id]; !ok {
		return domain.NotFound("Không tìm thấy quy trình đã lưu.")
	}
	delete(m.byID, id)
	return nil
}

func newTemplateApp(t *testing.T, store *memTemplates) (app *fiber.App, admin, member string) {
	t.Helper()
	return newAppFull(t, nil, &fakeRunner{}, fakeRunReader{}, &memLimit{value: 30}, func(h *router.Handlers) {
		h.WorkflowTemplate = handler.NewWorkflowTemplateHandler(store, service.NewAuditService(nil))
	})
}

func templateBody(name, nodes, edges string) string {
	return `{"name":"` + name + `","nodes":[` + nodes + `],"edges":` + edges + `}`
}

const (
	draftNodes = `{"id":"import-1","type":"import","x":40,"y":120},{"id":"send-1","type":"send","x":300,"y":120}`
	draftEdges = `[{"id":"e1","from":"import-1","port":"out","to":"send-1"}]`
)

func seed(store *memTemplates, by string) *domain.WorkflowTemplate {
	t := &domain.WorkflowTemplate{Name: "Của người khác", CreatedBy: by, Nodes: []domain.WorkflowNode{{ID: "import-1", Type: domain.NodeImport}}}
	_ = store.Create(context.Background(), t)
	return store.byID[t.ID.Hex()]
}

func TestWorkflowTemplateRoutesNeedALogin(t *testing.T) {
	app, _, _ := newTemplateApp(t, newMemTemplates())
	for _, r := range [][2]string{{"GET", "/api/v1/workflow-templates"}, {"POST", "/api/v1/workflow-templates"}, {"GET", "/api/v1/workflow-templates/x"}, {"PUT", "/api/v1/workflow-templates/x"}, {"DELETE", "/api/v1/workflow-templates/x"}} {
		if code, _ := do(t, app, r[0], r[1], "", `{}`); code != 401 {
			t.Errorf("%s %s needs a login: %d", r[0], r[1], code)
		}
	}
}

func TestAMemberSavesADraftThatCannotRunYet(t *testing.T) {
	store := newMemTemplates()
	app, _, member := newTemplateApp(t, store)
	code, body := do(t, app, "POST", "/api/v1/workflow-templates", member, templateBody("Nhắc KOC", draftNodes, draftEdges))
	data, _ := body["data"].(map[string]any)
	if code != 201 || data["_id"] == nil || data["name"] != "Nhắc KOC" || data["createdBy"] == "" {
		t.Fatalf("201 with the template: %d %v", code, body)
	}
	if len(store.byID) != 1 {
		t.Fatal("it was stored")
	}
}

func TestASavedTemplateNeverHoldsTheStepsOfACampaign(t *testing.T) {
	store := newMemTemplates()
	app, _, member := newTemplateApp(t, store)
	cheat := `{"id":"send-1","type":"send","campaignId":"c1","campaignName":"Tên","steps":[{"kind":"text","text":"nội dung không được lưu"}]}`
	if code, body := do(t, app, "POST", "/api/v1/workflow-templates", member, templateBody("Mẫu", `{"id":"import-1","type":"import"},`+cheat, draftEdges)); code != 201 {
		t.Fatalf("%d %v", code, body)
	}
	for _, tpl := range store.byID {
		for _, n := range tpl.Nodes {
			if n.Type == domain.NodeSend && (n.CampaignName != "" || n.Steps != nil || n.CampaignID != "c1") {
				t.Fatalf("only the id of the campaign is kept: %+v", n)
			}
		}
	}
}

func TestABadTemplateIsRefused(t *testing.T) {
	store := newMemTemplates()
	app, _, member := newTemplateApp(t, store)
	var many []string
	for i := 0; i < 26; i++ {
		many = append(many, `{"id":"n`+string(rune('a'+i))+`","type":"wait","waitMinutes":5}`)
	}
	bad := map[string]string{
		"no name":                 templateBody("", draftNodes, draftEdges),
		"no blocks":               `{"name":"x","nodes":[],"edges":[]}`,
		"twenty six blocks":       templateBody("x", strings.Join(many, ","), `[]`),
		"an unknown block type":   templateBody("x", `{"id":"a","type":"sms"}`, `[]`),
		"a wire to nowhere":       templateBody("x", draftNodes, `[{"id":"e","from":"import-1","port":"out","to":"ghost"}]`),
		"a text that is too long": templateBody("x", `{"id":"a","type":"send_text","text":"`+strings.Repeat("a", 2001)+`"}`, `[]`),
	}
	for name, b := range bad {
		if code, _ := do(t, app, "POST", "/api/v1/workflow-templates", member, b); code != 400 {
			t.Errorf("%s must be refused with 400, got %d", name, code)
		}
	}
	if len(store.byID) != 0 {
		t.Fatal("nothing was stored")
	}
}

func TestTheFiftyFirstTemplateAnswersConflict(t *testing.T) {
	store := newMemTemplates()
	store.limit = 1
	app, _, member := newTemplateApp(t, store)
	if code, _ := do(t, app, "POST", "/api/v1/workflow-templates", member, templateBody("Một", draftNodes, draftEdges)); code != 201 {
		t.Fatal("the first fits")
	}
	if code, _ := do(t, app, "POST", "/api/v1/workflow-templates", member, templateBody("Hai", draftNodes, draftEdges)); code != 409 {
		t.Fatalf("over the limit: %d", code)
	}
}

func TestTheListShowsASummaryAndNotTheBlocks(t *testing.T) {
	store := newMemTemplates()
	app, _, member := newTemplateApp(t, store)
	seed(store, "ai-do@x.vn")
	code, body := do(t, app, "GET", "/api/v1/workflow-templates", member, ``)
	items, _ := body["data"].([]any)
	if code != 200 || len(items) != 1 {
		t.Fatalf("%d %v", code, body)
	}
	first, _ := items[0].(map[string]any)
	if first["name"] != "Của người khác" || first["createdBy"] != "ai-do@x.vn" || first["nodeCount"] != float64(1) || first["updatedAt"] == nil {
		t.Fatalf("name, who made it, how many blocks, when: %v", first)
	}
	if _, has := first["nodes"]; has {
		t.Fatal("the list does not carry the blocks")
	}
}

func TestOneTemplateComesBackWithItsBlocks(t *testing.T) {
	store := newMemTemplates()
	app, _, member := newTemplateApp(t, store)
	tpl := seed(store, "ai-do@x.vn")
	code, body := do(t, app, "GET", "/api/v1/workflow-templates/"+tpl.ID.Hex(), member, ``)
	data, _ := body["data"].(map[string]any)
	nodes, _ := data["nodes"].([]any)
	if code != 200 || len(nodes) != 1 {
		t.Fatalf("%d %v", code, body)
	}
	if code, _ := do(t, app, "GET", "/api/v1/workflow-templates/"+domain.NewID().Hex(), member, ``); code != 404 {
		t.Fatalf("an unknown template: %d", code)
	}
}

func TestAMemberChangesAndDeletesOnlyTheirOwnTemplates(t *testing.T) {
	store := newMemTemplates()
	app, _, member := newTemplateApp(t, store)
	other := seed(store, "ai-do@x.vn")
	otherPath := "/api/v1/workflow-templates/" + other.ID.Hex()
	if code, _ := do(t, app, "PUT", otherPath, member, templateBody("Đổi tên", draftNodes, draftEdges)); code != 403 {
		t.Fatalf("a member may not change somebody else's template: %d", code)
	}
	if code, _ := do(t, app, "DELETE", otherPath, member, ``); code != 403 {
		t.Fatalf("or delete it: %d", code)
	}
	if store.byID[other.ID.Hex()].Name != "Của người khác" {
		t.Fatal("it is untouched")
	}

	// their own
	code, body := do(t, app, "POST", "/api/v1/workflow-templates", member, templateBody("Của tôi", draftNodes, draftEdges))
	mine := body["data"].(map[string]any)["_id"].(string)
	if code != 201 {
		t.Fatal("created")
	}
	if code, _ := do(t, app, "PUT", "/api/v1/workflow-templates/"+mine, member, templateBody("Tên mới", draftNodes, draftEdges)); code != 200 || store.byID[mine].Name != "Tên mới" {
		t.Fatalf("the owner changes it: %d", code)
	}
	if code, _ := do(t, app, "DELETE", "/api/v1/workflow-templates/"+mine, member, ``); code != 200 || store.byID[mine] != nil {
		t.Fatalf("the owner deletes it: %d", code)
	}
}

func TestAnAdminChangesAndDeletesAnyTemplate(t *testing.T) {
	store := newMemTemplates()
	app, admin, _ := newTemplateApp(t, store)
	other := seed(store, "ai-do@x.vn")
	path := "/api/v1/workflow-templates/" + other.ID.Hex()
	if code, _ := do(t, app, "PUT", path, admin, templateBody("Admin đổi", draftNodes, draftEdges)); code != 200 || store.byID[other.ID.Hex()].Name != "Admin đổi" {
		t.Fatalf("an admin may change any template: %d", code)
	}
	if code, _ := do(t, app, "DELETE", path, admin, ``); code != 200 || store.byID[other.ID.Hex()] != nil {
		t.Fatalf("and delete any: %d", code)
	}
	if code, _ := do(t, app, "DELETE", path, admin, ``); code != 404 {
		t.Fatalf("a template that is gone: %d", code)
	}
}

func TestChangingATemplateChecksTheDrawingAgain(t *testing.T) {
	store := newMemTemplates()
	app, admin, _ := newTemplateApp(t, store)
	tpl := seed(store, "ai-do@x.vn")
	if code, _ := do(t, app, "PUT", "/api/v1/workflow-templates/"+tpl.ID.Hex(), admin, templateBody("", draftNodes, draftEdges)); code != 400 {
		t.Fatalf("an empty name: %d", code)
	}
}
