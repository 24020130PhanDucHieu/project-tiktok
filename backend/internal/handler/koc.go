package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// KocHandler serves /kocs routes.
type KocHandler struct {
	kocs     KocAPI
	outreach OutreachAPI
	stats    DashboardReader
	audit    Auditor
	sendLog  SendLog // nil: bulk sends are not recorded
}

// KocAPI is the part of *service.KocService this handler uses.
type KocAPI interface {
	List(ctx context.Context, f service.ListFilter) (*service.ListResult, error)
	Lookup(ctx context.Context, tiktokID, phone string) (service.LookupResult, error)
	Create(ctx context.Context, in service.CreateInput) (*domain.Koc, error)
	BulkImport(ctx context.Context, items []service.BulkItem, defaultSource string) (*service.BulkReport, error)
	Archive(ctx context.Context, id string) (*domain.Koc, error)
	FindActive(ctx context.Context, id string) (*domain.Koc, error)
	InviteAllQualified(ctx context.Context, brief string) ([]*domain.Koc, error)
	RecentActive(ctx context.Context) ([]domain.Koc, error)
	ApplyStage(ctx context.Context, koc *domain.Koc, p service.StageUpdate) (*service.StageResult, error)
}

// OutreachAPI is the part of *service.OutreachService this handler uses.
type OutreachAPI interface {
	BuildMorningDigest(ctx context.Context) (*service.Digest, error)
	SyncKocTikTokChannel(ctx context.Context, koc *domain.Koc, autoDetectCollabVideo bool) (*service.ChannelSync, error)
	SyncKocShipment(ctx context.Context, koc *domain.Koc) (*service.ShipmentSync, error)
	SendEmail(koc *domain.Koc, text string) error
	QueryTracking(ctx context.Context, code, phoneLast4 string) (*service.JTTracking, error)
	FetchVideoMetrics(ctx context.Context, videoURL string) (*service.TikTokVideoMetrics, error)
	RunStage2Sweep(ctx context.Context, simulateNextStep bool) (*service.SweepSummary, error)
	SendTemplateBatch(ctx context.Context, body string, targets []service.BatchTarget) (*service.BatchReport, error)
}

// SendLog keeps what happened to every KOC of every bulk send (*repository.BulkSendLogRepository).
type SendLog interface {
	Record(ctx context.Context, rows []domain.BulkSendLog) error
	Recent(ctx context.Context, limit int64) ([]domain.BulkSendLog, error)
}

// WithSendLog gives the handler the log of bulk sends.
func (h *KocHandler) WithSendLog(l SendLog) *KocHandler {
	h.sendLog = l
	return h
}

// NewKocHandler builds a KocHandler.
func NewKocHandler(kocs KocAPI, outreach OutreachAPI, stats DashboardReader, audit Auditor) *KocHandler {
	return &KocHandler{kocs: kocs, outreach: outreach, stats: stats, audit: audit}
}

// Stats handles GET /kocs/stats (dashboard: stage counts, campaign funnels, today's tasks).
func (h *KocHandler) Stats(c *fiber.Ctx) error {
	d, err := h.stats.Dashboard(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, d)
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
	Email       string   `json:"email" validate:"omitempty,email"`
	Recipient   string   `json:"recipient" validate:"max=120"`
	Address     string   `json:"shippingAddress" validate:"max=300"`
	Followers   *float64 `json:"followers" validate:"omitempty,gte=0"`
	Gmv         *float64 `json:"gmv" validate:"omitempty,gte=0"`
	CastFee     *float64 `json:"castFee" validate:"omitempty,gte=0"`
}

// A request carries at most 20 rows: every row asks TikTok Shop once, so the screen sends small batches and shows
// progress instead of one long request.
type bulkRequest struct {
	Source string            `json:"source" validate:"omitempty,oneof='TikToday' 'TikTok Liên kết' 'TikTok'"`
	Items  []bulkItemRequest `json:"items" validate:"required,min=1,max=20,dive"`
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
		return httpx.Fail(c, fiber.StatusNotFound, "Chưa tìm thấy kênh trên TikTok hoặc trong hệ thống. Kiểm tra lại TikTok ID.")
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
			Category: it.Category, Email: it.Email, Recipient: it.Recipient, ShippingAddress: it.Address,
			Followers: it.Followers, Gmv: it.Gmv, CastFee: it.CastFee,
		}
	}
	report, err := h.kocs.BulkImport(c.UserContext(), items, defaultStr(req.Source, "TikToday"))
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true, "data": report,
		"message": fmt.Sprintf("Đã nhập %d KOC, bỏ qua %d trùng lặp, chặn %d KOC đã hợp tác, từ chối %d kênh không có trong Nhà sáng tạo của TikTok Shop.",
			len(report.Imported), len(report.SkippedDuplicates), len(report.BlockedCollaborated), len(report.NotVerified)),
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
	res, err := h.kocs.ApplyStage(c.UserContext(), koc, req)
	if err != nil {
		return err
	}
	// "draft" is the message for the creator that this action prepared: the page puts it into the chat box. Nothing
	// has been sent.
	return c.JSON(fiber.Map{"success": true, "data": res.Koc, "draft": res.Draft})
}

// Draft handles GET /kocs/:id/draft?kind=sample_approved|shipment|delivered&product=: a message for the creator, written
// from what is known about the KOC, for the booker to read, change and send. It sends nothing and changes nothing.
func (h *KocHandler) Draft(c *fiber.Ctx) error {
	koc, err := h.kocs.FindActive(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	var text string
	switch c.Query("kind") {
	case "sample_approved":
		text = service.SampleApprovedDraft(koc, truncateRunes(c.Query("product"), 150))
	case "shipment":
		text = service.ShipmentDraft(koc)
	case "delivered":
		text = service.DeliveredDraft(koc)
	default:
		return httpx.NewError(fiber.StatusBadRequest, "Loại lời nhắn không hợp lệ.")
	}
	return httpx.OK(c, fiber.Map{"text": text})
}

func truncateRunes(s string, n int) string {
	if r := []rune(strings.TrimSpace(s)); len(r) > n {
		return string(r[:n])
	}
	return strings.TrimSpace(s)
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
		PhoneLast4   string `json:"phoneLast4"`
	}
	_ = c.BodyParser(&body)
	if koc.TrackingCode == "" && body.TrackingCode == "" {
		return httpx.NewError(400, "KOC chưa có mã vận đơn J&T Express để tra cứu")
	}
	if body.TrackingCode != "" {
		koc.TrackingCode = strings.TrimSpace(body.TrackingCode)
	}
	if p := strings.TrimSpace(body.PhoneLast4); p != "" {
		if len(p) != 4 || service.NormalizePhoneLast4(p) != p {
			return httpx.NewError(400, service.ErrPhoneLast4.Error())
		}
		koc.TrackingPhoneLast4 = p
	}
	if service.ResolvePhoneLast4(koc) == "" {
		return httpx.NewError(400, service.ErrPhoneLast4.Error())
	}
	res, err := h.outreach.SyncKocShipment(c.UserContext(), koc)
	if err != nil {
		return err
	}
	message := "Đã cập nhật trạng thái vận đơn J&T Express"
	if res.Tracking != nil && res.Tracking.StatusText != "" {
		message = res.Tracking.StatusText
	}
	draft := ""
	if res.JustDelivered {
		draft = service.DeliveredDraft(res.Koc) // found delivered just now: the page offers it, nothing is sent
	}
	return c.JSON(fiber.Map{"success": true, "data": res.Koc, "tracking": res.Tracking, "message": message, "draft": draft})
}

// LookupJtTracking handles POST /kocs/track-shipment/lookup.
func (h *KocHandler) LookupJtTracking(c *fiber.Ctx) error {
	var body struct {
		TrackingCode string `json:"trackingCode"`
		PhoneLast4   string `json:"phoneLast4"`
	}
	_ = c.BodyParser(&body)
	code := strings.TrimSpace(body.TrackingCode)
	if code == "" {
		return httpx.NewError(400, "Vui lòng nhập mã vận đơn J&T Express")
	}
	phone4 := strings.TrimSpace(body.PhoneLast4)
	if len(phone4) != 4 || service.NormalizePhoneLast4(phone4) != phone4 {
		return httpx.NewError(400, service.ErrPhoneLast4.Error())
	}
	tracking, err := h.outreach.QueryTracking(c.UserContext(), code, phone4)
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
	a := middleware.ActorOf(c)
	h.audit.Log(nil, a.ID, a.Email, "outreach.run", "", "", req.Mode, a.IP)
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

type messageBatchRequest struct {
	Body    string   `json:"body" validate:"required,max=2000"`
	KocIDs  []string `json:"kocIds" validate:"required,min=1,max=5,dive,required"`
	Confirm bool     `json:"confirm"`
	RunID   string   `json:"runId" validate:"omitempty,max=64"` // ties the batches of one press of "Gửi" together
}

// SendMessageBatch handles POST /kocs/message-batch: one template to up to 5 KOCs over Zalo, through the anti-spam guard.
// It reaches real people, so it needs confirm=true. A KOC with no active record is handed on as nil and reported by the
// service; a repeated id is sent once.
func (h *KocHandler) SendMessageBatch(c *fiber.Ctx) error {
	var req messageBatchRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	if !req.Confirm {
		return httpx.Fail(c, fiber.StatusBadRequest, "Cần xác nhận trước khi gửi tin hàng loạt.")
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return httpx.Fail(c, fiber.StatusBadRequest, "Nội dung tin nhắn không được để trống.")
	}
	ctx := c.UserContext()
	seen := map[string]bool{}
	targets := make([]service.BatchTarget, 0, len(req.KocIDs))
	for _, id := range req.KocIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		koc, err := h.kocs.FindActive(ctx, id)
		var fault *domain.Fault
		switch {
		case errors.As(err, &fault) && fault.Kind == domain.FaultNotFound:
			koc = nil
		case err != nil:
			return err
		}
		targets = append(targets, service.BatchTarget{ID: id, Koc: koc})
	}
	report, err := h.outreach.SendTemplateBatch(ctx, body, targets)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, r := range report.Results {
		counts[r.Status]++
	}
	a := middleware.ActorOf(c)
	h.audit.Log(nil, a.ID, a.Email, "outreach.message_batch", "", "",
		fmt.Sprintf("%d KOC: sent=%d pending=%d skipped=%d not_sent=%d failed=%d", len(targets),
			counts[service.BatchSent], counts[service.BatchPending], counts[service.BatchSkipped], counts[service.BatchNotSent], counts[service.BatchFailed]), a.IP)
	h.recordBatch(c, req.RunID, body, targets, report, a.Email)
	return httpx.OK(c, report)
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
	a := middleware.ActorOf(c)
	h.audit.Log(nil, a.ID, a.Email, "koc.archived", "koc", koc.ID.Hex(), "@"+koc.TiktokID, a.IP)
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

// recordBatch writes one log row per KOC of the batch. Only the beginning of the text is kept. The messages have
// already gone out, so a failure here is logged and never turns the answer into an error.
func (h *KocHandler) recordBatch(c *fiber.Ctx, runID, body string, targets []service.BatchTarget, report *service.BatchReport, by string) {
	if h.sendLog == nil || len(report.Results) == 0 {
		return
	}
	if runID == "" {
		runID = domain.NewID().Hex()
	}
	preview := []rune(body)
	if len(preview) > 80 {
		preview = preview[:80]
	}
	byID := map[string]*domain.Koc{}
	for _, t := range targets {
		byID[t.ID] = t.Koc
	}
	now := time.Now()
	rows := make([]domain.BulkSendLog, 0, len(report.Results))
	for _, r := range report.Results {
		row := domain.BulkSendLog{RunID: runID, KocID: r.KocID, Status: r.Status, Reason: r.Reason, Preview: string(preview), By: by, At: now}
		if k := byID[r.KocID]; k != nil {
			row.KocName, row.Handle = k.DisplayName, "@"+k.TiktokID
		}
		rows = append(rows, row)
	}
	if err := h.sendLog.Record(c.UserContext(), rows); err != nil {
		log.Printf("bulk send log: %v", err)
	}
}

// MessageLog handles GET /kocs/message-log?limit=: what happened to the KOCs of the latest bulk sends, newest first.
func (h *KocHandler) MessageLog(c *fiber.Ctx) error {
	if h.sendLog == nil {
		return httpx.OK(c, []domain.BulkSendLog{})
	}
	limit := int64(c.QueryInt("limit", 200))
	if limit < 1 || limit > 500 {
		limit = 200
	}
	rows, err := h.sendLog.Recent(c.UserContext(), limit)
	if err != nil {
		return err
	}
	return httpx.OK(c, rows)
}
