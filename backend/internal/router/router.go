// Package router builds the Fiber application and registers every route.
package router

import (
	"encoding/json"
	"strings"
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
	// ZaloAccounts serves the personal Zalo accounts (several of them) and the bridge's credential callback; nil: not served
	ZaloAccounts *handler.ZaloAccountsHandler
	// Automation is the master switch for the messages the system sends by itself; it may be nil in tests
	Automation *handler.AutomationHandler
	// Campaign serves the campaigns of the bulk message and their pictures; nil: the routes are not served
	Campaign *handler.CampaignHandler
	// ImportJob serves the background import of KOCs from an Excel sheet; nil: the routes are not served
	ImportJob *handler.ImportJobHandler
	// Workflow serves the Quy trình tab (workflows of blocks drawn on a canvas); nil: the routes are not served
	Workflow *handler.WorkflowHandler
	// WorkflowTemplate serves the saved workflows; nil: the routes are not served
	WorkflowTemplate *handler.WorkflowTemplateHandler
	// BulkRun serves the bulk runs and their daily limit; nil: the routes are not served
	BulkRun *handler.BulkRunHandler
	// BulkSwitch is the switch of "Gửi hàng loạt", independent of Automation; nil: the routes are not served
	BulkSwitch *handler.AutomationHandler
	// Content serves the real videos and sales of KOCs (read-only); it may be nil in tests
	Content *handler.ContentHandler
}

const (
	maxJSONBody   = 1 * 1024 * 1024
	maxUploadBody = 3 * 1024 * 1024 // a picture of up to 2 MB plus the multipart framing
)

// New builds the Fiber app with middlewares and routes (prefix /api/v1).
func New(cfg config.Config, authSvc *service.AuthService, h Handlers, m *metrics.Collector) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "kocflow-api",
		ErrorHandler:          httpx.NewErrorHandler(!cfg.IsProduction()), // production never shows a driver or network error text
		BodyLimit:             maxUploadBody,                              // only the picture upload may use more than maxJSONBody: see the guard below
		DisableStartupMessage: cfg.IsProduction(),
		ProxyHeader:           fiber.HeaderXForwardedFor, // real client IP behind Caddy (rate limit, audit)
		// ...but only when the request really comes from a proxy on a private address (Caddy in the same Docker
		// network, or this machine). A caller reaching the API directly cannot forge the header to dodge rate limits.
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"127.0.0.0/8", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"},
		ReadTimeout:             15 * time.Second,
		WriteTimeout:            60 * time.Second, // TikTok/J&T lookups can be slow
		IdleTimeout:             90 * time.Second,
	})

	app.Use(recover.New())
	app.Use(requestid.New())
	// the picture upload is the only route that may carry a big body; fasthttp has already read it, so the guard keeps the
	// 1 MB limit for every other route
	app.Use(func(c *fiber.Ctx) error {
		// c.Body() would inflate a compressed body without a limit, and this runs before login and the rate limits: no
		// client of this API compresses what it sends, so a request that says it did is refused, and sizes are the raw ones
		if enc := strings.ToLower(strings.TrimSpace(c.Get(fiber.HeaderContentEncoding))); enc != "" && enc != "identity" {
			return httpx.Fail(c, fiber.StatusUnsupportedMediaType, "Dữ liệu nén không được hỗ trợ.")
		}
		raw := len(c.Request().Body())
		if declared := c.Request().Header.ContentLength(); declared > raw {
			raw = declared
		}
		if raw > maxJSONBody && !(c.Method() == fiber.MethodPost && c.Path() == "/api/v1/campaigns/images") {
			return httpx.Fail(c, fiber.StatusRequestEntityTooLarge, "Dữ liệu gửi lên quá lớn.")
		}
		return c.Next()
	})
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
		Next: func(c *fiber.Ctx) bool {
			return c.Path() == "/health" || c.Path() == "/internal/zalo/inbound" || c.Path() == "/internal/zalo/session"
		}}))
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
	if h.ZaloAccounts != nil { // the bridge hands over the credentials of a login so that they are stored
		app.Post("/internal/zalo/session", middleware.RequireBridgeSecret(cfg.ZaloBridgeSecret), h.ZaloAccounts.ReceiveSession)
	}

	// every authenticated group goes through requireAuth: it also stops the read-only guest from writing
	requireAuth := middleware.RequireAuthNoGuestWrites(authSvc)
	api := app.Group("/api/v1")

	tooMany := func(max int, window time.Duration) fiber.Handler {
		return limiter.New(limiter.Config{Max: max, Expiration: window,
			LimitReached: func(c *fiber.Ctx) error {
				return httpx.Fail(c, fiber.StatusTooManyRequests, "Bạn thao tác quá nhanh. Vui lòng thử lại sau ít phút.")
			}})
	}
	requireAdmin := middleware.RequireRole(domain.RoleAdmin)

	auth := api.Group("/auth")
	auth.Get("/options", h.Auth.Options)                           // public: is sign-up open, which captcha site key
	auth.Post("/register", tooMany(5, time.Hour), h.Auth.Register) // a person signs up once; a bot tries again and again
	auth.Post("/login", tooMany(10, time.Minute), h.Auth.Login)
	auth.Post("/email", tooMany(10, time.Minute), h.Auth.EmailAuth) // sign in, or create a member for a new email
	auth.Post("/guest", tooMany(10, time.Minute), h.Auth.Guest)     // shared read-only guest session
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
	ttshop.Get("/usage", tooMany(30, time.Minute), h.TTShop.Usage)
	ttshop.Post("/authorize", tooMany(10, 10*time.Minute), h.TTShop.Authorize)

	// read-only shop data used by the inbox (any signed-in member). Not under /ttshop: Fiber applies that group's
	// requireAdmin middleware to every path below the prefix.
	shop := api.Group("/shop", requireAuth)
	shop.Get("/open-collaborations", tooMany(60, time.Minute), h.TTShop.OpenCollabs)
	shop.Get("/open-collaboration-settings", tooMany(30, time.Minute), h.TTShop.OpenCollabSettings)

	// the master switch for what the system sends to creators by itself: anyone signed in can see it, only an admin
	// changes it
	if h.Automation != nil {
		automation := api.Group("/automation", requireAuth)
		automation.Get("/", h.Automation.Get)
		automation.Put("/", requireAdmin, tooMany(20, time.Hour), h.Automation.Set)
	}

	// campaigns of the bulk message: everybody signed in reads, only an admin changes them. Static paths before /:id.
	if h.Campaign != nil {
		campaigns := api.Group("/campaigns", requireAuth)
		campaigns.Get("/", h.Campaign.List)
		campaigns.Post("/images", requireAdmin, tooMany(30, time.Hour), h.Campaign.UploadImage)
		campaigns.Get("/images/:id", h.Campaign.Image)
		campaigns.Put("/:id", requireAdmin, tooMany(60, time.Hour), h.Campaign.SaveSteps)
	}

	// background import of KOCs: everybody signed in starts, follows and stops one; only one job runs at a time
	if h.ImportJob != nil {
		imp := api.Group("/import-jobs", requireAuth)
		imp.Post("/", tooMany(20, time.Hour), h.ImportJob.Start)
		imp.Get("/latest", h.ImportJob.Latest)
		imp.Get("/:id", h.ImportJob.Get)
		imp.Put("/:id/stop", tooMany(60, time.Hour), h.ImportJob.Stop)
	}

	// workflows: everybody signed in sees them and may start one that sends nothing; a workflow with a send block needs an
	// admin (checked in the handler, which reads the graph) and only an admin stops one. One workflow runs at a time.
	if h.Workflow != nil {
		wf := api.Group("/workflows", requireAuth)
		wf.Post("/", tooMany(10, time.Hour), h.Workflow.Start)
		wf.Get("/", h.Workflow.List)
		wf.Post("/preview", tooMany(60, time.Hour), h.Workflow.Preview)
		wf.Get("/latest", h.Workflow.Latest)
		wf.Get("/:id", h.Workflow.Get)
		wf.Put("/:id/stop", requireAdmin, tooMany(60, time.Hour), h.Workflow.Stop)
	}

	// saved workflows: everybody signed in sees and saves them; the one who made a template (or an admin) changes and deletes it
	if h.WorkflowTemplate != nil {
		tpl := api.Group("/workflow-templates", requireAuth)
		tpl.Get("/", h.WorkflowTemplate.List)
		tpl.Post("/", tooMany(60, time.Hour), h.WorkflowTemplate.Create)
		tpl.Get("/:id", h.WorkflowTemplate.Get)
		tpl.Put("/:id", tooMany(60, time.Hour), h.WorkflowTemplate.Update)
		tpl.Delete("/:id", tooMany(60, time.Hour), h.WorkflowTemplate.Delete)
	}

	// bulk runs: everybody signed in starts and follows one (the switch must be on, and only an admin turns it on); only an
	// admin changes the daily limit
	if h.BulkRun != nil {
		runs := api.Group("/bulk-runs", requireAuth)
		runs.Post("/", tooMany(10, time.Hour), h.BulkRun.Start)
		runs.Get("/current", h.BulkRun.Current)
		runs.Get("/", h.BulkRun.List)
		limit := api.Group("/bulk-limit", requireAuth)
		limit.Get("/", h.BulkRun.GetLimit)
		limit.Put("/", requireAdmin, tooMany(20, time.Hour), h.BulkRun.SetLimit)
	}

	// the switch of the bulk message: everybody signed in sees it, only an admin changes it. Not the chatbot's switch.
	if h.BulkSwitch != nil {
		bulkSwitch := api.Group("/bulk-switch", requireAuth)
		bulkSwitch.Get("/", h.BulkSwitch.Get)
		// the limit is for turning ON; turning OFF is the stop button of a run and is never refused
		limitOn := tooMany(20, time.Hour)
		bulkSwitch.Put("/", requireAdmin, func(c *fiber.Ctx) error {
			var in struct {
				Enabled bool `json:"enabled"`
			}
			if json.Unmarshal(c.Body(), &in) == nil && !in.Enabled {
				return c.Next()
			}
			return limitOn(c)
		}, h.BulkSwitch.Set)
	}

	// target collaboration: reading is open to signed-in members; sending, link and cancel reach real creators and
	// change the shop, so they are admin-only and rate limited. Static paths first, then /:id.
	target := api.Group("/target-collabs", requireAuth)
	target.Get("/config", h.Target.Config)
	target.Put("/switch", requireAdmin, tooMany(20, time.Hour), h.Target.SetSwitch)
	target.Get("/creators", tooMany(60, time.Minute), h.Target.Creators)
	target.Get("/products", tooMany(30, time.Minute), h.Target.Products)
	target.Get("/products/images", tooMany(60, time.Minute), h.Target.ProductImages)
	target.Post("/preview", tooMany(30, time.Minute), h.Target.Preview)
	target.Get("/", tooMany(60, time.Minute), h.Target.List)
	target.Post("/", requireAdmin, tooMany(5, time.Hour), h.Target.Create)
	target.Get("/:id", tooMany(60, time.Minute), h.Target.Detail)
	target.Post("/:id/link", requireAdmin, tooMany(30, time.Hour), h.Target.Link)
	target.Post("/:id/cancel", requireAdmin, tooMany(20, time.Hour), h.Target.Cancel)

	// Zalo OA webhook: served only when ZALO_WEBHOOK_SECRET is set, and then only for a caller that knows it. Without
	// the guard anybody could post a fake "KOC replied" event and drive the chatbot.
	if cfg.ZaloWebhookSecret != "" {
		guard := middleware.RequireWebhookSecret(cfg.ZaloWebhookSecret)
		api.Get("/webhooks/zalo", guard, h.Zalo.VerifyWebhook)
		api.Post("/webhooks/zalo", guard, h.Zalo.ReceiveWebhook)
	}

	api.Get("/zalo/status", requireAuth, h.Zalo.Status)
	// the Zalo account is shared by everybody: logging it in or out affects every conversation, so these are throttled
	api.Post("/zalo/qr/start", requireAuth, tooMany(10, 10*time.Minute), h.Zalo.StartQR)
	api.Post("/zalo/connect", requireAuth, tooMany(10, 10*time.Minute), h.Zalo.Connect)
	api.Post("/zalo/disconnect", requireAuth, tooMany(10, 10*time.Minute), h.Zalo.Disconnect)

	// Several personal Zalo accounts. Everybody signed in sees them and can log one in or out (as with the single
	// account before); adding or removing an account changes which numbers the company sends from: admins only.
	if za := h.ZaloAccounts; za != nil {
		api.Get("/zalo/accounts", requireAuth, za.List)
		api.Post("/zalo/accounts", requireAuth, requireAdmin, tooMany(20, 10*time.Minute), za.Create)
		api.Delete("/zalo/accounts/:id", requireAuth, requireAdmin, tooMany(20, 10*time.Minute), za.Delete)
		api.Get("/zalo/accounts/:id/status", requireAuth, za.Status)
		api.Post("/zalo/accounts/:id/qr/start", requireAuth, tooMany(10, 10*time.Minute), za.StartQR)
		api.Post("/zalo/accounts/:id/connect", requireAuth, tooMany(10, 10*time.Minute), za.Connect)
		api.Post("/zalo/accounts/:id/disconnect", requireAuth, tooMany(10, 10*time.Minute), za.Disconnect)
	}

	kocs := api.Group("/kocs", requireAuth)
	kocs.Get("/", h.Koc.List)
	kocs.Get("/digest/morning", h.Koc.MorningDigest)
	kocs.Get("/stats", h.Koc.Stats)
	// lookups call paid or rate-limited outside services; bulk import and outreach reach real creators
	kocs.Post("/lookup", tooMany(30, time.Minute), h.Koc.Lookup)
	kocs.Post("/track-shipment/lookup", tooMany(30, time.Minute), h.Koc.LookupJtTracking)
	kocs.Post("/video/lookup", tooMany(30, time.Minute), h.Koc.LookupVideo)
	kocs.Post("/outreach/run", tooMany(10, 10*time.Minute), h.Koc.RunOutreach)
	kocs.Post("/", h.Koc.Create)
	kocs.Patch("/:id/stage", h.Koc.UpdateStage)
	kocs.Get("/:id/draft", h.Koc.Draft)
	kocs.Post("/:id/sync-channel", tooMany(30, time.Minute), h.Koc.SyncChannel)
	kocs.Post("/:id/track-shipment", tooMany(30, time.Minute), h.Koc.TrackShipment)
	kocs.Post("/:id/email", tooMany(20, time.Minute), h.Koc.Email)
	kocs.Delete("/:id", h.Koc.Archive)

	kocs.Get("/:id/collabs", h.Collab.List)
	kocs.Post("/:id/collabs", h.Collab.Open)
	if h.Content != nil {
		kocs.Get("/:id/videos", tooMany(30, time.Minute), h.Content.KocVideos)
		api.Group("/content", requireAuth).Get("/videos", tooMany(30, time.Minute), h.Content.Videos)
	}
	// the links of the products in open collaboration: anyone signed in sees them, only an admin changes them
	openProducts := api.Group("/open-products", requireAuth)
	openProducts.Get("/", h.Collab.OpenProducts)
	openProducts.Post("/", requireAdmin, tooMany(20, time.Hour), h.Collab.AddOpenProducts)
	openProducts.Put("/:productId", requireAdmin, tooMany(60, time.Hour), h.Collab.SetOpenProductAds)
	openProducts.Delete("/:productId", requireAdmin, tooMany(60, time.Hour), h.Collab.DeleteOpenProduct)
	collabs := api.Group("/collabs", requireAuth)
	collabs.Get("/overview", h.Collab.Overview)
	if h.Content != nil {
		collabs.Get("/:cid/performance", tooMany(30, time.Minute), h.Content.Performance)
	}
	collabs.Patch("/:cid", h.Collab.Patch)
	collabs.Put("/:cid/open-product", h.Collab.SetOpenProduct)
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
	inbox.Put("/conversations/:conversationId/chatbot", h.Inbox.SetChatbot)
	inbox.Get("/labels", h.Inbox.ListLabels)
	inbox.Post("/labels", h.Inbox.CreateLabel)
	inbox.Delete("/labels/:labelId", h.Inbox.DeleteLabel)
	inbox.Get("/conversations/:conversationId/messages", h.Inbox.ListMessages)
	inbox.Post("/conversations/:conversationId/messages", h.Inbox.SendMessage)
	inbox.Post("/conversations/:conversationId/inbound", h.Inbox.ReceiveInbound)
	// by hand, on Zalo: ask the KOC to be friends (Zalo watches how many are sent), react to a message they sent
	inbox.Get("/conversations/:conversationId/zalo/friend", h.Inbox.ZaloFriend)
	inbox.Post("/conversations/:conversationId/zalo/friend-request", tooMany(30, time.Hour), h.Inbox.ZaloFriendRequest)
	inbox.Put("/conversations/:conversationId/messages/:messageId/reaction", tooMany(120, time.Minute), h.Inbox.ReactToMessage)
	inbox.Delete("/conversations/:conversationId/messages", h.Inbox.ClearMessages)
	inbox.Delete("/conversations/:conversationId", h.Inbox.DeleteConversation)
	inbox.Post("/kocs/:kocId/conversation", h.Inbox.OpenKocConversation)
	inbox.Delete("/conversations/:conversationId/messages/:messageId", h.Inbox.DeleteMessage)
	inbox.Post("/chatbot/parse", h.Inbox.ParseChatbot)

	return app
}
