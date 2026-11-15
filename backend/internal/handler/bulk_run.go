package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
)

// RunStarter begins a bulk run (*service.CampaignRunner).
type RunStarter interface {
	Start(ctx context.Context, by, campaignName string, steps []domain.CampaignStep, targets []domain.RunTarget) (*domain.BulkRun, error)
	Estimate(kocs, steps int) time.Duration
}

// RunReader reads the runs (*repository.BulkRunRepository).
type RunReader interface {
	Running(ctx context.Context) (*domain.BulkRun, error)
	Recent(ctx context.Context, limit int64) ([]domain.BulkRun, error)
}

// LimitStore keeps how many KOCs a day may be started (*repository.BulkLimitRepository).
type LimitStore interface {
	Get(ctx context.Context) int
	Set(ctx context.Context, n int) error
}

// RunKocs finds the active KOC of an id (*service.KocService); a missing one is a NotFound fault.
type RunKocs interface {
	FindActive(ctx context.Context, id string) (*domain.Koc, error)
}

// ImageChecker says whether pictures are stored (*repository.CampaignRepository).
type ImageChecker interface {
	ImagesExist(ctx context.Context, ids []domain.ID) (bool, error)
}

// BulkRunHandler serves the bulk runs: start one, follow it, and the daily limit.
type BulkRunHandler struct {
	starter RunStarter
	runs    RunReader
	limit   LimitStore
	kocs    RunKocs
	images  ImageChecker
	audit   Auditor
}

// NewBulkRunHandler builds the handler.
func NewBulkRunHandler(starter RunStarter, runs RunReader, limit LimitStore, kocs RunKocs, images ImageChecker, audit Auditor) *BulkRunHandler {
	return &BulkRunHandler{starter: starter, runs: runs, limit: limit, kocs: kocs, images: images, audit: audit}
}

type startRunRequest struct {
	CampaignName string                `json:"campaignName" validate:"required,max=120"`
	Steps        []domain.CampaignStep `json:"steps" validate:"required,min=1,max=12"`
	KocIDs       []string              `json:"kocIds" validate:"required,min=1,max=50,dive,required"`
	Confirm      bool                  `json:"confirm"`
}

// Start handles POST /bulk-runs. It reaches real people, so it needs confirm=true. A KOC with no active record stays in
// the run and is reported as not found; a repeated id is sent once.
func (h *BulkRunHandler) Start(c *fiber.Ctx) error {
	var req startRunRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	if !req.Confirm {
		return httpx.Fail(c, fiber.StatusBadRequest, "Cần xác nhận trước khi gửi hàng loạt.")
	}
	ctx := c.UserContext()
	if err := CheckSteps(ctx, h.images, req.Steps); err != nil {
		return err
	}

	seen := map[string]bool{}
	targets := make([]domain.RunTarget, 0, len(req.KocIDs))
	for _, id := range req.KocIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		t := domain.RunTarget{KocID: id}
		koc, err := h.kocs.FindActive(ctx, id)
		switch {
		case err == nil && koc != nil:
			t.KocName, t.Handle = koc.DisplayName, "@"+koc.TiktokID
		case err == nil || isNotFound(err):
			// stays in the run; the runner reports it as not found
		default:
			return err
		}
		targets = append(targets, t)
	}

	a := middleware.ActorOf(c)
	run, err := h.starter.Start(ctx, a.Email, req.CampaignName, req.Steps, targets)
	if err != nil {
		return err
	}
	h.audit.Log(ctx, a.ID, a.Email, "bulk_run.start", "bulk_run", run.ID.Hex(), fmt.Sprintf("%d KOC, %d tin", len(targets), len(req.Steps)), a.IP)
	minutes := int(h.starter.Estimate(len(targets), len(req.Steps)).Minutes())
	return httpx.Created(c, fiber.Map{"run": run, "estimateMinutes": minutes})
}

func isNotFound(err error) bool {
	var f *domain.Fault
	return errors.As(err, &f) && f.Kind == domain.FaultNotFound
}

// Current handles GET /bulk-runs/current: the run that is running, or null.
func (h *BulkRunHandler) Current(c *fiber.Ctx) error {
	run, err := h.runs.Running(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, fiber.Map{"run": run})
}

// List handles GET /bulk-runs?limit=: the latest runs, newest first.
func (h *BulkRunHandler) List(c *fiber.Ctx) error {
	limit := int64(c.QueryInt("limit", 20))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	runs, err := h.runs.Recent(c.UserContext(), limit)
	if err != nil {
		return err
	}
	return httpx.OK(c, runs)
}

// GetLimit handles GET /bulk-limit.
func (h *BulkRunHandler) GetLimit(c *fiber.Ctx) error {
	return httpx.OK(c, fiber.Map{"value": h.limit.Get(c.UserContext())})
}

// SetLimit handles PUT /bulk-limit with {value}: 10, 20, 30 or 50.
func (h *BulkRunHandler) SetLimit(c *fiber.Ctx) error {
	var in struct {
		Value int `json:"value"`
	}
	if err := c.BodyParser(&in); err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Dữ liệu không hợp lệ")
	}
	if err := h.limit.Set(c.UserContext(), in.Value); err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	h.audit.Log(c.UserContext(), a.ID, a.Email, "bulk_limit.set", "bulk_limit", "", fmt.Sprintf("%d", in.Value), a.IP)
	return httpx.OK(c, fiber.Map{"value": in.Value})
}
