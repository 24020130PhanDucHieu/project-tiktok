package handler

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
)

// WorkflowStarter begins a workflow in the background (*service.WorkflowRunner).
type WorkflowStarter interface {
	Start(ctx context.Context, by string, wf *domain.Workflow) (*domain.Workflow, error)
}

// WorkflowReader reads workflows and asks one to stop (*repository.WorkflowRepository).
type WorkflowReader interface {
	Latest(ctx context.Context) (*domain.Workflow, error)
	Find(ctx context.Context, id string) (*domain.Workflow, error)
	List(ctx context.Context, limit int) ([]*domain.Workflow, error)
	RequestStop(ctx context.Context, id string) (bool, error)
}

// WorkflowCampaigns holds the saved campaigns that the send blocks choose from (*repository.CampaignRepository).
type WorkflowCampaigns interface {
	Find(ctx context.Context, id string) (*domain.Campaign, error)
	ImagesExist(ctx context.Context, ids []domain.ID) (bool, error)
}

// WorkflowKocs gives the names of the KOCs a report speaks of (*repository.KocRepository).
type WorkflowKocs interface {
	FindByIDs(ctx context.Context, ids []domain.ID) ([]*domain.Koc, error)
}

// WorkflowPicker takes KOCs from the directory for a pick block (*service.KocPickerService).
type WorkflowPicker interface {
	PickBy(ctx context.Context, spec domain.PickSpec) ([]domain.TokenKoc, error)
}

// WorkflowAccounts checks the Zalo account a send block names (*service.ZaloService): it has to exist and be usable.
type WorkflowAccounts interface {
	ResolveAccount(ctx context.Context, id string) (string, error)
}

// WorkflowHandler serves the Quy trình tab: start a workflow, follow it, stop it.
type WorkflowHandler struct {
	starter   WorkflowStarter
	workflows WorkflowReader
	campaigns WorkflowCampaigns
	audit     Auditor
	kocs      WorkflowKocs
	picker    WorkflowPicker
	accounts  WorkflowAccounts // nil: the account of a send block is not checked here
}

// WithAccounts lets the handler refuse a workflow whose send block names a Zalo account that does not exist or was
// disconnected, before anything starts.
func (h *WorkflowHandler) WithAccounts(a WorkflowAccounts) *WorkflowHandler {
	h.accounts = a
	return h
}

// WithPicker lets the handler show which KOCs a pick block would take (the preview).
func (h *WorkflowHandler) WithPicker(p WorkflowPicker) *WorkflowHandler {
	h.picker = p
	return h
}

// WithKocs lets the answers carry the names of the KOCs in the report of a workflow.
func (h *WorkflowHandler) WithKocs(k WorkflowKocs) *WorkflowHandler {
	h.kocs = k
	return h
}

// NewWorkflowHandler builds the handler.
func NewWorkflowHandler(starter WorkflowStarter, workflows WorkflowReader, campaigns WorkflowCampaigns, audit Auditor) *WorkflowHandler {
	return &WorkflowHandler{starter: starter, workflows: workflows, campaigns: campaigns, audit: audit}
}

type workflowRequest struct {
	Name   string `json:"name" validate:"omitempty,max=100"`
	Source string `json:"source" validate:"omitempty,oneof='TikToday' 'TikTok Liên kết' 'TikTok'"`
	// the graph is checked by domain.ValidateWorkflowGraph (one place for the rules), not by tags
	Nodes []domain.WorkflowNode `json:"nodes" validate:"required,min=1,max=25"`
	Edges []domain.WorkflowEdge `json:"edges" validate:"max=60"`
	// the rows are not validated here: a bad row is marked invalid in the workflow, it does not refuse the sheet. A workflow that
	// starts from a pick block has no sheet.
	Items   []importItemRequest `json:"items" validate:"omitempty,max=200"`
	Confirm bool                `json:"confirm"`
}

// withoutRows is the workflow as the browser may see it: the sheet holds phones, emails and addresses of people, and the
// screen only needs the blocks, the tokens and the states.
func withoutRows(wf *domain.Workflow) *domain.Workflow {
	if wf == nil {
		return nil
	}
	cp := *wf
	cp.Rows = nil
	return &cp
}

// workflowView is a workflow with the names of the KOCs its report speaks of (id → name), so the screen can say who.
type workflowView struct {
	*domain.Workflow
	KocNames map[string]string `json:"kocNames,omitempty"`
}

// view is the workflow as the browser may see it: without the sheet, and with the names of the KOCs of its report.
func (h *WorkflowHandler) view(ctx context.Context, wf *domain.Workflow) *workflowView {
	wf = withoutRows(wf)
	if wf == nil {
		return nil
	}
	v := &workflowView{Workflow: wf}
	if h.kocs == nil || len(wf.Log) == 0 {
		return v
	}
	var all []string
	for _, ev := range wf.Log {
		all = append(all, ev.Kocs...)
	}
	v.KocNames = h.namesOf(ctx, all) // an error leaves the report with the ids only
	return v
}

// namesOf gives the names of KOCs by id: the display name, or the TikTok handle when a KOC has none. A KOC that is gone, or a
// store that cannot answer, leaves the id out.
func (h *WorkflowHandler) namesOf(ctx context.Context, kocIDs []string) map[string]string {
	seen := map[string]bool{}
	var ids []domain.ID
	for _, k := range kocIDs {
		if id, err := domain.ParseID(k); err == nil && !seen[k] {
			seen[k] = true
			ids = append(ids, id)
		}
	}
	found, err := h.kocs.FindByIDs(ctx, ids)
	if err != nil {
		return nil
	}
	names := make(map[string]string, len(found))
	for _, k := range found {
		name := k.DisplayName
		if name == "" {
			name = "@" + k.TiktokID
		}
		names[k.ID.Hex()] = name
	}
	return names
}

type previewRequest struct {
	Nodes []domain.WorkflowNode `json:"nodes" validate:"required,min=1,max=25"`
	Edges []domain.WorkflowEdge `json:"edges" validate:"max=60"`
}

type previewKoc struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Preview handles POST /workflows/preview: who a workflow that starts with "Lấy KOC có sẵn" would take if it started now.
// It writes nothing and sends nothing, so anybody signed in may ask. A workflow that starts from an Excel file has no such
// list: its people are the rows of the file, which the browser already has.
func (h *WorkflowHandler) Preview(c *fiber.Ctx) error {
	var req previewRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	var start *domain.WorkflowNode
	for i := range req.Nodes {
		req.Nodes[i] = domain.SanitizeNode(req.Nodes[i])
		if domain.IsStartNode(req.Nodes[i].Type) {
			start = &req.Nodes[i]
		}
	}
	if err := domain.ValidateWorkflowGraph(req.Nodes, req.Edges); err != nil {
		return err
	}
	if start == nil || start.Type != domain.NodePick {
		return domain.Invalid("Xem trước chỉ dùng được khi quy trình bắt đầu bằng khối Lấy KOC có sẵn.")
	}
	if h.picker == nil || h.kocs == nil {
		return domain.Invalid("Chưa cấu hình được việc xem trước.")
	}
	ctx := c.UserContext()
	picked, err := h.picker.PickBy(ctx, domain.PickSpec{Stages: start.Stages, Folder: start.Folder, Source: start.Source, KocIDs: start.KocIDs, Limit: start.Limit})
	if err != nil {
		return err
	}
	ids := make([]string, len(picked))
	for i, k := range picked {
		ids[i] = k.KocID
	}
	names := h.namesOf(ctx, ids)
	out := make([]previewKoc, len(ids))
	for i, id := range ids {
		name := names[id]
		if name == "" {
			name = "KOC không còn trong danh sách"
		}
		out[i] = previewKoc{ID: id, Name: name}
	}
	return httpx.OK(c, fiber.Map{"total": len(out), "kocs": out})
}

// Start handles POST /workflows. It checks the graph and the rows again (the screen is not trusted), takes the steps of
// every campaign a send block chose at this moment, starts the runner and answers 202 with the workflow: the work has
// been taken on, not done. A workflow that sends needs an admin and confirm=true: it messages people on its own, for days.
func (h *WorkflowHandler) Start(c *fiber.Ctx) error {
	var req workflowRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	if !req.Confirm {
		return httpx.Fail(c, fiber.StatusBadRequest, "Cần xác nhận trước khi chạy quy trình.")
	}

	// only the settings each block type has are taken: the browser never says which steps a send block sends
	sending := false
	var startType string
	for i := range req.Nodes {
		req.Nodes[i] = domain.SanitizeNode(req.Nodes[i])
		sending = sending || domain.IsSendingNode(req.Nodes[i].Type)
		if domain.IsStartNode(req.Nodes[i].Type) {
			startType = req.Nodes[i].Type
		}
	}
	if err := domain.ValidateWorkflowGraph(req.Nodes, req.Edges); err != nil {
		return err
	}
	if u := middleware.CurrentUser(c); sending && (u == nil || u.Role != domain.RoleAdmin) {
		return httpx.Fail(c, fiber.StatusForbidden, "Chỉ quản trị viên được chạy quy trình có bước gửi tin.")
	}

	var rows []domain.ImportRow
	if startType == domain.NodeImport {
		rows = make([]domain.ImportRow, len(req.Items))
		valid := 0
		for i, it := range req.Items {
			line := it.Line
			if line == 0 {
				line = i + 2 // the header is row 1
			}
			rows[i] = domain.ImportRow{
				Line: line, TiktokID: it.TiktokID, DisplayName: it.DisplayName, Phone: it.Phone, Email: it.Email,
				Recipient: it.Recipient, ShippingAddress: it.Address, Category: it.Category,
				Followers: it.Followers, Gmv: it.Gmv, CastFee: it.CastFee,
			}
			if reason := rowProblem(it); reason != "" {
				rows[i].Status, rows[i].Reason = domain.RowInvalid, reason
			} else {
				valid++
			}
		}
		if valid == 0 {
			return domain.Invalid("Không có dòng nào hợp lệ để nhập.")
		}
	}

	ctx := c.UserContext()
	for i := range req.Nodes {
		n := &req.Nodes[i]
		if h.accounts != nil && n.ZaloAccountID != "" { // only send / send_text blocks keep one (SanitizeNode)
			id, err := h.accounts.ResolveAccount(ctx, n.ZaloAccountID)
			if err != nil {
				return err
			}
			n.ZaloAccountID = id
		}
		if n.Type != domain.NodeSend {
			continue
		}
		campaign, err := h.campaigns.Find(ctx, n.CampaignID)
		if err != nil {
			if isNotFound(err) {
				return domain.Invalid("Không tìm thấy chiến dịch của một bước gửi.")
			}
			return err
		}
		if len(campaign.Steps) == 0 {
			return domain.Invalid(fmt.Sprintf("Chiến dịch %q chưa có tin nào.", campaign.Name))
		}
		if err := CheckSteps(ctx, h.campaigns, campaign.Steps); err != nil {
			return err
		}
		n.CampaignName, n.Steps = campaign.Name, campaign.Steps
	}

	name := req.Name
	if name == "" {
		name = "Quy trình"
	}
	a := middleware.ActorOf(c)
	started, err := h.starter.Start(ctx, a.Email, &domain.Workflow{
		Name: name, Nodes: req.Nodes, Edges: req.Edges, Rows: rows, Source: defaultStr(req.Source, "TikToday"),
	})
	if err != nil {
		return err
	}
	h.audit.Log(ctx, a.ID, a.Email, "workflow.start", "workflow", started.ID.Hex(), fmt.Sprintf("%d khối, %d dòng", len(req.Nodes), len(rows)), a.IP)
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"success": true, "data": withoutRows(started), "message": "Đã nhận quy trình, máy chủ sẽ tự chạy."})
}

// Latest handles GET /workflows/latest: the workflow that was made last, running or ended, or null. The screen asks it when
// it opens, so that a workflow started before the page was closed or reloaded shows up again.
func (h *WorkflowHandler) Latest(c *fiber.Ctx) error {
	wf, err := h.workflows.Latest(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, fiber.Map{"workflow": h.view(c.UserContext(), wf)})
}

// HistoryLimit is how many past runs the history lists.
const HistoryLimit = 50

// List handles GET /workflows: the past runs, newest first, as summaries (the blocks and the state of each, without the
// sheet and without the report, which is read one run at a time).
func (h *WorkflowHandler) List(c *fiber.Ctx) error {
	list, err := h.workflows.List(c.UserContext(), HistoryLimit)
	if err != nil {
		return err
	}
	out := make([]*domain.Workflow, 0, len(list))
	for _, wf := range list {
		cp := withoutRows(wf)
		cp.Log, cp.Tokens = nil, nil
		out = append(out, cp)
	}
	return httpx.OK(c, fiber.Map{"workflows": out})
}

// Get handles GET /workflows/:id: the workflow as it is now, with the state of every block.
func (h *WorkflowHandler) Get(c *fiber.Ctx) error {
	wf, err := h.workflows.Find(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, h.view(c.UserContext(), wf))
}

// Stop handles PUT /workflows/:id/stop: the runner stops at its next look (a few seconds), and so does the job it waits for.
func (h *WorkflowHandler) Stop(c *fiber.Ctx) error {
	found, err := h.workflows.RequestStop(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	if !found {
		return domain.Conflict("Quy trình này đã kết thúc, không có gì để dừng.")
	}
	a := middleware.ActorOf(c)
	h.audit.Log(c.UserContext(), a.ID, a.Email, "workflow.stop", "workflow", c.Params("id"), "", a.IP)
	return httpx.OK(c, fiber.Map{"stopping": true})
}
