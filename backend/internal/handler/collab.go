package handler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/repository"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// OpenCollabLookup resolves a TikTok Shop product to its open collaboration (public commission). Satisfied by
// *service.TTShopService.
type OpenCollabLookup interface {
	OpenCollabByProduct(ctx context.Context, productID string) (*service.OpenCollabProduct, error)
}

// CollabHandler serves the "lần hợp tác" routes of the inbox right panel.
type CollabHandler struct {
	kocs    *repository.KocRepository
	collabs *repository.CollabRepository
	convs   *repository.ConversationRepository
	msgs    *repository.MessageRepository
	open    OpenCollabLookup
}

// NewCollabHandler builds a CollabHandler.
func NewCollabHandler(kocs *repository.KocRepository, collabs *repository.CollabRepository,
	convs *repository.ConversationRepository, msgs *repository.MessageRepository) *CollabHandler {
	return &CollabHandler{kocs: kocs, collabs: collabs, convs: convs, msgs: msgs}
}

// WithOpenCollabs enables the open-collaboration mode by giving it a TikTok Shop lookup.
func (h *CollabHandler) WithOpenCollabs(l OpenCollabLookup) *CollabHandler {
	h.open = l
	return h
}

const (
	collabNotFound    = "Không tìm thấy lần hợp tác"
	collabClosedMsg   = "Lần hợp tác đã kết thúc, không thể chỉnh sửa."
	kocNotFoundCollab = "Không tìm thấy KOC đang hoạt động"
)

func vnd(n float64) string { return service.FormatVND(n) + "đ" }

// note writes a system line ("Chốt phí cast 550.000đ") into the KOC thread.
func (h *CollabHandler) note(ctx context.Context, koc *domain.Koc, text string) {
	conv, err := h.convs.FindByHandle(ctx, "@"+koc.TiktokID)
	if err != nil || conv == nil {
		return
	}
	_ = h.msgs.Create(ctx, &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionOutbound,
		Kind: "system", Text: text, Status: domain.MessageDelivered})
}

// sync mirrors the collaboration onto the KOC so dashboards and next-task tags stay correct.
func (h *CollabHandler) sync(ctx context.Context, koc *domain.Koc, c *domain.Collab) error {
	if c.Status == domain.CollabActive {
		koc.CastFee, koc.CollabStep = c.CastFee, c.Step
		if c.ProductName != "" {
			koc.ProductName = c.ProductName
		}
		if c.CommissionRate > 0 {
			koc.CommissionRate = c.CommissionRate
		}
	}
	return h.kocs.Save(ctx, koc)
}

func (h *CollabHandler) load(c *fiber.Ctx) (*domain.Collab, *domain.Koc, error) {
	ctx := c.UserContext()
	col, err := h.collabs.FindByID(ctx, c.Params("cid"))
	if err != nil {
		return nil, nil, err
	}
	if col == nil {
		return nil, nil, httpx.NewError(fiber.StatusNotFound, collabNotFound)
	}
	koc, err := h.kocs.FindActiveByID(ctx, col.KocID.Hex())
	if err != nil {
		return nil, nil, err
	}
	if koc == nil {
		return nil, nil, httpx.NewError(fiber.StatusNotFound, kocNotFoundCollab)
	}
	return col, koc, nil
}

func (h *CollabHandler) loadActive(c *fiber.Ctx) (*domain.Collab, *domain.Koc, error) {
	col, koc, err := h.load(c)
	if err != nil {
		return nil, nil, err
	}
	if col.Status == domain.CollabClosed {
		return nil, nil, httpx.NewError(fiber.StatusConflict, collabClosedMsg)
	}
	return col, koc, nil
}

// List handles GET /kocs/:id/collabs.
func (h *CollabHandler) List(c *fiber.Ctx) error {
	ctx := c.UserContext()
	koc, err := h.kocs.FindActiveByID(ctx, c.Params("id"))
	if err != nil {
		return err
	}
	if koc == nil {
		return httpx.NewError(fiber.StatusNotFound, kocNotFoundCollab)
	}
	items, err := h.collabs.ListByKoc(ctx, koc.ID)
	if err != nil {
		return err
	}
	owed := 0.0
	for i := range items {
		owed += items[i].Owed()
	}
	return httpx.OK(c, fiber.Map{"items": items, "owed": owed})
}

// Open handles POST /kocs/:id/collabs (Chốt phí cast opens a new collaboration).
func (h *CollabHandler) Open(c *fiber.Ctx) error {
	var req struct {
		CastFee float64 `json:"castFee" validate:"gte=0,lte=1000000000"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	ctx := c.UserContext()
	koc, err := h.kocs.FindActiveByID(ctx, c.Params("id"))
	if err != nil {
		return err
	}
	if koc == nil {
		return httpx.NewError(fiber.StatusNotFound, kocNotFoundCollab)
	}
	if cur, err := h.collabs.Active(ctx, koc.ID); err != nil {
		return err
	} else if cur != nil {
		return httpx.NewError(fiber.StatusConflict, "KOC này đang có một lần hợp tác chưa kết thúc.")
	}
	col := &domain.Collab{KocID: koc.ID, FeeSet: true, CastFee: req.CastFee, CommissionRate: koc.CommissionRate,
		ProductName: koc.ProductName, FeeHistory: []domain.FeeChange{{Fee: req.CastFee, At: time.Now()}}}
	if err := h.collabs.Create(ctx, col); err != nil {
		return err
	}
	if err := h.sync(ctx, koc, col); err != nil {
		return err
	}
	if req.CastFee > 0 {
		h.note(ctx, koc, "Chốt phí cast "+vnd(req.CastFee))
	} else {
		h.note(ctx, koc, "Chốt Free cast (0đ)")
	}
	return httpx.Created(c, col)
}

type collabPatch struct {
	Name           *string  `json:"name" validate:"omitempty,max=80"`
	CastFee        *float64 `json:"castFee" validate:"omitempty,gte=0,lte=1000000000"`
	FeePaid        *bool    `json:"feePaid"`
	Mode           *string  `json:"mode" validate:"omitempty,oneof=targeted open product"`
	ProductName    *string  `json:"productName" validate:"omitempty,max=200"`
	ProductID      *string  `json:"productId" validate:"omitempty,max=40"`
	ProductLink    *string  `json:"productLink" validate:"omitempty,max=500"`
	CommissionRate *float64 `json:"commissionRate" validate:"omitempty,gte=0,lte=100"`
	Deadline       *string  `json:"deadline" validate:"omitempty,max=20"`
	FreeSample     *bool    `json:"freeSample"`
	ShareOrders    *float64 `json:"shareOrders" validate:"omitempty,gte=0"`
	ShareRevenue   *float64 `json:"shareRevenue" validate:"omitempty,gte=0"`
}

// Patch handles PATCH /collabs/:cid.
func (h *CollabHandler) Patch(c *fiber.Ctx) error {
	var req collabPatch
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	col, koc, err := h.load(c)
	if err != nil {
		return err
	}
	ctx := c.UserContext()
	// After closing, only the paid flag may change ("Đánh dấu đã trả phí").
	if col.Status == domain.CollabClosed {
		if req.FeePaid == nil {
			return httpx.NewError(fiber.StatusConflict, collabClosedMsg)
		}
		col.FeePaid = *req.FeePaid
		if err := h.collabs.Save(ctx, col); err != nil {
			return err
		}
		h.note(ctx, koc, "Đã trả phí cast "+vnd(col.CastFee))
		return httpx.OK(c, col)
	}

	if req.Name != nil {
		col.Name = strings.TrimSpace(*req.Name)
	}
	if req.CastFee != nil && (!col.FeeSet || *req.CastFee != col.CastFee) {
		col.CastFee, col.FeeSet = *req.CastFee, true
		col.FeeHistory = append(col.FeeHistory, domain.FeeChange{Fee: col.CastFee, At: time.Now()})
		h.note(ctx, koc, "Đổi phí cast thành "+vnd(col.CastFee))
	}
	if req.FeePaid != nil && *req.FeePaid != col.FeePaid {
		col.FeePaid = *req.FeePaid
		if col.FeePaid {
			h.note(ctx, koc, "Đã trả phí cast "+vnd(col.CastFee))
		}
	}
	settingMode, modeChosen := false, false
	if req.Mode != nil && *req.Mode != col.Mode {
		settingMode = *req.Mode == domain.ModeOpen
		if col.Mode != "" {
			return httpx.NewError(fiber.StatusConflict, "Không đổi được hình thức hợp tác. Hãy kết thúc lần này và mở lần mới.")
		}
		col.Mode = *req.Mode
		modeChosen = true
	}
	lockedByShop := col.Mode == domain.ModeOpen && col.ProductID != "" // name and public rate come from TikTok Shop
	if req.ProductName != nil && !lockedByShop {
		col.ProductName = strings.TrimSpace(*req.ProductName)
	}
	if req.ProductLink != nil {
		col.ProductLink = strings.TrimSpace(*req.ProductLink)
	}
	if req.CommissionRate != nil && !lockedByShop {
		col.CommissionRate = *req.CommissionRate
	}
	if err := h.applyOpenProduct(ctx, col, req.ProductID, settingMode); err != nil {
		return err
	}
	if modeChosen { // after validation, so a rejected request leaves no note in the thread
		h.note(ctx, koc, "Chọn hình thức: "+modeLabel(col.Mode))
	}
	if req.Deadline != nil {
		col.Deadline = *req.Deadline
	}
	if req.FreeSample != nil {
		col.FreeSample = *req.FreeSample
	}
	if req.ShareOrders != nil {
		col.ShareOrders = *req.ShareOrders
	}
	if req.ShareRevenue != nil {
		col.ShareRevenue = *req.ShareRevenue
	}
	return h.saveAndSync(c, col, koc)
}

// applyOpenProduct handles the open-collaboration mode: the product and its public commission always come from
// TikTok Shop (never from the browser), and an open collaboration needs a product that is in effect.
func (h *CollabHandler) applyOpenProduct(ctx context.Context, col *domain.Collab, productID *string, settingMode bool) error {
	if col.Mode != domain.ModeOpen {
		if productID != nil && *productID != "" {
			return httpx.NewError(fiber.StatusBadRequest, "Chỉ chọn sản phẩm khi hình thức là cộng tác mở.")
		}
		return nil
	}
	if productID == nil || *productID == "" || *productID == col.ProductID {
		if col.ProductID == "" && settingMode {
			return httpx.NewError(fiber.StatusBadRequest, "Hãy chọn một sản phẩm đang mở cộng tác.")
		}
		return nil
	}
	if h.open == nil {
		return httpx.NewError(fiber.StatusServiceUnavailable, "Chưa kết nối TikTok Shop.")
	}
	p, err := h.open.OpenCollabByProduct(ctx, *productID)
	switch {
	case errors.Is(err, service.ErrOpenCollabNotFound):
		return httpx.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrTTShopNotConnected):
		return httpx.NewError(fiber.StatusConflict, err.Error())
	case err != nil:
		return httpx.NewError(fiber.StatusBadGateway, err.Error())
	}
	if p.Status != service.OpenCollabNormal {
		return httpx.NewError(fiber.StatusConflict, "Cộng tác mở của sản phẩm này sắp hết hiệu lực, hãy chọn sản phẩm khác.")
	}
	col.ProductID, col.ProductName, col.CommissionRate = p.ProductID, p.Title, p.CommissionRate
	return nil
}

func modeLabel(m string) string {
	switch m {
	case domain.ModeTargeted:
		return "Mời cộng tác mục tiêu"
	case domain.ModeOpen:
		return "Tạo link cộng tác mở"
	default:
		return "Dán link sản phẩm"
	}
}

func (h *CollabHandler) saveAndSync(c *fiber.Ctx, col *domain.Collab, koc *domain.Koc) error {
	ctx := c.UserContext()
	if err := h.collabs.Save(ctx, col); err != nil {
		return err
	}
	if err := h.sync(ctx, koc, col); err != nil {
		return err
	}
	return httpx.OK(c, col)
}

// AddSample handles POST /collabs/:cid/samples (a sample request arriving from the KOC).
func (h *CollabHandler) AddSample(c *fiber.Ctx) error {
	var req struct {
		Product string `json:"product" validate:"required,max=200"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	col, koc, err := h.loadActive(c)
	if err != nil {
		return err
	}
	col.Samples = append(col.Samples, domain.SampleRequest{ID: primitive.NewObjectID().Hex(), Product: req.Product,
		Status: domain.SamplePending, At: time.Now()})
	h.note(c.UserContext(), koc, "KOC xin mẫu: "+req.Product)
	return h.saveAndSync(c, col, koc)
}

func (h *CollabHandler) decideSample(approve bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			Reason string `json:"reason" validate:"max=200"`
		}
		if err := httpx.BindAndValidate(c, &req); err != nil {
			return err
		}
		if !approve && strings.TrimSpace(req.Reason) == "" {
			return httpx.NewError(fiber.StatusBadRequest, "Vui lòng chọn lý do từ chối.")
		}
		col, koc, err := h.loadActive(c)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(col.Samples, func(s domain.SampleRequest) bool { return s.ID == c.Params("sid") })
		if i < 0 {
			return httpx.NewError(fiber.StatusNotFound, "Không tìm thấy yêu cầu mẫu")
		}
		if col.Samples[i].Status != domain.SamplePending {
			return httpx.NewError(fiber.StatusConflict, "Yêu cầu mẫu này đã được xử lý.")
		}
		if approve {
			col.Samples[i].Status = domain.SampleApproved
			h.note(c.UserContext(), koc, "Đã duyệt mẫu: "+col.Samples[i].Product)
		} else {
			col.Samples[i].Status, col.Samples[i].RejectReason = domain.SampleRejected, strings.TrimSpace(req.Reason)
			h.note(c.UserContext(), koc, "Từ chối mẫu ("+col.Samples[i].RejectReason+")")
		}
		return h.saveAndSync(c, col, koc)
	}
}

// Approve / Reject handle POST /collabs/:cid/samples/:sid/(approve|reject).
func (h *CollabHandler) Approve() fiber.Handler { return h.decideSample(true) }
func (h *CollabHandler) Reject() fiber.Handler  { return h.decideSample(false) }

// AddVideo handles POST /collabs/:cid/videos.
func (h *CollabHandler) AddVideo(c *fiber.Ctx) error {
	var req struct {
		URL     string  `json:"url" validate:"required,max=500"`
		Title   string  `json:"title" validate:"max=300"`
		Views   float64 `json:"views" validate:"gte=0"`
		Orders  float64 `json:"orders" validate:"gte=0"`
		Revenue float64 `json:"revenue" validate:"gte=0"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	col, koc, err := h.loadActive(c)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(col.Videos, func(v domain.CollabVideo) bool { return v.URL == req.URL }) {
		return httpx.NewError(fiber.StatusConflict, "Video này đã được thêm.")
	}
	col.Videos = append(col.Videos, domain.CollabVideo{URL: req.URL, Title: req.Title, Views: req.Views, Orders: req.Orders, Revenue: req.Revenue})
	h.note(c.UserContext(), koc, "KOC đăng bài: "+req.URL)
	return h.saveAndSync(c, col, koc)
}

// SetAdsCode handles PUT /collabs/:cid/videos/:idx/ads.
func (h *CollabHandler) SetAdsCode(c *fiber.Ctx) error {
	var req struct {
		Code string `json:"code" validate:"max=200"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	col, koc, err := h.loadActive(c)
	if err != nil {
		return err
	}
	idx, err := strconv.Atoi(c.Params("idx"))
	if err != nil || idx < 0 || idx >= len(col.Videos) {
		return httpx.NewError(fiber.StatusNotFound, "Không tìm thấy video")
	}
	col.Videos[idx].AdsCode = strings.TrimSpace(req.Code)
	return h.saveAndSync(c, col, koc)
}

// Close handles POST /collabs/:cid/close.
func (h *CollabHandler) Close(c *fiber.Ctx) error {
	var req struct {
		Outcome string `json:"outcome" validate:"required,oneof=posted no_post no_response no_need"`
		Paid    bool   `json:"paid"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	col, koc, err := h.loadActive(c)
	if err != nil {
		return err
	}
	now := time.Now()
	col.Status, col.Outcome, col.EndedAt = domain.CollabClosed, req.Outcome, &now
	if req.Outcome == domain.OutcomePosted && req.Paid {
		col.FeePaid = true
	}
	if col.CastFee == 0 {
		col.FeePaid = true
	}
	ctx := c.UserContext()
	if err := h.collabs.Save(ctx, col); err != nil {
		return err
	}
	koc.CollabStep = 0
	if err := h.kocs.Save(ctx, koc); err != nil {
		return err
	}
	h.note(ctx, koc, fmt.Sprintf("Kết thúc lần hợp tác thứ %d: %s", col.Seq, outcomeLabel(col.Outcome)))
	return httpx.OK(c, col)
}

// SetOutcome handles PATCH /collabs/:cid/outcome ("Sửa kết cục").
func (h *CollabHandler) SetOutcome(c *fiber.Ctx) error {
	var req struct {
		Outcome string `json:"outcome" validate:"required,oneof=posted no_post no_response no_need"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	col, koc, err := h.load(c)
	if err != nil {
		return err
	}
	if col.Status != domain.CollabClosed {
		return httpx.NewError(fiber.StatusConflict, "Chỉ sửa được kết cục của lần hợp tác đã kết thúc.")
	}
	col.Outcome = req.Outcome
	ctx := c.UserContext()
	if err := h.collabs.Save(ctx, col); err != nil {
		return err
	}
	h.note(ctx, koc, "Sửa kết cục lần "+strconv.Itoa(col.Seq)+": "+outcomeLabel(col.Outcome))
	return httpx.OK(c, col)
}

func outcomeLabel(o string) string {
	switch o {
	case domain.OutcomePosted:
		return "KOC đã đăng bài"
	case domain.OutcomeNoPost:
		return "Nhận mẫu rồi không đăng"
	case domain.OutcomeNoResponse:
		return "Không nhận mẫu / không phản hồi"
	default:
		return "Hết nhu cầu hợp tác"
	}
}
