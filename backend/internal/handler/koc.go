package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// KocHandler serves /kocs routes.
type KocHandler struct {
	kocs     *service.KocService
	outreach *service.OutreachService
}

// NewKocHandler builds a KocHandler.
func NewKocHandler(kocs *service.KocService, outreach *service.OutreachService) *KocHandler {
	return &KocHandler{kocs: kocs, outreach: outreach}
}

type createKocRequest struct {
	TiktokID        string   `json:"tiktokId" validate:"required,min=2,max=100"`
	DisplayName     string   `json:"displayName" validate:"required,min=1,max=120"`
	AvatarURL       string   `json:"avatarUrl" validate:"max=1000"`
	Followers       *int64   `json:"followers" validate:"omitempty,gte=0"`
	Gmv             *float64 `json:"gmv" validate:"omitempty,gte=0"`
	Category        string   `json:"category" validate:"max=80"`
	ContentPillar   string   `json:"contentPillar" validate:"max=150"`
	TargetAudience  string   `json:"targetAudience" validate:"max=150"`
	Source          string   `json:"source" validate:"omitempty,oneof='TikToday' 'TikTok Liên kết' 'TikTok'"`
	Email           string   `json:"email" validate:"omitempty,email"`
	Phone           string   `json:"phone" validate:"max=30"`
	Recipient       string   `json:"recipient" validate:"max=120"`
	ShippingAddress string   `json:"shippingAddress" validate:"max=300"`
	DaHopTac        *bool    `json:"daHopTac"`
	Campaign        string   `json:"campaign" validate:"max=120"`
	ProductName     string   `json:"productName" validate:"max=150"`
	CommissionRate  *float64 `json:"commissionRate" validate:"omitempty,gte=0,lte=100"`
	CastFee         *float64 `json:"castFee" validate:"omitempty,gte=0"`
	Stage           string   `json:"stage" validate:"omitempty,oneof=SOURCED QUALIFIED AWAITING_APPROVAL FILTERED_OUT CONTACTED NO_REPLY REPLIED DORMANT AGREED DECLINED SHIPPING DELIVERED VIDEO_POSTED NOT_FIT RETURNING RETURNED STALLED COMPLETED"`
}

type bulkItemRequest struct {
	TiktokID    string   `json:"tiktokId" validate:"required,min=2,max=100"`
	DisplayName string   `json:"displayName" validate:"max=120"`
	Phone       string   `json:"phone" validate:"max=30"`
	Source      string   `json:"source" validate:"omitempty,oneof='TikToday' 'TikTok Liên kết' 'TikTok'"`
	Category    string   `json:"category" validate:"max=80"`
	Followers   *float64 `json:"followers" validate:"omitempty,gte=0"`
	Gmv         *float64 `json:"gmv" validate:"omitempty,gte=0"`
	CastFee     *float64 `json:"castFee" validate:"omitempty,gte=0"`
}

type bulkRequest struct {
	Source string            `json:"source" validate:"omitempty,oneof='TikToday' 'TikTok Liên kết' 'TikTok'"`
	Items  []bulkItemRequest `json:"items" validate:"required,min=1,dive"`
}

type outreachRequest struct {
	Mode             string `json:"mode" validate:"required,oneof=invite_all_qualified sweep_followups"`
	SimulateNextStep bool   `json:"simulateNextStep"`
	InviteBrief      string `json:"inviteBrief" validate:"max=1000"`
}

type lookupRequest struct {
	TiktokID string `json:"tiktokId" validate:"required,min=2,max=100"`
	Phone    string `json:"phone" validate:"max=30"`
}

type emailRequest struct {
	Text string `json:"text" validate:"max=4000"`
}

// List handles GET /kocs.
func (h *KocHandler) List(c *fiber.Ctx) error {
	page, _ := strconv.ParseInt(c.Query("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(c.Query("limit", "50"), 10, 64)
	res, err := h.kocs.List(c.UserContext(), service.ListFilter{
		Q: c.Query("q"), Category: c.Query("category"), Stage: c.Query("stage"), Source: c.Query("source"),
		Page: page, Limit: limit,
	})
	if err != nil {
		return err
	}
	return httpx.OK(c, res)
}

// Lookup handles POST /kocs/lookup.
func (h *KocHandler) Lookup(c *fiber.Ctx) error {
	var req lookupRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	res, err := h.kocs.Lookup(c.UserContext(), req.TiktokID, req.Phone)
	if err != nil {
		return err
	}
	if res == nil {
		return httpx.Fail(c, fiber.StatusNotFound, "Chưa tìm thấy kênh trên TikTok hoặc trong hệ thống. Vẫn có thể thêm thủ công vào danh bạ.")
	}
	return httpx.OK(c, res)
}

// Create handles POST /kocs.
func (h *KocHandler) Create(c *fiber.Ctx) error {
	var req createKocRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	koc, err := h.kocs.Create(c.UserContext(), service.CreateInput{
		TiktokID: req.TiktokID, DisplayName: req.DisplayName, AvatarURL: req.AvatarURL, Category: req.Category,
		ContentPillar: req.ContentPillar, TargetAudience: req.TargetAudience, Source: req.Source, Email: req.Email,
		Phone: req.Phone, Recipient: req.Recipient, ShippingAddress: req.ShippingAddress, Campaign: req.Campaign,
		ProductName: req.ProductName, Stage: req.Stage, Followers: req.Followers, Gmv: req.Gmv,
		CommissionRate: req.CommissionRate, CastFee: req.CastFee, DaHopTac: req.DaHopTac,
	})
	if err != nil {
		return err
	}
	return httpx.Created(c, koc)
}

// BulkImport handles POST /kocs/bulk.
func (h *KocHandler) BulkImport(c *fiber.Ctx) error {
	var req bulkRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	items := make([]service.BulkItem, len(req.Items))
	for i, it := range req.Items {
		items[i] = service.BulkItem{
			TiktokID: it.TiktokID, DisplayName: it.DisplayName, Phone: it.Phone, Source: it.Source,
			Category: it.Category, Followers: it.Followers, Gmv: it.Gmv, CastFee: it.CastFee,
		}
	}
	report, err := h.kocs.BulkImport(c.UserContext(), items, defaultStr(req.Source, "TikToday"))
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true, "data": report,
		"message": fmt.Sprintf("Đã nhập %d KOC, bỏ qua %d trùng lặp, chặn %d KOC đã hợp tác.",
			len(report.Imported), len(report.SkippedDuplicates), len(report.BlockedCollaborated)),
	})
}

// UpdateStage handles PATCH /kocs/:id/stage.
func (h *KocHandler) UpdateStage(c *fiber.Ctx) error {
	var req service.StageUpdate
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	koc, err := h.kocs.FindActive(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	updated, err := h.kocs.ApplyStageUpdate(c.UserContext(), koc, req)
	if err != nil {
		return err
	}
	return httpx.OK(c, updated)
}

// SyncChannel handles POST /kocs/:id/sync-channel.
func (h *KocHandler) SyncChannel(c *fiber.Ctx) error {
	koc, err := h.kocs.FindActive(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	var body struct {
		AutoDetectCollabVideo bool `json:"autoDetectCollabVideo"`
	}
	_ = c.BodyParser(&body)
	res, err := h.outreach.SyncKocTikTokChannel(c.UserContext(), koc, body.AutoDetectCollabVideo)
	if err != nil {
		return err
	}
	message := fmt.Sprintf("Không lấy được video công khai từ kênh @%s.", res.Koc.TiktokID)
	if res.ChannelData.Found {
		message = fmt.Sprintf("Đã đồng bộ kênh thật @%s: %s follower · %d video mới nhất.",
			res.Koc.TiktokID, service.FormatVND(res.Koc.Followers), len(res.Koc.Videos))
	}
	return c.JSON(fiber.Map{"success": true, "data": res.Koc, "channelData": res.ChannelData, "message": message})
}

// TrackShipment handles POST /kocs/:id/track-shipment.
func (h *KocHandler) TrackShipment(c *fiber.Ctx) error {
	koc, err := h.kocs.FindActive(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	var body struct {
		TrackingCode string `json:"trackingCode"`
	}
	_ = c.BodyParser(&body)
	if koc.TrackingCode == "" && body.TrackingCode == "" {
		return httpx.NewError(400, "KOC chưa có mã vận đơn J&T Express để tra cứu")
	}
	if body.TrackingCode != "" {
		koc.TrackingCode = strings.TrimSpace(body.TrackingCode)
	}
	res, err := h.outreach.SyncKocShipment(c.UserContext(), koc)
	if err != nil {
		return err
	}
	message := "Đã cập nhật trạng thái vận đơn J&T Express"
	if res.Tracking != nil && res.Tracking.StatusText != "" {
		message = res.Tracking.StatusText
	}
	return c.JSON(fiber.Map{"success": true, "data": res.Koc, "tracking": res.Tracking, "message": message})
}

// LookupJtTracking handles POST /kocs/track-shipment/lookup.
func (h *KocHandler) LookupJtTracking(c *fiber.Ctx) error {
	var body struct {
		TrackingCode string `json:"trackingCode"`
	}
	_ = c.BodyParser(&body)
	code := strings.TrimSpace(body.TrackingCode)
	if code == "" {
		return httpx.NewError(400, "Vui lòng nhập mã vận đơn J&T Express")
	}
	tracking, err := h.outreach.QueryTracking(c.UserContext(), code)
	if err != nil {
		return err
	}
	message := tracking.StatusText
	if message == "" {
		message = "Đã tra cứu vận đơn J&T #" + code
	}
	return httpx.OKMessage(c, tracking, message)
}

// LookupVideo handles POST /kocs/video/lookup.
func (h *KocHandler) LookupVideo(c *fiber.Ctx) error {
	var body struct {
		VideoURL string `json:"videoUrl"`
	}
	_ = c.BodyParser(&body)
	url := strings.TrimSpace(body.VideoURL)
	if url == "" {
		return httpx.NewError(400, "Vui lòng nhập đường dẫn video TikTok")
	}
	metrics, err := h.outreach.FetchVideoMetrics(c.UserContext(), url)
	if err != nil {
		return err
	}
	message := "Đã ghi nhận đường dẫn video TikTok"
	if metrics.VideoID != "" {
		author := defaultStr(metrics.AuthorUniqueID, "koc")
		message = fmt.Sprintf("Đã xác thực video TikTok thật (ID: %s) của @%s", metrics.VideoID, author)
	}
	return httpx.OKMessage(c, metrics, message)
}

// RunOutreach handles POST /kocs/outreach/run.
func (h *KocHandler) RunOutreach(c *fiber.Ctx) error {
	var req outreachRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	ctx := c.UserContext()
	if req.Mode == "invite_all_qualified" {
		items, err := h.kocs.InviteAllQualified(ctx, req.InviteBrief)
		if err != nil {
			return err
		}
		return httpx.OKMessage(c, fiber.Map{"count": len(items), "items": items},
			fmt.Sprintf("Đã gửi lời mời lần 1 (Zalo + Brief) cho %d KOC đạt phễu.", len(items)))
	}
	summary, err := h.outreach.RunStage2Sweep(ctx, req.SimulateNextStep)
	if err != nil {
		return err
	}
	digest, err := h.outreach.BuildMorningDigest(ctx)
	if err != nil {
		return err
	}
	items, err := h.kocs.RecentActive(ctx)
	if err != nil {
		return err
	}
	return httpx.OKMessage(c, fiber.Map{"summary": summary, "digest": digest, "items": items},
		fmt.Sprintf("Quét tự động hoàn tất: Nhắc Email (+2N): %d · Nhắc Zalo lối thoát (+4N): %d · Task gọi (+7N): %d · Vận đơn J&T: %d · Quét TikTok: %d · Nhắc video: %d · Cờ đỏ STALLED: %d.",
			summary.EmailReminded, summary.ZaloReminded, summary.CallTasksCreated, summary.ShipmentsChecked,
			summary.TiktokDailyScanned, summary.VideoReminded, summary.StalledFlagged))
}

// MorningDigest handles GET /kocs/digest/morning.
func (h *KocHandler) MorningDigest(c *fiber.Ctx) error {
	digest, err := h.outreach.BuildMorningDigest(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, digest)
}

// Archive handles DELETE /kocs/:id.
func (h *KocHandler) Archive(c *fiber.Ctx) error {
	koc, err := h.kocs.Archive(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, koc)
}

// Email handles POST /kocs/:id/email.
func (h *KocHandler) Email(c *fiber.Ctx) error {
	var req emailRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	koc, err := h.kocs.FindActive(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	if koc.Email == "" {
		return httpx.NewError(422, "KOC chưa có email để gửi")
	}
	if err := h.outreach.SendEmail(koc, req.Text); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "Đã gửi email cho KOC"})
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
