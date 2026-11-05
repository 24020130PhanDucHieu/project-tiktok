// Package router builds the Fiber application and registers every route.
package router

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/metrics"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// Handlers groups every HTTP handler.
type Handlers struct {
	Auth   *handler.AuthHandler
	Koc    *handler.KocHandler
	Inbox  *handler.InboxHandler
	Collab *handler.CollabHandler
	Gmail  *handler.GmailHandler
	Zalo   *handler.ZaloHandler
	Admin  *handler.AdminHandler
	TTShop *handler.TTShopHandler
	Target *handler.TargetCollabHandler
}

// New builds the Fiber app with middlewares and routes (prefix /api/v1).
func New(cfg config.Config, authSvc *service.AuthService, h Handlers, m *metrics.Collector) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "kocflow-api",
		ErrorHandler:          httpx.ErrorHandler,
		BodyLimit:             1 * 1024 * 1024,
		DisableStartupMessage: cfg.IsProduction(),
		ProxyHeader:           fiber.HeaderXForwardedFor, // real client IP behind Caddy (rate limit, audit)
		ReadTimeout:           15 * time.Second,
		WriteTimeout:          60 * time.Second, // TikTok/J&T lookups can be slow
		IdleTimeout:           90 * time.Second,
	})

	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(m.Middleware())
	app.Use(compress.New(compress.Config{Level: compress.LevelBestSpeed})) // JSON shrinks ~10x: saves bandwidth
	app.Use(helmet.New())
	app.Use(func(c *fiber.Ctx) error { // API responses are per-user: never cache them in proxies/browsers
		c.Set("Cache-Control", "no-store")
		return c.Next()
	})
	app.Use(limiter.New(limiter.Config{Max: 600, Expiration: time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return httpx.Fail(c, fiber.StatusTooManyRequests, "Quá nhiều yêu cầu. Vui lòng thử lại sau.")
		},
		Next: func(c *fiber.Ctx) bool { return c.Path() == "/health" || c.Path() == "/internal/zalo/inbound" }}))
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

	tooMany := func(max int, window time.Duration) fiber.Handler {
		return limiter.New(limiter.Config{Max: max, Expiration: window,
			LimitReached: func(c *fiber.Ctx) error {
				return httpx.Fail(c, fiber.StatusTooManyRequests, "Bạn thao tác quá nhanh. Vui lòng thử lại sau ít phút.")
			}})
	}
	requireAdmin := middleware.RequireRole(domain.RoleAdmin)

	auth := api.Group("/auth")
	auth.Post("/register", tooMany(10, time.Minute), h.Auth.Register)
	auth.Post("/login", tooMany(10, time.Minute), h.Auth.Login)
	auth.Post("/forgot-password", tooMany(5, 10*time.Minute), h.Auth.ForgotPassword)
	auth.Post("/reset-password", tooMany(10, 10*time.Minute), h.Auth.ResetPassword)
	auth.Get("/me", requireAuth, h.Auth.Me)
	auth.Patch("/me", requireAuth, h.Auth.UpdateMe)
	auth.Post("/change-password", requireAuth, tooMany(10, 10*time.Minute), h.Auth.ChangePassword)

	admin := api.Group("/admin", requireAuth, requireAdmin)
	admin.Get("/users", h.Admin.ListUsers)
	admin.Post("/users", h.Admin.CreateUser)
	admin.Patch("/users/:id", h.Admin.UpdateUser)
	admin.Post("/users/:id/reset-password", tooMany(20, 10*time.Minute), h.Admin.SendReset)
	admin.Delete("/users/:id", h.Admin.DeleteUser)
	admin.Get("/audit", h.Admin.Audit)
	admin.Get("/system", h.Admin.System)

	ttshop := api.Group("/ttshop", requireAuth, requireAdmin)
	ttshop.Get("/status", h.TTShop.Status)
	ttshop.Get("/logs", h.TTShop.Logs)
	ttshop.Post("/authorize", tooMany(10, 10*time.Minute), h.TTShop.Authorize)

	// read-only shop data used by the inbox (any signed-in member). Not under /ttshop: Fiber applies that group's
	// requireAdmin middleware to every path below the prefix.
	shop := api.Group("/shop", requireAuth)
	shop.Get("/open-collaborations", tooMany(60, time.Minute), h.TTShop.OpenCollabs)
	shop.Get("/open-collaboration-settings", tooMany(30, time.Minute), h.TTShop.OpenCollabSettings)

	// target collaboration: reading is open to signed-in members; sending, link and cancel reach real creators and
	// change the shop, so they are admin-only and rate limited. Static paths first, then /:id.
	target := api.Group("/target-collabs", requireAuth)
	target.Get("/config", h.Target.Config)
	target.Get("/creators", tooMany(60, time.Minute), h.Target.Creators)
	target.Get("/products", tooMany(30, time.Minute), h.Target.Products)
	target.Post("/preview", tooMany(30, time.Minute), h.Target.Preview)
	target.Get("/", tooMany(60, time.Minute), h.Target.List)
	target.Post("/", requireAdmin, tooMany(5, time.Hour), h.Target.Create)
	target.Get("/:id", tooMany(60, time.Minute), h.Target.Detail)
	target.Post("/:id/link", requireAdmin, tooMany(30, time.Hour), h.Target.Link)
	target.Post("/:id/cancel", requireAdmin, tooMany(20, time.Hour), h.Target.Cancel)

	api.Get("/webhooks/zalo", h.Zalo.VerifyWebhook)
	api.Post("/webhooks/zalo", h.Zalo.ReceiveWebhook)

	api.Get("/zalo/status", requireAuth, h.Zalo.Status)
	api.Post("/zalo/qr/start", requireAuth, h.Zalo.StartQR)
	api.Post("/zalo/connect", requireAuth, h.Zalo.Connect)
	api.Post("/zalo/disconnect", requireAuth, h.Zalo.Disconnect)

	kocs := api.Group("/kocs", requireAuth)
	kocs.Get("/", h.Koc.List)
	kocs.Get("/digest/morning", h.Koc.MorningDigest)
	kocs.Get("/stats", h.Koc.Stats)
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

	kocs.Get("/:id/collabs", h.Collab.List)
	kocs.Post("/:id/collabs", h.Collab.Open)
	collabs := api.Group("/collabs", requireAuth)
	collabs.Get("/overview", h.Collab.Overview)
	collabs.Patch("/:cid", h.Collab.Patch)
	collabs.Post("/:cid/open-link", tooMany(20, time.Minute), h.Collab.OpenLink)
	collabs.Post("/:cid/samples", h.Collab.AddSample)
	collabs.Post("/:cid/samples/:sid/approve", h.Collab.Approve())
	collabs.Post("/:cid/samples/:sid/reject", h.Collab.Reject())
	collabs.Post("/:cid/videos", h.Collab.AddVideo)
	collabs.Put("/:cid/videos/:idx/ads", h.Collab.SetAdsCode)
	collabs.Post("/:cid/close", h.Collab.Close)
	collabs.Patch("/:cid/outcome", h.Collab.SetOutcome)

	kocs.Get("/:id/contacts", h.Inbox.ListContacts)
	kocs.Post("/:id/contacts", h.Inbox.AddContact)
	kocs.Patch("/:id/contacts/:cid", h.Inbox.PatchContact)

	api.Get("/gmail/callback", h.Gmail.Callback)
	gmail := api.Group("/gmail", requireAuth)
	gmail.Get("/status", h.Gmail.Status)
	gmail.Get("/connect", h.Gmail.Connect)
	gmail.Delete("/", h.Gmail.Disconnect)

	inbox := api.Group("/inbox", requireAuth)
	inbox.Get("/templates", h.Inbox.ListTemplates)
	inbox.Post("/templates", h.Inbox.CreateTemplate)
	inbox.Delete("/templates/:templateId", h.Inbox.DeleteTemplate)
	inbox.Post("/conversations/:conversationId/messages/:messageId/retry", h.Inbox.RetryMessage)
	inbox.Get("/conversations", h.Inbox.ListConversations)
	inbox.Get("/conversations/recent", h.Inbox.RecentConversations)
	inbox.Post("/conversations/:conversationId/read", h.Inbox.MarkRead)
	inbox.Post("/conversations/:conversationId/hide", h.Inbox.SetHidden(true))
	inbox.Post("/conversations/:conversationId/unhide", h.Inbox.SetHidden(false))
	inbox.Put("/conversations/:conversationId/labels", h.Inbox.SetLabels)
	inbox.Get("/labels", h.Inbox.ListLabels)
	inbox.Post("/labels", h.Inbox.CreateLabel)
	inbox.Delete("/labels/:labelId", h.Inbox.DeleteLabel)
	inbox.Get("/conversations/:conversationId/messages", h.Inbox.ListMessages)
	inbox.Post("/conversations/:conversationId/messages", h.Inbox.SendMessage)
	inbox.Post("/conversations/:conversationId/inbound", h.Inbox.ReceiveInbound)
	inbox.Delete("/conversations/:conversationId/messages", h.Inbox.ClearMessages)
	inbox.Delete("/conversations/:conversationId/messages/:messageId", h.Inbox.DeleteMessage)
	inbox.Post("/chatbot/parse", h.Inbox.ParseChatbot)

	return app
}
