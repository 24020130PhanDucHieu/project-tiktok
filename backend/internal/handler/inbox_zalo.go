package handler

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// What a person does by hand on a Zalo conversation besides writing: asking the KOC to be friends, and putting a
// reaction on a message the KOC sent. Both are a click by a signed-in member: the master switch for automatic
// messages does not apply, and nothing here runs by itself.

// ensureZaloContact makes sure the conversation knows the Zalo account of the KOC, looking it up by phone number the
// first time. The error is written for the person using the screen.
func (h *InboxHandler) ensureZaloContact(ctx context.Context, conv *domain.Conversation) error {
	if conv.Channel == domain.ChannelZalo && conv.ExternalContactID != "" {
		return nil
	}
	if conv.ContactPhone == "" {
		return errors.New("KOC chưa có số điện thoại Zalo.")
	}
	user, err := h.outreach.Zalo().Personal().ResolveByPhone(ctx, conv.ContactPhone)
	if err != nil {
		return errors.New("Số này không có tài khoản Zalo hoặc không tra được: " + err.Error())
	}
	conv.Channel, conv.ExternalContactID = domain.ChannelZalo, user.UserID
	if user.DisplayName != "" {
		conv.ContactName = user.DisplayName
	}
	return h.convs.SetZaloContact(ctx, conv.ID, domain.ZaloContact{Channel: domain.ChannelZalo, ExternalContactID: user.UserID, Phone: conv.ContactPhone, Name: user.DisplayName})
}

func (h *InboxHandler) zaloConv(c *fiber.Ctx) (*domain.Conversation, error) {
	conv, err := h.convs.FindByID(c.UserContext(), c.Params("conversationId"))
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, httpx.Fail(c, fiber.StatusNotFound, conversationNotFound)
	}
	return conv, nil
}

type zaloFriendState struct {
	Available       bool   `json:"available"` // false: the question cannot be asked now (see Reason)
	Reason          string `json:"reason,omitempty"`
	IsFriend        bool   `json:"isFriend"`
	RequestSent     bool   `json:"requestSent"`
	RequestReceived bool   `json:"requestReceived"`
	NoteMax         int    `json:"noteMax"`
}

// ZaloFriend handles GET /inbox/conversations/:conversationId/zalo/friend.
func (h *InboxHandler) ZaloFriend(c *fiber.Ctx) error {
	conv, err := h.zaloConv(c)
	if conv == nil {
		return err
	}
	ctx := c.UserContext()
	out := zaloFriendState{NoteMax: service.ZaloFriendNoteMax}
	if !h.outreach.Zalo().Personal().IsConnected(ctx) {
		out.Reason = "Zalo cá nhân chưa kết nối."
		return httpx.OK(c, out)
	}
	if err := h.ensureZaloContact(ctx, conv); err != nil {
		out.Reason = err.Error()
		return httpx.OK(c, out)
	}
	st, err := h.outreach.Zalo().Personal().FriendStatus(ctx, conv.ExternalContactID)
	if err != nil {
		out.Reason = "Không đọc được trạng thái kết bạn: " + err.Error()
		return httpx.OK(c, out)
	}
	out.Available, out.IsFriend, out.RequestSent, out.RequestReceived = true, st.IsFriend, st.RequestSent, st.RequestReceived
	return httpx.OK(c, out)
}

// ZaloFriendRequest handles POST /inbox/conversations/:conversationId/zalo/friend-request.
func (h *InboxHandler) ZaloFriendRequest(c *fiber.Ctx) error {
	var req struct {
		Message string `json:"message"`
	}
	if err := c.BodyParser(&req); err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Dữ liệu không hợp lệ")
	}
	note := strings.TrimSpace(req.Message)
	if note == "" {
		return httpx.Fail(c, fiber.StatusBadRequest, "Hãy viết vài chữ để KOC biết bạn là ai.")
	}
	if n := utf8.RuneCountInString(note); n > service.ZaloFriendNoteMax {
		return httpx.Fail(c, fiber.StatusBadRequest, "Lời nhắn kết bạn tối đa 150 ký tự (đang có "+strconv.Itoa(n)+").")
	}
	conv, err := h.zaloConv(c)
	if conv == nil {
		return err
	}
	ctx := c.UserContext()
	zalo := h.outreach.Zalo().Personal()
	if !zalo.IsConnected(ctx) {
		return httpx.Fail(c, fiber.StatusUnprocessableEntity, "Zalo cá nhân chưa kết nối.")
	}
	if err := h.ensureZaloContact(ctx, conv); err != nil {
		return httpx.Fail(c, fiber.StatusUnprocessableEntity, err.Error())
	}
	// Asking someone who is already a friend, or who was already asked, only annoys them (and Zalo counts it).
	if st, err := zalo.FriendStatus(ctx, conv.ExternalContactID); err == nil {
		switch {
		case st.IsFriend:
			return httpx.Fail(c, fiber.StatusConflict, "Hai bên đã là bạn bè trên Zalo.")
		case st.RequestSent:
			return httpx.Fail(c, fiber.StatusConflict, "Bạn đã gửi lời mời rồi, đang chờ KOC chấp nhận.")
		}
	}
	if err := zalo.SendFriendRequest(ctx, conv.ExternalContactID, note); err != nil {
		return httpx.Fail(c, fiber.StatusBadGateway, "Zalo không nhận được lời mời kết bạn: "+err.Error())
	}
	_ = h.msgs.Create(ctx, &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionOutbound, Kind: "system",
		Text: "Đã gửi lời mời kết bạn Zalo: " + note, Status: domain.MessageDelivered})
	return httpx.OK(c, fiber.Map{"sent": true})
}

// canReact: only a message the KOC sent through the connected line carries the ids Zalo needs.
func canReact(m *domain.Message) bool {
	return m.Direction == domain.DirectionInbound && m.Kind == "" && m.ZaloMsgID != "" && m.ZaloCliMsgID != ""
}

// ReactToMessage handles PUT /inbox/conversations/:conversationId/messages/:messageId/reaction.
func (h *InboxHandler) ReactToMessage(c *fiber.Ctx) error {
	var req struct {
		Reaction string `json:"reaction"` // heart, like, haha, wow, cry, angry, pray, ok; empty takes it back
	}
	if err := c.BodyParser(&req); err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Dữ liệu không hợp lệ")
	}
	code := ""
	if req.Reaction != "" {
		var ok bool
		if code, ok = service.ZaloReactions[req.Reaction]; !ok {
			return httpx.Fail(c, fiber.StatusBadRequest, "Cảm xúc này chưa được hỗ trợ.")
		}
	}
	conv, err := h.zaloConv(c)
	if conv == nil {
		return err
	}
	ctx := c.UserContext()
	msg, err := h.msgs.FindByID(ctx, conv.ID, c.Params("messageId"))
	if err != nil {
		return err
	}
	if msg == nil {
		return httpx.Fail(c, fiber.StatusNotFound, "Không tìm thấy tin nhắn")
	}
	if !canReact(msg) || conv.ExternalContactID == "" {
		return httpx.Fail(c, fiber.StatusConflict, "Chỉ thả được cảm xúc cho tin KOC nhắn qua Zalo (tin cũ, trước khi có tính năng này, không thả được).")
	}
	if err := h.outreach.Zalo().Personal().React(ctx, conv.ExternalContactID, msg.ZaloMsgID, msg.ZaloCliMsgID, code); err != nil {
		return httpx.Fail(c, fiber.StatusBadGateway, "Zalo không nhận được cảm xúc: "+err.Error())
	}
	if err := h.msgs.SetReaction(ctx, conv.ID, c.Params("messageId"), req.Reaction); err != nil {
		return err
	}
	msg.Reaction, msg.CanReact = req.Reaction, true
	return httpx.OK(c, msg)
}
