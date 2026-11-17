package handler

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
)

// ImportStarter begins a background import (*service.ImportRunner).
type ImportStarter interface {
	Start(ctx context.Context, by, source string, rows []domain.ImportRow) (*domain.ImportJob, error)
}

// ImportJobs reads import jobs and asks one to stop (*repository.ImportJobRepository).
type ImportJobs interface {
	Running(ctx context.Context) (*domain.ImportJob, error)
	Find(ctx context.Context, id string) (*domain.ImportJob, error)
	RequestStop(ctx context.Context, id string) (bool, error)
}

// ImportJobHandler serves the background import of KOCs from an Excel sheet: start a job, follow it, stop it.
type ImportJobHandler struct {
	starter ImportStarter
	jobs    ImportJobs
	audit   Auditor
}

// NewImportJobHandler builds the handler.
func NewImportJobHandler(starter ImportStarter, jobs ImportJobs, audit Auditor) *ImportJobHandler {
	return &ImportJobHandler{starter: starter, jobs: jobs, audit: audit}
}

// importItemRequest is one row of the sheet: the fields of a bulk row and its number in the sheet.
type importItemRequest struct {
	bulkItemRequest
	Line int `json:"line" validate:"omitempty,gte=1"`
}

type importJobRequest struct {
	Source string              `json:"source" validate:"omitempty,oneof='TikToday' 'TikTok Liên kết' 'TikTok'"`
	Items  []importItemRequest `json:"items" validate:"required,min=1,max=200,dive"`
}

// Start handles POST /import-jobs. It checks the rows, stores the job, starts the worker and answers 202 with the job:
// the work has been taken on, not done. The screen then follows it with GET /import-jobs/:id.
func (h *ImportJobHandler) Start(c *fiber.Ctx) error {
	var req importJobRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	rows := make([]domain.ImportRow, len(req.Items))
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
	}
	a := middleware.ActorOf(c)
	job, err := h.starter.Start(c.UserContext(), a.Email, defaultStr(req.Source, "TikToday"), rows)
	if err != nil {
		return err
	}
	h.audit.Log(c.UserContext(), a.ID, a.Email, "import_job.start", "import_job", job.ID.Hex(), fmt.Sprintf("%d dòng", len(rows)), a.IP)
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"success": true, "data": job, "message": "Đã nhận file, máy chủ đang nhập từng dòng."})
}

// Current handles GET /import-jobs/current: the job that is running, or null. The screen asks it when it opens, so
// that a job started before the page was closed or reloaded shows up again.
func (h *ImportJobHandler) Current(c *fiber.Ctx) error {
	job, err := h.jobs.Running(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, fiber.Map{"job": job})
}

// Get handles GET /import-jobs/:id: the job as it is now, with the answer of every row.
func (h *ImportJobHandler) Get(c *fiber.Ctx) error {
	job, err := h.jobs.Find(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, job)
}

// Stop handles PUT /import-jobs/:id/stop: the worker stops after the row it is working on.
func (h *ImportJobHandler) Stop(c *fiber.Ctx) error {
	found, err := h.jobs.RequestStop(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	if !found {
		return domain.Conflict("Đợt nhập này đã kết thúc, không có gì để dừng.")
	}
	a := middleware.ActorOf(c)
	h.audit.Log(c.UserContext(), a.ID, a.Email, "import_job.stop", "import_job", c.Params("id"), "", a.IP)
	return httpx.OK(c, fiber.Map{"stopping": true})
}
