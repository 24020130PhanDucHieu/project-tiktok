package handler

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg" // registers the decoders image.DecodeConfig needs
	_ "image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
)

// CampaignStore is what the campaign routes need from the store (*repository.CampaignRepository).
type CampaignStore interface {
	List(ctx context.Context) ([]domain.Campaign, error)
	SaveSteps(ctx context.Context, id string, steps []domain.CampaignStep, at time.Time) error
	AddImage(ctx context.Context, img *domain.CampaignImage) error
	Image(ctx context.Context, id string) (*domain.CampaignImage, error)
	ImagesExist(ctx context.Context, ids []domain.ID) (bool, error)
}

// CampaignHandler serves the campaigns and their pictures. Everybody signed in reads; only an admin changes them.
type CampaignHandler struct {
	store CampaignStore
	audit Auditor
}

// NewCampaignHandler builds the handler.
func NewCampaignHandler(store CampaignStore, audit Auditor) *CampaignHandler {
	return &CampaignHandler{store: store, audit: audit}
}

// List handles GET /campaigns.
func (h *CampaignHandler) List(c *fiber.Ctx) error {
	list, err := h.store.List(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, list)
}

// ImageIDs are the pictures a list of steps points at.
func ImageIDs(steps []domain.CampaignStep) []domain.ID {
	var ids []domain.ID
	for _, s := range steps {
		if s.Kind == domain.StepImage {
			ids = append(ids, s.ImageID)
		}
	}
	return ids
}

// CheckSteps validates the steps and that every picture they use is stored.
func CheckSteps(ctx context.Context, store interface {
	ImagesExist(context.Context, []domain.ID) (bool, error)
}, steps []domain.CampaignStep) error {
	if err := domain.ValidateSteps(steps); err != nil {
		return err
	}
	ok, err := store.ImagesExist(ctx, ImageIDs(steps))
	if err != nil {
		return err
	}
	if !ok {
		return domain.Invalid("Có tin dùng ảnh không còn tồn tại. Hãy tải ảnh lại.")
	}
	return nil
}

// SaveSteps handles PUT /campaigns/:id with {steps}.
func (h *CampaignHandler) SaveSteps(c *fiber.Ctx) error {
	var in struct {
		Steps []domain.CampaignStep `json:"steps"`
	}
	if err := c.BodyParser(&in); err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Dữ liệu không hợp lệ")
	}
	ctx := c.UserContext()
	if err := CheckSteps(ctx, h.store, in.Steps); err != nil {
		return err
	}
	id := c.Params("id")
	if err := h.store.SaveSteps(ctx, id, in.Steps, time.Now()); err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	h.audit.Log(ctx, a.ID, a.Email, "campaign.update", "campaign", id, "", a.IP)
	return httpx.OK(c, fiber.Map{"saved": true})
}

// UploadImage handles POST /campaigns/images (multipart, field "file"): a real PNG or JPEG of at most 2 MB.
func (h *CampaignHandler) UploadImage(c *fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Hãy chọn một ảnh PNG hoặc JPG.")
	}
	if fh.Size > domain.MaxImageBytes {
		return httpx.Fail(c, fiber.StatusBadRequest, "Ảnh lớn quá, tối đa 2 MB.")
	}
	f, err := fh.Open()
	if err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Không đọc được ảnh.")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, domain.MaxImageBytes+1))
	if err != nil || len(data) > domain.MaxImageBytes {
		return httpx.Fail(c, fiber.StatusBadRequest, "Ảnh lớn quá, tối đa 2 MB.")
	}
	// the content decides, not the file name or the type the browser claims
	kind := http.DetectContentType(data)
	if kind != "image/png" && kind != "image/jpeg" {
		return httpx.Fail(c, fiber.StatusBadRequest, "Chỉ nhận ảnh PNG hoặc JPG.")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return httpx.Fail(c, fiber.StatusBadRequest, "Ảnh bị hỏng, hãy chọn ảnh khác.")
	}
	name := filepath.Base(strings.ReplaceAll(fh.Filename, "\\", "/"))
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	img := &domain.CampaignImage{Filename: name, ContentType: kind, Width: cfg.Width, Height: cfg.Height, Size: len(data), Data: data}
	if err := h.store.AddImage(c.UserContext(), img); err != nil {
		return err
	}
	a := middleware.ActorOf(c)
	h.audit.Log(c.UserContext(), a.ID, a.Email, "campaign.image_upload", "campaign_image", img.ID.Hex(), "", a.IP)
	return httpx.Created(c, fiber.Map{"_id": img.ID, "width": img.Width, "height": img.Height, "filename": img.Filename})
}

// Image handles GET /campaigns/images/:id and sends the picture itself.
func (h *CampaignHandler) Image(c *fiber.Ctx) error {
	img, err := h.store.Image(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, img.ContentType)
	c.Set(fiber.HeaderCacheControl, "private, max-age=86400")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.Send(img.Data)
}
