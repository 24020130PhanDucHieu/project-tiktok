package handler

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/repository"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// TargetService is the part of *service.TTShopService this handler uses.
type TargetService interface {
	SearchCreators(ctx context.Context, keyword string, size int) ([]service.TargetCreator, error)
	ListProducts(ctx context.Context, pageToken string, size int) (*service.ShopProductPage, error)
	PreviewTarget(ctx context.Context, in service.TargetInviteInput, now time.Time, maxCreators int) (*service.TargetPreview, error)
	CreateTarget(ctx context.Context, payload *service.TargetAPIPayload) (string, error)
	SearchTargets(ctx context.Context, p service.TargetListParams) (*service.TargetPage, error)
	GetTarget(ctx context.Context, id string) (*service.TargetDetail, error)
	TargetLink(ctx context.Context, id string) (string, error)
	RemoveTarget(ctx context.Context, id string) error
}

// TargetStore is the part of *repository.TargetInviteRepository this handler uses.
type TargetStore interface {
	Insert(ctx context.Context, inv *domain.TargetInvite) (*domain.TargetInvite, error)
	SetResult(ctx context.Context, id primitive.ObjectID, status, targetID, errText string) error
	MarkCancelled(ctx context.Context, targetID string) error
	ByTargetIDs(ctx context.Context, ids []string) (map[string]domain.TargetInvite, error)
	ByTargetID(ctx context.Context, id string) (*domain.TargetInvite, error)
	SendingEnabled(ctx context.Context) (bool, error)
	SetSendingEnabled(ctx context.Context, enabled bool, by string) error
	ShopContact(ctx context.Context) (domain.ShopContact, error)
	SetShopContact(ctx context.Context, c domain.ShopContact, by string) error
}

// Auditor records sensitive actions (*service.AuditService).
type Auditor interface {
	Log(ctx context.Context, actorID, actorEmail, action, targetType, targetID, detail, ip string)
}

// TargetCollabHandler serves target collaboration ("cộng tác mục tiêu") routes. Sending and cancelling reach real
// creators and change the shop, so the router restricts them to admins and Create is switched off unless the server
// enables it.
type TargetCollabHandler struct {
	tts         TargetService
	store       TargetStore
	audit       Auditor
	maxCreators int
}

// NewTargetCollabHandler builds the handler. Sending is blocked until an admin turns the switch on in the app
// (stored in the database, off by default); everything else works regardless.
func NewTargetCollabHandler(tts TargetService, store TargetStore, audit Auditor, maxCreators int) *TargetCollabHandler {
	if maxCreators < 1 {
		maxCreators = 1
	}
	return &TargetCollabHandler{tts: tts, store: store, audit: audit, maxCreators: maxCreators}
}

var (
	reRequestKey = regexp.MustCompile(`^[A-Za-z0-9-]{16,64}$`)
	reObjectID   = regexp.MustCompile(`^[a-f0-9]{24}$`)
)

func targetFail(c *fiber.Ctx, err error) error {
	var rej *service.TTSRejected
	switch {
	case errors.Is(err, service.ErrTTShopNotConnected):
		return httpx.Fail(c, fiber.StatusConflict, err.Error())
	case errors.Is(err, service.ErrCreatorNotFound):
		return httpx.Fail(c, fiber.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrBadProductID):
		return httpx.Fail(c, fiber.StatusBadRequest, err.Error())
	case errors.As(err, &rej) && rej.Code == 36009002:
		return httpx.Fail(c, fiber.StatusTooManyRequests, "TikTok đang giới hạn tốc độ tìm kiếm. Hãy đợi vài phút rồi thử lại.")
	case errors.As(err, &rej):
		return httpx.Fail(c, fiber.StatusBadGateway, err.Error())
	default:
		return httpx.Fail(c, fiber.StatusBadGateway, err.Error())
	}
}

// sendingEnabled reads the switch. A missing setting or any read error means OFF (fail closed).
func (h *TargetCollabHandler) sendingEnabled(ctx context.Context) bool {
	on, err := h.store.SendingEnabled(ctx)
	return err == nil && on
}

// Config handles GET /target-collabs/config: the limits and whether sending is switched on.
func (h *TargetCollabHandler) Config(c *fiber.Ctx) error {
	contact, _ := h.store.ShopContact(c.UserContext()) // empty when unreadable: the form just starts blank
	return httpx.OK(c, fiber.Map{
		"contact": contact,
		"enabled": h.sendingEnabled(c.UserContext()), "maxCreators": h.maxCreators, "maxProducts": service.TargetMaxProducts,
		"minRatePct": service.TargetMinRatePct, "maxRatePct": service.TargetMaxRatePct, "minAdsRatePct": service.TargetMinAdsRatePct,
		"maxName": service.TargetMaxName, "maxMessage": service.TargetMaxMessage,
	})
}

// SetSwitch handles PUT /target-collabs/switch with {enabled, confirm}: turns sending of real invitations on or off.
// Turning it on needs explicit confirmation. Admin only (router).
func (h *TargetCollabHandler) SetSwitch(c *fiber.Ctx) error {
	var in struct {
		Enabled bool `json:"enabled"`
		Confirm bool `json:"confirm"`
	}
	if err := c.BodyParser(&in); err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Dữ liệu không hợp lệ")
	}
	if in.Enabled && !in.Confirm {
		return httpx.Fail(c, fiber.StatusBadRequest, "Cần xác nhận trước khi bật gửi lời mời thật.")
	}
	a := middleware.ActorOf(c)
	if err := h.store.SetSendingEnabled(c.UserContext(), in.Enabled, a.Email); err != nil {
		return httpx.Fail(c, fiber.StatusInternalServerError, "Không lưu được công tắc, trạng thái giữ nguyên.")
	}
	action := "ttshop.target.switch_off"
	if in.Enabled {
		action = "ttshop.target.switch_on"
	}
	h.audit.Log(c.UserContext(), a.ID, a.Email, action, "target_collab", "", "", a.IP)
	return httpx.OK(c, fiber.Map{"enabled": in.Enabled})
}

// Creators handles GET /target-collabs/creators?keyword= (read-only search in the Creator Marketplace).
func (h *TargetCollabHandler) Creators(c *fiber.Ctx) error {
	list, err := h.tts.SearchCreators(c.UserContext(), c.Query("keyword"), 20)
	if err != nil {
		return targetFail(c, err)
	}
	return httpx.OK(c, fiber.Map{"items": list})
}

// Products handles GET /target-collabs/products?pageToken= (read-only).
func (h *TargetCollabHandler) Products(c *fiber.Ctx) error {
	size, _ := strconv.Atoi(c.Query("pageSize", "50"))
	page, err := h.tts.ListProducts(c.UserContext(), c.Query("pageToken"), size)
	if err != nil {
		return targetFail(c, err)
	}
	return httpx.OK(c, page)
}

// Preview handles POST /target-collabs/preview: validates the form, re-checks creators and products with TikTok
// and returns the exact body that would be sent. It sends nothing.
func (h *TargetCollabHandler) Preview(c *fiber.Ctx) error {
	var in service.TargetInviteInput
	if err := c.BodyParser(&in); err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Dữ liệu lời mời không hợp lệ")
	}
	pv, err := h.tts.PreviewTarget(c.UserContext(), in, time.Now(), h.maxCreators)
	if err != nil {
		return targetFail(c, err)
	}
	return httpx.OK(c, fiber.Map{"preview": pv, "sendingEnabled": h.sendingEnabled(c.UserContext())})
}

type createTargetReq struct {
	RequestKey string                    `json:"requestKey"`
	Confirm    bool                      `json:"confirm"`
	KocID      string                    `json:"kocId"`
	CollabID   string                    `json:"collabId"`
	Invite     service.TargetInviteInput `json:"invite"`
}

// Create handles POST /target-collabs: sends a REAL invitation to the chosen creators.
func (h *TargetCollabHandler) Create(c *fiber.Ctx) error {
	if !h.sendingEnabled(c.UserContext()) {
		return httpx.Fail(c, fiber.StatusConflict, "Gửi lời mời đang tắt. Quản trị viên bật công tắc \"Cho phép gửi lời mời thật\" ở tab Cộng tác mục tiêu.")
	}
	var req createTargetReq
	if err := c.BodyParser(&req); err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Dữ liệu lời mời không hợp lệ")
	}
	if !req.Confirm {
		return httpx.Fail(c, fiber.StatusBadRequest, "Cần xác nhận trước khi gửi lời mời.")
	}
	if !reRequestKey.MatchString(req.RequestKey) {
		return httpx.Fail(c, fiber.StatusBadRequest, "Thiếu mã yêu cầu gửi lời mời.")
	}
	if (req.KocID != "" && !reObjectID.MatchString(req.KocID)) || (req.CollabID != "" && !reObjectID.MatchString(req.CollabID)) {
		return httpx.Fail(c, fiber.StatusBadRequest, "Mã KOC hoặc mã lần hợp tác không hợp lệ.")
	}
	ctx := c.UserContext()

	// Everything is re-checked on the server with TikTok: the browser's word is never enough.
	pv, err := h.tts.PreviewTarget(ctx, req.Invite, time.Now(), h.maxCreators)
	if err != nil {
		return targetFail(c, err)
	}
	if !pv.Ready || pv.Payload == nil {
		return httpx.Fail(c, fiber.StatusUnprocessableEntity, strings.Join(pv.Problems, " · "))
	}

	actor := middleware.ActorOf(c)
	rec := &domain.TargetInvite{
		RequestKey: req.RequestKey, Status: domain.TargetInviteSending, Name: pv.Payload.Name, EndDate: req.Invite.EndDate,
		Message: pv.Payload.Message, KocID: req.KocID, CollabID: req.CollabID, SentByID: actor.ID, SentByEmail: actor.Email,
	}
	rates := map[string]service.TargetProductInput{}
	for _, p := range req.Invite.Products {
		rates[strings.TrimSpace(p.ID)] = p
	}
	for _, p := range pv.Products {
		in := rates[p.ID]
		rec.Products = append(rec.Products, domain.TargetInviteProduct{ID: p.ID, Title: p.Title, CommissionPct: in.CommissionPct, AdsCommissionPct: in.AdsCommissionPct})
	}
	for _, cr := range pv.Creators {
		rec.Creators = append(rec.Creators, domain.TargetInviteCreator{OpenID: cr.OpenID, Username: cr.Username, Nickname: cr.Nickname})
	}

	stored, err := h.store.Insert(ctx, rec)
	if errors.Is(err, repository.ErrDuplicateRequest) {
		// the same request again (double click, retry): never send a second time
		if stored != nil && stored.Status == domain.TargetInviteSent {
			return httpx.OK(c, fiber.Map{"invite": stored, "duplicate": true})
		}
		return httpx.Fail(c, fiber.StatusConflict, "Yêu cầu này đã được gửi hoặc đang gửi. Hãy kiểm tra ở tab Cộng tác mục tiêu trước khi gửi lại.")
	}
	if err != nil {
		return httpx.Fail(c, fiber.StatusInternalServerError, "Không lưu được bản ghi lời mời, chưa gửi gì cho TikTok.")
	}

	id, err := h.tts.CreateTarget(ctx, pv.Payload)
	if err != nil {
		var conflict *service.ErrTargetConflict
		var rej *service.TTSRejected
		switch {
		case errors.As(err, &conflict):
			_ = h.store.SetResult(ctx, stored.ID, domain.TargetInviteFailed, "", err.Error())
			h.audit.Log(ctx, actor.ID, actor.Email, "ttshop.target.create_failed", "target_collab", "", err.Error(), actor.IP)
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "message": err.Error(), "conflicts": conflict.Conflicts})
		case errors.As(err, &rej):
			_ = h.store.SetResult(ctx, stored.ID, domain.TargetInviteFailed, "", err.Error())
			h.audit.Log(ctx, actor.ID, actor.Email, "ttshop.target.create_failed", "target_collab", "", err.Error(), actor.IP)
			return httpx.Fail(c, fiber.StatusBadGateway, err.Error())
		default:
			// timeout or broken connection: TikTok may have created it. Say so instead of risking a duplicate.
			_ = h.store.SetResult(ctx, stored.ID, domain.TargetInviteUnknown, "", err.Error())
			h.audit.Log(ctx, actor.ID, actor.Email, "ttshop.target.create_unknown", "target_collab", "", err.Error(), actor.IP)
			return httpx.Fail(c, fiber.StatusBadGateway, "Chưa rõ TikTok đã nhận lời mời hay chưa ("+err.Error()+"). Hãy kiểm tra ở tab Cộng tác mục tiêu trước khi gửi lại.")
		}
	}
	_ = h.store.SetResult(ctx, stored.ID, domain.TargetInviteSent, id, "")
	// remember the shop contact so the next window starts filled in (best effort, never blocks the answer)
	_ = h.store.SetShopContact(ctx, domain.ShopContact{Email: pv.Payload.SellerContactInfo.Email, Zalo: strings.TrimSpace(req.Invite.Zalo), Facebook: strings.TrimSpace(req.Invite.Facebook)}, actor.Email)
	stored.Status, stored.TargetID = domain.TargetInviteSent, id
	h.audit.Log(ctx, actor.ID, actor.Email, "ttshop.target.create", "target_collab", id,
		fmt.Sprintf("%d nhà sáng tạo, %d sản phẩm: %s", len(rec.Creators), len(rec.Products), pv.Payload.Name), actor.IP)
	return httpx.Created(c, fiber.Map{"invite": stored})
}

type targetListItem struct {
	service.TargetSummary
	SentBy       string   `json:"sentBy,omitempty"`
	CreatorNames []string `json:"creatorNames,omitempty"`
	LocalStatus  string   `json:"localStatus,omitempty"`
}

// List handles GET /target-collabs: the shop's invitations from TikTok, with who sent each from this app.
func (h *TargetCollabHandler) List(c *fiber.Ctx) error {
	size, _ := strconv.Atoi(c.Query("pageSize", "20"))
	page, err := h.tts.SearchTargets(c.UserContext(), service.TargetListParams{
		Status: c.Query("status"), AcceptStatus: c.Query("acceptStatus"), SampleSetting: c.Query("freeSample"), Keyword: c.Query("keyword"),
		KeywordType: c.Query("keywordType"), PageToken: c.Query("pageToken"), PageSize: size,
	})
	if err != nil {
		if strings.Contains(err.Error(), "không hợp lệ") {
			return httpx.Fail(c, fiber.StatusBadRequest, err.Error())
		}
		return targetFail(c, err)
	}
	ids := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		ids = append(ids, it.ID)
	}
	local, err := h.store.ByTargetIDs(c.UserContext(), ids)
	if err != nil {
		local = map[string]domain.TargetInvite{} // the TikTok list is still useful without our own notes
	}
	items := make([]targetListItem, 0, len(page.Items))
	for _, it := range page.Items {
		row := targetListItem{TargetSummary: it}
		if inv, ok := local[it.ID]; ok {
			row.SentBy, row.LocalStatus = inv.SentByEmail, inv.Status
			for _, cr := range inv.Creators {
				row.CreatorNames = append(row.CreatorNames, "@"+cr.Username)
			}
		}
		items = append(items, row)
	}
	return httpx.OK(c, fiber.Map{"items": items, "nextPageToken": page.NextPageToken, "total": page.Total})
}

// Detail handles GET /target-collabs/:id.
func (h *TargetCollabHandler) Detail(c *fiber.Ctx) error {
	d, err := h.tts.GetTarget(c.UserContext(), c.Params("id"))
	if err != nil {
		return targetFail(c, err)
	}
	local, _ := h.store.ByTargetID(c.UserContext(), d.ID)
	return httpx.OK(c, fiber.Map{"detail": d, "local": local})
}

// Link handles POST /target-collabs/:id/link: the link a creator opens to accept the invitation.
func (h *TargetCollabHandler) Link(c *fiber.Ctx) error {
	id := c.Params("id")
	link, err := h.tts.TargetLink(c.UserContext(), id)
	if err != nil {
		return targetFail(c, err)
	}
	a := middleware.ActorOf(c)
	h.audit.Log(c.UserContext(), a.ID, a.Email, "ttshop.target.link", "target_collab", id, "", a.IP)
	return httpx.OK(c, fiber.Map{"link": link})
}

// Cancel handles POST /target-collabs/:id/cancel with {confirm:true}: cancels the invitation on TikTok Shop.
func (h *TargetCollabHandler) Cancel(c *fiber.Ctx) error {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.BodyParser(&in); err != nil || !in.Confirm {
		return httpx.Fail(c, fiber.StatusBadRequest, "Cần xác nhận trước khi hủy lời mời.")
	}
	id := c.Params("id")
	ctx := c.UserContext()
	// TikTok's own docs scope edits to STANDARD invitations; a Top Creator Program invitation is never cancelled here.
	d, err := h.tts.GetTarget(ctx, id)
	if err != nil {
		return targetFail(c, err)
	}
	if d.Type != "STANDARD" {
		return httpx.Fail(c, fiber.StatusConflict, "Chỉ hủy được lời mời cộng tác mục tiêu thường, không hủy lời mời của chương trình Top Creator ở đây.")
	}
	if err := h.tts.RemoveTarget(ctx, id); err != nil {
		return targetFail(c, err)
	}
	_ = h.store.MarkCancelled(ctx, id)
	a := middleware.ActorOf(c)
	h.audit.Log(ctx, a.ID, a.Email, "ttshop.target.cancel", "target_collab", id, "", a.IP)
	return httpx.OK(c, fiber.Map{"cancelled": true})
}
