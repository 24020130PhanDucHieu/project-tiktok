package handler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// InboxHandler serves /inbox routes.
type InboxHandler struct {
	kocs     InboxKocStore
	convs    ConversationStore
	msgs     MessageStore
	outreach InboxOutreach
	gmail    MailSender
}

// InboxKocStore is the part of *repository.KocRepository the inbox uses.
type InboxKocStore interface {
	FindActiveByTiktokIDs(ctx context.Context, ids []string) ([]*domain.Koc, error)
	FindActiveByID(ctx context.Context, id string) (*domain.Koc, error)
	Save(ctx context.Context, koc *domain.Koc) error
}

// ConversationStore is the part of *repository.ConversationRepository the inbox and collaboration routes use.
type ConversationStore interface {
	ListOpen(ctx context.Context, f domain.ConversationFilter) ([]domain.Conversation, error)
	FindByID(ctx context.Context, id string) (*domain.Conversation, error)
	FindByHandle(ctx context.Context, handle string) (*domain.Conversation, error)
	Save(ctx context.Context, c *domain.Conversation) error
	MarkRead(ctx context.Context, id primitive.ObjectID) error
	SetHidden(ctx context.Context, id primitive.ObjectID, hidden bool) error
	Delete(ctx context.Context, id primitive.ObjectID) error
	SetLabels(ctx context.Context, id primitive.ObjectID, labels []string) error
	SetLastMessage(ctx context.Context, id primitive.ObjectID, text string) error
	ListLabels(ctx context.Context) ([]domain.Label, error)
	CreateLabel(ctx context.Context, l *domain.Label) error
	DeleteLabel(ctx context.Context, id string) error
	ListTemplates(ctx context.Context) ([]domain.MessageTemplate, error)
	CreateTemplate(ctx context.Context, t *domain.MessageTemplate) error
	DeleteTemplate(ctx context.Context, id string) error
}

// MessageStore is the part of *repository.MessageRepository the inbox and collaboration routes use.
type MessageStore interface {
	Create(ctx context.Context, m *domain.Message) error
	Save(ctx context.Context, m *domain.Message) error
	ListByConversation(ctx context.Context, convID, channel string) ([]domain.Message, error)
	FindByID(ctx context.Context, convID primitive.ObjectID, id string) (*domain.Message, error)
	DeleteByConversation(ctx context.Context, convID primitive.ObjectID) error
	DeleteOne(ctx context.Context, convID, msgID string) error
	Latest(ctx context.Context, convID primitive.ObjectID) (*domain.Message, error)
}

// InboxOutreach is the part of *service.OutreachService the inbox uses.
type InboxOutreach interface {
	Zalo() *service.ZaloService
	ProcessInboundWithChatbot(ctx context.Context, conv *domain.Conversation, text string, autoReply bool) (*service.InboundResult, error)
}

// MailSender sends an email through a user's linked mailbox (*service.GmailService).
type MailSender interface {
	Send(ctx context.Context, userID, to, subject, text string) (string, error)
}

// NewInboxHandler builds an InboxHandler.
func NewInboxHandler(kocs InboxKocStore, convs ConversationStore, msgs MessageStore, outreach InboxOutreach, gmail MailSender) *InboxHandler {
	return &InboxHandler{kocs: kocs, convs: convs, msgs: msgs, outreach: outreach, gmail: gmail}
}

type messageRequest struct {
	Text      string `json:"text" validate:"required,min=1,max=4000"`
	AutoReply *bool  `json:"autoReply"`
	Channel   string `json:"channel" validate:"omitempty,oneof=zalo email"`
	To        string `json:"to" validate:"omitempty,max=200"`
}

const conversationNotFound = "Không tìm thấy cuộc hội thoại"

// ListConversations handles GET /inbox/conversations.
// Threads for KOCs are created when the KOC is added (and once at startup for legacy data), so listing is a single indexed query.
func (h *InboxHandler) ListConversations(c *fiber.Ctx) error {
	f := domain.ConversationFilter{
		Query: c.Query("q"), Unread: c.Query("unread") == "1", Hidden: c.Query("hidden") == "1",
		Labels: splitCSV(c.Query("label")), Tasks: splitCSV(c.Query("task")),
	}
	ctx := c.UserContext()
	items, err := h.convs.ListOpen(ctx, f)
	if err != nil {
		return err
	}
	if err := h.attachNextTasks(ctx, items); err != nil {
		return err
	}
	if len(f.Tasks) > 0 {
		kept := items[:0]
		for _, it := range items {
			if slices.Contains(f.Tasks, it.NextTask) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	return httpx.OK(c, items)
}

// attachNextTasks fills Conversation.NextTask from the KOC behind each "@handle" thread.
func (h *InboxHandler) attachNextTasks(ctx context.Context, items []domain.Conversation) error {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		if strings.HasPrefix(it.ContactHandle, "@") {
			ids = append(ids, strings.TrimPrefix(it.ContactHandle, "@"))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	kocs, err := h.kocs.FindActiveByTiktokIDs(ctx, ids)
	if err != nil {
		return err
	}
	tasks := make(map[string]string, len(kocs))
	for _, k := range kocs {
		tasks["@"+k.TiktokID] = k.NextTask()
	}
	for i := range items {
		items[i].NextTask = tasks[items[i].ContactHandle]
	}
	return nil
}

func splitCSV(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// RecentConversations handles GET /inbox/conversations/recent ("Mở lại nhanh").
func (h *InboxHandler) RecentConversations(c *fiber.Ctx) error {
	items, err := h.convs.ListOpen(c.UserContext(), domain.ConversationFilter{})
	if err != nil {
		return err
	}
	if len(items) > 8 {
		items = items[:8]
	}
	return httpx.OK(c, items)
}

func (h *InboxHandler) findConv(c *fiber.Ctx) (*domain.Conversation, error) {
	conv, err := h.convs.FindByID(c.UserContext(), c.Params("conversationId"))
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, httpx.Fail(c, fiber.StatusNotFound, conversationNotFound)
	}
	return conv, nil
}

// MarkRead handles POST /inbox/conversations/:conversationId/read.
func (h *InboxHandler) MarkRead(c *fiber.Ctx) error {
	conv, err := h.findConv(c)
	if conv == nil {
		return err
	}
	if err := h.convs.MarkRead(c.UserContext(), conv.ID); err != nil {
		return err
	}
	return httpx.OK(c, fiber.Map{"unreadCount": 0})
}

// SetHidden handles POST /inbox/conversations/:conversationId/hide|unhide.
func (h *InboxHandler) SetHidden(hidden bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		conv, err := h.findConv(c)
		if conv == nil {
			return err
		}
		if err := h.convs.SetHidden(c.UserContext(), conv.ID, hidden); err != nil {
			return err
		}
		return httpx.OK(c, fiber.Map{"hidden": hidden})
	}
}

// SetLabels handles PUT /inbox/conversations/:conversationId/labels.
func (h *InboxHandler) SetLabels(c *fiber.Ctx) error {
	var req struct {
		Labels []string `json:"labels" validate:"max=20,dive,len=24"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	conv, err := h.findConv(c)
	if conv == nil {
		return err
	}
	if err := h.convs.SetLabels(c.UserContext(), conv.ID, req.Labels); err != nil {
		return err
	}
	return httpx.OK(c, fiber.Map{"labels": req.Labels})
}

// ListLabels handles GET /inbox/labels.
func (h *InboxHandler) ListLabels(c *fiber.Ctx) error {
	items, err := h.convs.ListLabels(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, items)
}

// CreateLabel handles POST /inbox/labels.
func (h *InboxHandler) CreateLabel(c *fiber.Ctx) error {
	var req struct {
		Name  string `json:"name" validate:"required,min=1,max=30"`
		Color string `json:"color" validate:"omitempty,hexcolor"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Color == "" {
		req.Color = "#2563eb"
	}
	l := &domain.Label{Name: strings.TrimSpace(req.Name), Color: req.Color}
	if err := h.convs.CreateLabel(c.UserContext(), l); err != nil {
		return err
	}
	return httpx.Created(c, l)
}

// DeleteLabel handles DELETE /inbox/labels/:labelId.
func (h *InboxHandler) DeleteLabel(c *fiber.Ctx) error {
	if err := h.convs.DeleteLabel(c.UserContext(), c.Params("labelId")); err != nil {
		return err
	}
	return httpx.OKMessage(c, nil, "Đã xóa nhãn.")
}

// ListMessages handles GET /inbox/conversations/:conversationId/messages.
func (h *InboxHandler) ListMessages(c *fiber.Ctx) error {
	items, err := h.msgs.ListByConversation(c.UserContext(), c.Params("conversationId"), c.Query("channel"))
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
	channel := req.Channel
	if channel == "" {
		channel = domain.ChannelZalo
	}
	msg := &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionOutbound, Text: req.Text,
		Status: domain.MessagePending, Channel: channel, To: strings.TrimSpace(req.To)}
	if channel == domain.ChannelEmail {
		msg.Subject = service.KocMailSubject
	}
	if err := h.msgs.Create(ctx, msg); err != nil {
		return err
	}
	return h.deliver(c, conv, msg)
}

// RetryMessage handles POST /inbox/conversations/:conversationId/messages/:messageId/retry.
func (h *InboxHandler) RetryMessage(c *fiber.Ctx) error {
	ctx := c.UserContext()
	conv, err := h.findConv(c)
	if conv == nil {
		return err
	}
	msg, err := h.msgs.FindByID(ctx, conv.ID, c.Params("messageId"))
	if err != nil {
		return err
	}
	if msg == nil || msg.Direction != domain.DirectionOutbound || msg.Status != domain.MessageFailed {
		return httpx.Fail(c, fiber.StatusConflict, "Chỉ gửi lại được tin nhắn bị lỗi.")
	}
	msg.Status = domain.MessagePending
	return h.deliver(c, conv, msg)
}

// deliver sends one stored outbound message over its channel and records the outcome.
func (h *InboxHandler) deliver(c *fiber.Ctx, conv *domain.Conversation, msg *domain.Message) error {
	ctx := c.UserContext()
	var sendErr error
	if msg.Channel == domain.ChannelEmail {
		sendErr = h.sendEmail(c, msg)
	} else {
		sendErr = h.sendZalo(ctx, conv, msg)
	}
	if sendErr != nil {
		msg.Status = domain.MessageFailed
		_ = h.msgs.Save(ctx, msg)
		status := fiber.StatusBadGateway
		if errors.Is(sendErr, service.ErrGmailNotConnected) || errors.Is(sendErr, service.ErrGmailBroken) {
			status = fiber.StatusUnprocessableEntity
		}
		return c.Status(status).JSON(fiber.Map{"success": false, "message": sendErr.Error(), "data": msg})
	}
	if err := h.msgs.Save(ctx, msg); err != nil {
		return err
	}
	conv.LastMessage, conv.LastMessageAt = msg.Text, msg.SentAt
	if err := h.convs.Save(ctx, conv); err != nil {
		return err
	}
	notice := "Đã lưu tin nhắn nội bộ; cần kết nối Zalo để gửi ra ngoài."
	switch {
	case msg.Channel == domain.ChannelEmail:
		notice = "Đã gửi email qua Gmail."
	case msg.Status == domain.MessageSent:
		notice = "Đã gửi qua Zalo."
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": msg, "notice": notice})
}

func (h *InboxHandler) sendEmail(c *fiber.Ctx, msg *domain.Message) error {
	user := middleware.CurrentUser(c)
	if user == nil || h.gmail == nil {
		return service.ErrGmailNotConnected
	}
	if msg.To == "" {
		return errors.New("KOC chưa có email")
	}
	id, err := h.gmail.Send(c.UserContext(), user.ID.Hex(), msg.To, msg.Subject, msg.Text)
	if err != nil {
		return err
	}
	msg.Status, msg.ExternalID = domain.MessageSent, id
	return nil
}

func (h *InboxHandler) sendZalo(ctx context.Context, conv *domain.Conversation, msg *domain.Message) error {
	zalo := h.outreach.Zalo()
	if phone := service.NormalizePhone(msg.To); phone != "" && phone != conv.ContactPhone {
		conv.ContactPhone, conv.ExternalContactID, conv.Channel = phone, "", domain.ChannelInternal
	}
	if conv.ExternalContactID == "" && conv.ContactPhone != "" {
		user, err := zalo.Personal().ResolveByPhone(ctx, conv.ContactPhone)
		if err != nil {
			return errors.New("Số này không có tài khoản Zalo hoặc không tra được: " + err.Error())
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
	if conv.Channel == domain.ChannelZalo && conv.ExternalContactID != "" {
		if _, err := zalo.SendText(ctx, conv.ExternalContactID, msg.Text); err != nil {
			return errors.New("Zalo không nhận được tin nhắn: " + err.Error())
		}
		msg.Status = domain.MessageSent
	}
	return nil
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

// DeleteConversation handles DELETE /inbox/conversations/:conversationId: the conversation and all its messages are
// removed for good (unlike hide, which only takes it out of the list). The KOC stays in the directory; if the KOC
// writes again on Zalo a new conversation is opened.
func (h *InboxHandler) DeleteConversation(c *fiber.Ctx) error {
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
	if err := h.convs.Delete(ctx, conv.ID); err != nil {
		return err
	}
	return httpx.OKMessage(c, fiber.Map{"deleted": true}, fmt.Sprintf("Đã xóa hẳn cuộc trò chuyện với %s.", conv.ContactName))
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
