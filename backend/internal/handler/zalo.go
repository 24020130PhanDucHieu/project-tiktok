package handler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/repository"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// ZaloHandler serves the personal-Zalo control routes, the OA webhook and the bridge callback.
type ZaloHandler struct {
	bridge   *service.BridgeClient
	convs    *repository.ConversationRepository
	msgs     *repository.MessageRepository
	outreach *service.OutreachService
}

// NewZaloHandler builds a ZaloHandler.
func NewZaloHandler(bridge *service.BridgeClient, convs *repository.ConversationRepository,
	msgs *repository.MessageRepository, outreach *service.OutreachService) *ZaloHandler {
	return &ZaloHandler{bridge: bridge, convs: convs, msgs: msgs, outreach: outreach}
}

// Status handles GET /zalo/status.
func (h *ZaloHandler) Status(c *fiber.Ctx) error {
	return httpx.OK(c, h.bridge.Status(c.UserContext()))
}

// StartQR handles POST /zalo/qr/start.
func (h *ZaloHandler) StartQR(c *fiber.Ctx) error {
	return httpx.OK(c, h.bridge.StartQR(c.UserContext()))
}

// Disconnect handles POST /zalo/disconnect.
func (h *ZaloHandler) Disconnect(c *fiber.Ctx) error {
	return httpx.OK(c, h.bridge.Disconnect(c.UserContext()))
}

// Connect handles POST /zalo/connect with cookie/imei/userAgent (falls back to env credentials).
func (h *ZaloHandler) Connect(c *fiber.Ctx) error {
	var creds service.ZaloCredentials
	_ = c.BodyParser(&creds)
	if len(creds.Cookie) == 0 || creds.IMEI == "" || creds.UserAgent == "" {
		env := h.bridge.EnvCredentials()
		if env == nil {
			return httpx.NewError(fiber.StatusBadRequest, "Thiếu cookie, IMEI hoặc user-agent Zalo cá nhân.")
		}
		creds = *env
	}
	status, err := h.bridge.Connect(c.UserContext(), creds)
	if err != nil {
		return err
	}
	return httpx.OK(c, status)
}

// VerifyWebhook handles GET /webhooks/zalo.
func (h *ZaloHandler) VerifyWebhook(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"success": true, "service": "zalo-webhook"})
}

// ReceiveWebhook handles POST /webhooks/zalo (Zalo OA events).
func (h *ZaloHandler) ReceiveWebhook(c *fiber.Ctx) error {
	var payload struct {
		EventName string `json:"event_name"`
		Event     string `json:"event"`
		UserID    string `json:"user_id"`
		Text      string `json:"text"`
		Sender    struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"sender"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
	}
	_ = json.Unmarshal(c.Body(), &payload)

	event := defaultStr(payload.EventName, payload.Event)
	userID := defaultStr(payload.Sender.ID, payload.UserID)
	text := defaultStr(payload.Message.Text, payload.Text)
	if event != "" && event != "user_send_text" {
		return c.JSON(fiber.Map{"success": true, "ignored": true})
	}
	if userID == "" || text == "" {
		return httpx.Fail(c, fiber.StatusBadRequest, "Webhook thiếu user_id hoặc nội dung tin nhắn")
	}

	ctx := c.UserContext()
	conv, err := h.convs.UpsertZaloInbound(ctx, userID, defaultStr(payload.Sender.DisplayName, "Zalo "+userID), text, time.Now())
	if err != nil {
		return err
	}
	if err := h.msgs.Create(ctx, &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionInbound, Text: text, Status: domain.MessageDelivered}); err != nil {
		return err
	}
	res, err := h.outreach.ProcessInboundWithChatbot(ctx, conv, text, true)
	if err != nil {
		return err
	}
	var updatedStage any
	if res.Koc != nil {
		updatedStage = res.Koc.Stage
	}
	return c.JSON(fiber.Map{"success": true, "chatbot": fiber.Map{
		"intent": res.Parsed.Intent, "extractedPhone": res.Parsed.Phone,
		"extractedAddress": res.Parsed.ShippingAddress, "updatedStage": updatedStage,
		"autoReplied": res.BotMessage != nil,
	}})
}

// ReceivePersonalInbound handles POST /internal/zalo/inbound, called by the zalo-bridge for each incoming
// personal-account message. It stores the message, runs the chatbot and sends the bot reply back through Zalo.
func (h *ZaloHandler) ReceivePersonalInbound(c *fiber.Ctx) error {
	var in struct {
		ExternalContactID string `json:"externalContactId"`
		Name              string `json:"name"`
		Text              string `json:"text"`
		Timestamp         int64  `json:"ts"`
	}
	if err := c.BodyParser(&in); err != nil || in.ExternalContactID == "" || in.Text == "" {
		return httpx.Fail(c, fiber.StatusBadRequest, "Thiếu externalContactId hoặc nội dung tin nhắn")
	}
	ctx := c.UserContext()
	at := time.Now()
	if in.Timestamp > 0 {
		at = time.UnixMilli(in.Timestamp)
	}
	conv, err := h.convs.UpsertZaloInbound(ctx, in.ExternalContactID, defaultStr(in.Name, "Zalo "+in.ExternalContactID), in.Text, at)
	if err != nil {
		return err
	}
	if err := h.msgs.Create(ctx, &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionInbound, Text: in.Text, Status: domain.MessageDelivered}); err != nil {
		return err
	}
	// Chatbot / delivery failures must not fail the inbound ingestion.
	if res, err := h.outreach.ProcessInboundWithChatbot(ctx, conv, in.Text, true); err == nil && res.BotMessage != nil {
		if err := h.bridge.Send(ctx, in.ExternalContactID, res.BotMessage.Text); err != nil {
			fmt.Printf("zalo bot reply not delivered: %v\n", err)
		}
	}
	return c.JSON(fiber.Map{"success": true})
}
