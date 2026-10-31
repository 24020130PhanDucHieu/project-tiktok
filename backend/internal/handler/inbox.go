package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/repository"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// InboxHandler serves /inbox routes.
type InboxHandler struct {
	kocs     *repository.KocRepository
	convs    *repository.ConversationRepository
	msgs     *repository.MessageRepository
	outreach *service.OutreachService
}

// NewInboxHandler builds an InboxHandler.
func NewInboxHandler(kocs *repository.KocRepository, convs *repository.ConversationRepository,
	msgs *repository.MessageRepository, outreach *service.OutreachService) *InboxHandler {
	return &InboxHandler{kocs: kocs, convs: convs, msgs: msgs, outreach: outreach}
}

type messageRequest struct {
	Text      string `json:"text" validate:"required,min=1,max=4000"`
	AutoReply *bool  `json:"autoReply"`
}

const conversationNotFound = "Không tìm thấy cuộc hội thoại"

// ListConversations handles GET /inbox/conversations (also back-fills a chat thread for every KOC).
func (h *InboxHandler) ListConversations(c *fiber.Ctx) error {
	ctx := c.UserContext()
	kocs, err := h.kocs.Find(ctx, bson.M{"status": domain.KocActive})
	if err != nil {
		return err
	}
	if err := h.convs.EnsureForKocs(ctx, kocs); err != nil {
		return err
	}
	items, err := h.convs.ListOpen(ctx)
	if err != nil {
		return err
	}
	return httpx.OK(c, items)
}

// ListMessages handles GET /inbox/conversations/:conversationId/messages.
func (h *InboxHandler) ListMessages(c *fiber.Ctx) error {
	items, err := h.msgs.ListByConversation(c.UserContext(), c.Params("conversationId"))
	if err != nil {
		return err
	}
	return httpx.OK(c, items)
}

// SendMessage handles POST /inbox/conversations/:conversationId/messages.
func (h *InboxHandler) SendMessage(c *fiber.Ctx) error {
	var req messageRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	ctx := c.UserContext()
	conv, err := h.convs.FindByID(ctx, c.Params("conversationId"))
	if err != nil {
		return err
	}
	if conv == nil {
		return httpx.Fail(c, fiber.StatusNotFound, conversationNotFound)
	}
	zalo := h.outreach.Zalo()

	if conv.ExternalContactID == "" && conv.ContactPhone != "" {
		user, err := zalo.Personal().ResolveByPhone(ctx, conv.ContactPhone)
		if err != nil {
			return httpx.Fail(c, fiber.StatusUnprocessableEntity, "Không thể tìm tài khoản Zalo theo số điện thoại: "+err.Error())
		}
		conv.Channel = domain.ChannelZalo
		conv.ExternalContactID = user.UserID
		if user.DisplayName != "" {
			conv.ContactName = user.DisplayName
		}
		if err := h.convs.Save(ctx, conv); err != nil {
			return err
		}
	}

	msg := &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionOutbound, Text: req.Text, Status: domain.MessagePending}
	if err := h.msgs.Create(ctx, msg); err != nil {
		return err
	}
	if conv.Channel == domain.ChannelZalo && conv.ExternalContactID != "" {
		if _, err := zalo.SendText(ctx, conv.ExternalContactID, msg.Text); err != nil {
			msg.Status = domain.MessageFailed
			_ = h.msgs.Save(ctx, msg)
			return httpx.Fail(c, fiber.StatusBadGateway, "Zalo không nhận được tin nhắn: "+err.Error())
		}
		msg.Status = domain.MessageSent
		if err := h.msgs.Save(ctx, msg); err != nil {
			return err
		}
	}
	conv.LastMessage, conv.LastMessageAt = msg.Text, msg.SentAt
	if err := h.convs.Save(ctx, conv); err != nil {
		return err
	}
	notice := "Đã lưu tin nhắn nội bộ; cần cấu hình Zalo OA để gửi ra ngoài."
	if msg.Status == domain.MessageSent {
		notice = "Đã gửi qua Zalo OA."
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": msg, "notice": notice})
}

// ReceiveInbound handles POST /inbox/conversations/:conversationId/inbound (simulates a KOC reply).
func (h *InboxHandler) ReceiveInbound(c *fiber.Ctx) error {
	var req messageRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	ctx := c.UserContext()
	conv, err := h.convs.FindByID(ctx, c.Params("conversationId"))
	if err != nil {
		return err
	}
	if conv == nil {
		return httpx.Fail(c, fiber.StatusNotFound, conversationNotFound)
	}
	autoReply := req.AutoReply == nil || *req.AutoReply

	inbound := &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionInbound, Text: req.Text, Status: domain.MessageDelivered}
	if err := h.msgs.Create(ctx, inbound); err != nil {
		return err
	}
	conv.LastMessage, conv.LastMessageAt = req.Text, inbound.SentAt
	if err := h.convs.Save(ctx, conv); err != nil {
		return err
	}
	res, err := h.outreach.ProcessInboundWithChatbot(ctx, conv, req.Text, autoReply)
	if err != nil {
		return err
	}
	return httpx.Created(c, fiber.Map{
		"inboundMessage": inbound, "botMessage": res.BotMessage, "koc": res.Koc, "parsed": res.Parsed,
	})
}

// ParseChatbot handles POST /inbox/chatbot/parse (stateless intent preview).
func (h *InboxHandler) ParseChatbot(c *fiber.Ctx) error {
	var body struct {
		Text string `json:"text"`
		Koc  struct {
			Name           string  `json:"name"`
			DisplayName    string  `json:"displayName"`
			ProductName    string  `json:"productName"`
			CommissionRate float64 `json:"commissionRate"`
			Dnc            bool    `json:"dnc"`
		} `json:"koc"`
	}
	_ = c.BodyParser(&body)
	parsed := service.ParseKocInboundText(body.Text)
	name := defaultStr(body.Koc.Name, defaultStr(body.Koc.DisplayName, "bạn"))
	reply := service.BuildChatbotAutoReply(service.ChatbotKocInfo{
		DisplayName: name, ProductName: defaultStr(body.Koc.ProductName, "sản phẩm mẫu"),
		CommissionRate: body.Koc.CommissionRate, Dnc: body.Koc.Dnc,
	}, parsed)
	return httpx.OK(c, fiber.Map{"parsed": parsed, "botReply": reply})
}

// ClearMessages handles DELETE /inbox/conversations/:conversationId/messages.
func (h *InboxHandler) ClearMessages(c *fiber.Ctx) error {
	ctx := c.UserContext()
	conv, err := h.convs.FindByID(ctx, c.Params("conversationId"))
	if err != nil {
		return err
	}
	if conv == nil {
		return httpx.Fail(c, fiber.StatusNotFound, conversationNotFound)
	}
	if err := h.msgs.DeleteByConversation(ctx, conv.ID); err != nil {
		return err
	}
	conv.LastMessage, conv.LastMessageAt, conv.UnreadCount = "Đã làm mới cuộc trò chuyện", time.Now(), 0
	if err := h.convs.Save(ctx, conv); err != nil {
		return err
	}
	return httpx.OKMessage(c, conv, fmt.Sprintf("Đã xóa toàn bộ lịch sử tin nhắn với %s.", conv.ContactName))
}

// DeleteMessage handles DELETE /inbox/conversations/:conversationId/messages/:messageId.
func (h *InboxHandler) DeleteMessage(c *fiber.Ctx) error {
	ctx := c.UserContext()
	convID := c.Params("conversationId")
	if err := h.msgs.DeleteOne(ctx, convID, c.Params("messageId")); err != nil {
		return err
	}
	if conv, err := h.convs.FindByID(ctx, convID); err == nil && conv != nil {
		last := "Chưa có tin nhắn"
		if latest, err := h.msgs.Latest(ctx, conv.ID); err == nil && latest != nil {
			last = latest.Text
		}
		if err := h.convs.SetLastMessage(ctx, conv.ID, last); err != nil {
			return err
		}
	}
	return c.JSON(fiber.Map{"success": true, "message": "Đã xóa tin nhắn."})
}
