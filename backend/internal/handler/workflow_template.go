package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
)

// WorkflowTemplates keeps the saved workflows (*repository.WorkflowTemplateRepository).
type WorkflowTemplates interface {
	List(ctx context.Context) ([]domain.WorkflowTemplate, error)
	Find(ctx context.Context, id string) (*domain.WorkflowTemplate, error)
	Create(ctx context.Context, t *domain.WorkflowTemplate) error
	Update(ctx context.Context, id, name string, nodes []domain.WorkflowNode, edges []domain.WorkflowEdge, at time.Time) error
	Delete(ctx context.Context, id string) error
}

// WorkflowTemplateHandler serves the saved workflows of the Quy trình tab. Everybody signed in sees and saves them; a template is
// changed or deleted by the person who made it, or by an admin.
type WorkflowTemplateHandler struct {
	store WorkflowTemplates
	audit Auditor
}

// NewWorkflowTemplateHandler builds the handler.
func NewWorkflowTemplateHandler(store WorkflowTemplates, audit Auditor) *WorkflowTemplateHandler {
	return &WorkflowTemplateHandler{store: store, audit: audit}
}

func trimName(s string) string { return strings.TrimSpace(s) }

type templateRequest struct {
	Name string `json:"name" validate:"required,max=400"` // the length in characters is checked by ValidateTemplate
	// the structure is checked by domain.ValidateTemplate (one place for the rules), not by tags
	Nodes []domain.WorkflowNode `json:"nodes" validate:"required,min=1,max=25"`
	Edges []domain.WorkflowEdge `json:"edges" validate:"max=60"`
}

type templateSummary struct {
	ID        domain.ID `json:"_id"`
	Name      string    `json:"name"`
	CreatedBy string    `json:"createdBy"`
	NodeCount int       `json:"nodeCount"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// List handles GET /workflow-templates: a summary of each template, newest change first.
func (h *WorkflowTemplateHandler) List(c *fiber.Ctx) error {
	items, err := h.store.List(c.UserContext())
	if err != nil {
		return err
	}
	out := make([]templateSummary, len(items))
	for i, t := range items {
		out[i] = templateSummary{ID: t.ID, Name: t.Name, CreatedBy: t.CreatedBy, NodeCount: len(t.Nodes), UpdatedAt: t.UpdatedAt}
	}
	return httpx.OK(c, out)
}

// Get handles GET /workflow-templates/:id: one template with its blocks and wires.
func (h *WorkflowTemplateHandler) Get(c *fiber.Ctx) error {
	t, err := h.store.Find(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, t)
}

// Create handles POST /workflow-templates. A draft that cannot run yet is saved as it is.
func (h *WorkflowTemplateHandler) Create(c *fiber.Ctx) error {
	var req templateRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	nodes, err := domain.ValidateTemplate(req.Name, req.Nodes, req.Edges)
	if err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	t := &domain.WorkflowTemplate{Name: trimName(req.Name), Nodes: nodes, Edges: req.Edges, CreatedBy: a.Email}
	if err := h.store.Create(c.UserContext(), t); err != nil {
		return err
	}
	h.audit.Log(c.UserContext(), a.ID, a.Email, "workflow_template.create", "workflow_template", t.ID.Hex(), fmt.Sprintf("%d khối", len(nodes)), a.IP)
	return httpx.Created(c, t)
}

// mayChange says whether the caller made the template or is an admin.
func mayChange(c *fiber.Ctx, t *domain.WorkflowTemplate) bool {
	u := middleware.CurrentUser(c)
	return u != nil && (u.Role == domain.RoleAdmin || (t.CreatedBy != "" && t.CreatedBy == u.Email))
}

// Update handles PUT /workflow-templates/:id: a new name, new blocks, or both.
func (h *WorkflowTemplateHandler) Update(c *fiber.Ctx) error {
	var req templateRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	existing, err := h.store.Find(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	if !mayChange(c, existing) {
		return httpx.Fail(c, fiber.StatusForbidden, "Chỉ người tạo hoặc quản trị viên được sửa quy trình đã lưu này.")
	}
	nodes, err := domain.ValidateTemplate(req.Name, req.Nodes, req.Edges)
	if err != nil {
		return err
	}
	if err := h.store.Update(c.UserContext(), c.Params("id"), trimName(req.Name), nodes, req.Edges, time.Now()); err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	h.audit.Log(c.UserContext(), a.ID, a.Email, "workflow_template.update", "workflow_template", c.Params("id"), "", a.IP)
	return httpx.OK(c, fiber.Map{"updated": true})
}

// Delete handles DELETE /workflow-templates/:id.
func (h *WorkflowTemplateHandler) Delete(c *fiber.Ctx) error {
	existing, err := h.store.Find(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	if !mayChange(c, existing) {
		return httpx.Fail(c, fiber.StatusForbidden, "Chỉ người tạo hoặc quản trị viên được xóa quy trình đã lưu này.")
	}
	if err := h.store.Delete(c.UserContext(), c.Params("id")); err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	h.audit.Log(c.UserContext(), a.ID, a.Email, "workflow_template.delete", "workflow_template", c.Params("id"), "", a.IP)
	return httpx.OK(c, fiber.Map{"deleted": true})
}
