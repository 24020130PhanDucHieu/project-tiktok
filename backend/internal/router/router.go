// Package router builds the Fiber application and registers every route.
package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// Handlers groups every HTTP handler.
type Handlers struct {
	Auth  *handler.AuthHandler
	Koc   *handler.KocHandler
	Inbox *handler.InboxHandler
	Zalo  *handler.ZaloHandler
}

// New builds the Fiber app with middlewares and routes (prefix /api/v1).
func New(cfg config.Config, authSvc *service.AuthService, h Handlers) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "kocflow-api",
		ErrorHandler:          httpx.ErrorHandler,
		BodyLimit:             1 * 1024 * 1024,
		DisableStartupMessage: cfg.IsProduction(),
	})

	app.Use(recover.New())
	app.Use(helmet.New())
	app.Use(cors.New(cors.Config{AllowOrigins: cfg.FrontendURL}))
	if cfg.IsProduction() {
		app.Use(logger.New())
	} else {
		app.Use(logger.New(logger.Config{Format: "${method} ${path} ${status} ${latency}\n"}))
	}

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "service": "kocflow-api"})
	})

	// Internal callbacks from the zalo-bridge sidecar (shared-secret protected).
	app.Post("/internal/zalo/inbound", middleware.RequireBridgeSecret(cfg.ZaloBridgeSecret), h.Zalo.ReceivePersonalInbound)

	requireAuth := middleware.RequireAuth(authSvc)
	api := app.Group("/api/v1")

	auth := api.Group("/auth")
	auth.Post("/register", h.Auth.Register)
	auth.Post("/login", h.Auth.Login)
	auth.Get("/me", requireAuth, h.Auth.Me)

	api.Get("/webhooks/zalo", h.Zalo.VerifyWebhook)
	api.Post("/webhooks/zalo", h.Zalo.ReceiveWebhook)

	api.Get("/zalo/status", requireAuth, h.Zalo.Status)
	api.Post("/zalo/qr/start", requireAuth, h.Zalo.StartQR)
	api.Post("/zalo/connect", requireAuth, h.Zalo.Connect)
	api.Post("/zalo/disconnect", requireAuth, h.Zalo.Disconnect)

	kocs := api.Group("/kocs", requireAuth)
	kocs.Get("/", h.Koc.List)
	kocs.Get("/digest/morning", h.Koc.MorningDigest)
	kocs.Post("/lookup", h.Koc.Lookup)
	kocs.Post("/track-shipment/lookup", h.Koc.LookupJtTracking)
	kocs.Post("/video/lookup", h.Koc.LookupVideo)
	kocs.Post("/bulk", h.Koc.BulkImport)
	kocs.Post("/outreach/run", h.Koc.RunOutreach)
	kocs.Post("/", h.Koc.Create)
	kocs.Patch("/:id/stage", h.Koc.UpdateStage)
	kocs.Post("/:id/sync-channel", h.Koc.SyncChannel)
	kocs.Post("/:id/track-shipment", h.Koc.TrackShipment)
	kocs.Post("/:id/email", h.Koc.Email)
	kocs.Delete("/:id", h.Koc.Archive)

	inbox := api.Group("/inbox", requireAuth)
	inbox.Get("/conversations", h.Inbox.ListConversations)
	inbox.Get("/conversations/:conversationId/messages", h.Inbox.ListMessages)
	inbox.Post("/conversations/:conversationId/messages", h.Inbox.SendMessage)
	inbox.Post("/conversations/:conversationId/inbound", h.Inbox.ReceiveInbound)
	inbox.Delete("/conversations/:conversationId/messages", h.Inbox.ClearMessages)
	inbox.Delete("/conversations/:conversationId/messages/:messageId", h.Inbox.DeleteMessage)
	inbox.Post("/chatbot/parse", h.Inbox.ParseChatbot)

	return app
}
