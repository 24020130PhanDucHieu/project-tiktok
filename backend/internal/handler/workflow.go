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
	RequestStop(ctx context.Context, id string) (bool, error)
}

// WorkflowCampaigns holds the saved campaigns that the send blocks choose from (*repository.CampaignRepository).
type WorkflowCampaigns interface {
	Find(ctx context.Context, id string) (*domain.Campaign, error)
	ImagesExist(ctx context.Context, ids []domain.ID) (bool, error)
}

// WorkflowHandler serves the Quy trình tab: start a workflow, follow it, stop it.
type WorkflowHandler struct {
	starter   WorkflowStarter
	workflows WorkflowReader
	campaigns WorkflowCampaigns
	audit     Auditor
}

// NewWorkflowHandler builds the handler.
func NewWorkflowHandler(starter WorkflowStarter, workflows WorkflowReader, campaigns WorkflowCampaigns, audit Auditor) *WorkflowHandler {
	return &WorkflowHandler{starter: starter, workflows: workflows, campaigns: campaigns, audit: audit}
}

type workflowRequest struct {
	Name   string `json:"name" validate:"omitempty,max=100"`
	Source string `json:"source" validate:"omitempty,oneof='TikToday' 'TikTok Liên kết' 'TikTok'"`
	// the graph is checked by domain.ValidateWorkflowGraph (one place for the rules), not by tags
	Nodes []domain.WorkflowNode `json:"nodes" validate:"required,min=1,max=15"`
	Edges []domain.WorkflowEdge `json:"edges" validate:"max=60"`
	// the rows are not validated here: a bad row is marked invalid in the workflow, it does not refuse the sheet
	Items   []importItemRequest `json:"items" validate:"required,min=1,max=200"`
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
	hasSend := false
	for i := range req.Nodes {
		n := &req.Nodes[i]
		if n.Type != domain.NodeWait {
			n.WaitMinutes = 0
		}
		if n.Type == domain.NodeSend {
			hasSend = true
		} else {
			n.CampaignID = ""
		}
		n.CampaignName, n.Steps = "", nil
	}
	if err := domain.ValidateWorkflowGraph(req.Nodes, req.Edges); err != nil {
		return err
	}
	if u := middleware.CurrentUser(c); hasSend && (u == nil || u.Role != domain.RoleAdmin) {
		return httpx.Fail(c, fiber.StatusForbidden, "Chỉ quản trị viên được chạy quy trình có bước gửi tin.")
	}

	rows := make([]domain.ImportRow, len(req.Items))
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

	ctx := c.UserContext()
	for i := range req.Nodes {
		n := &req.Nodes[i]
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
	return httpx.OK(c, fiber.Map{"workflow": withoutRows(wf)})
}

// Get handles GET /workflows/:id: the workflow as it is now, with the state of every block.
func (h *WorkflowHandler) Get(c *fiber.Ctx) error {
	wf, err := h.workflows.Find(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, withoutRows(wf))
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
