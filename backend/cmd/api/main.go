// Command api starts the KocFlow HTTP API.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/tiktoday/kocflow-api/internal/campaignseed"
	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/database"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/metrics"
	"github.com/tiktoday/kocflow-api/internal/platform"
	"github.com/tiktoday/kocflow-api/internal/repository"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/scheduler"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// version is injected at build time: -ldflags "-X main.version=<git sha>".
var version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatalf("Unable to start API: %v", err)
	}
}

func run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, db, err := database.Connect(ctx, cfg.MongoURI, cfg.MongoDBName)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(shutdownCtx)
	}()
	if err := database.EnsureIndexes(ctx, db); err != nil {
		return err
	}

	// repositories
	kocRepo := repository.NewKocRepository(db)
	convRepo := repository.NewConversationRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	userRepo := repository.NewUserRepository(db)
	resetRepo := repository.NewResetRepository(db)
	auditRepo := repository.NewAuditRepository(db)

	// services
	mailer := service.NewSMTPMailer(cfg)
	bridge := platform.NewZaloBridge(cfg)
	zalo := service.NewZaloService(cfg, bridge, platform.NewZaloOAClient())
	outreach := service.NewOutreachService(kocRepo, convRepo, msgRepo, mailer,
		platform.NewJTClient(cfg), platform.NewTikTokWeb(cfg), zalo)
	audit := service.NewAuditService(auditRepo)
	automation := repository.NewAutomationRepository(db)
	outreach.WithAutomation(automation) // the master switch: off until an admin turns it on
	bulkSwitch := repository.NewBulkSwitchRepository(db)
	outreach.WithBulkSwitch(bulkSwitch) // its own switch for the bulk message: off until an admin turns it on
	gmailSvc := service.NewGmailService(cfg, repository.NewGmailAccountRepository(db), platform.NewGoogleClient())
	authSvc := service.NewAuthService(userRepo, resetRepo, audit, mailer, cfg)
	if cfg.TurnstileSecret != "" { // without it (development) the sign-up form has no captcha; production refuses that
		authSvc.WithCaptcha(platform.NewTurnstileClient(cfg.TurnstileSecret))
	}
	kocSvc := service.NewKocService(kocRepo, convRepo, outreach)
	statsSvc := service.NewStatsService(kocRepo)
	collector := metrics.New()
	jobs := metrics.NewJobs()
	adminSvc := service.NewAdminService(userRepo, authSvc, audit, repository.NewDatabaseInspector(db), bridge, mailer, collector, jobs, version)

	if err := authSvc.BootstrapAdmin(ctx); err != nil {
		return err
	}

	ttShop := service.NewTTShopService(cfg, repository.NewTTSTokenRepository(db)).WithReadCache(repository.NewReadCacheRepository(db))
	// a KOC can only be added when TikTok Shop lists the creator (Nhà sáng tạo)
	kocSvc.WithCreatorVerifier(ttShop)
	tiktokPublic := platform.NewTikTokWeb(cfg)
	collabRepo := repository.NewCollabRepository(db)
	campaignRepo := repository.NewCampaignRepository(db)
	importRepo := repository.NewImportJobRepository(db)
	importRunner := service.NewImportRunner(kocSvc, importRepo) // the worker of the background import of KOCs from Excel
	runRepo, limitRepo := repository.NewBulkRunRepository(db), repository.NewBulkLimitRepository(db)
	runner := service.NewCampaignRunner(outreach, runRepo, campaignRepo, limitRepo, bulkSwitch, kocRepo.FindActiveByID)
	if err := campaignRepo.EnsureSeed(ctx, campaignseed.Campaigns()); err != nil { // the three first campaigns; a failure must not stop the API
		log.Printf("campaigns: could not store the first campaigns: %v", err)
	}
	// the workflows of the Quy trình tab: they drive the import runner and the campaign runner above, and read the replies
	workflowRepo := repository.NewWorkflowRepository(db)
	workflowRunner := service.NewWorkflowRunner(workflowRepo,
		struct { // starts import jobs and reads them
			*service.ImportRunner
			*repository.ImportJobRepository
		}{importRunner, importRepo},
		struct { // starts bulk runs and reads them
			*service.CampaignRunner
			*repository.BulkRunRepository
		}{runner, runRepo},
		bulkSwitch, service.NewReplyLookup(kocRepo, convRepo, msgRepo))
	workflowRunner.
		// the pick block reads the KOC directory; the label block reads a KOC and its conversation and adds the label; the stage
		// block moves KOCs with the action a booker uses
		WithPicker(service.NewKocPicker(kocRepo)).
		WithLabeler(struct {
			*repository.KocRepository
			*repository.ConversationRepository
		}{kocRepo, convRepo}).
		WithStages(kocSvc).
		WithEmailer(service.NewEmailSender(kocRepo, mailer, nil))

	app := router.New(cfg, authSvc, router.Handlers{
		Auth:             handler.NewAuthHandler(authSvc, audit),
		Koc:              handler.NewKocHandler(kocSvc, outreach, statsSvc, audit),
		Inbox:            handler.NewInboxHandler(kocRepo, convRepo, msgRepo, outreach, gmailSvc),
		Gmail:            handler.NewGmailHandler(gmailSvc, cfg.FrontendURL),
		Collab:           handler.NewCollabHandler(kocRepo, collabRepo, convRepo, msgRepo).WithOpenCollabs(ttShop).WithOpenProducts(repository.NewOpenProductRepository(db)),
		Zalo:             handler.NewZaloHandler(bridge, convRepo, msgRepo, outreach, audit),
		Admin:            handler.NewAdminHandler(adminSvc, audit, statsSvc),
		TTShop:           handler.NewTTShopHandler(ttShop, audit),
		Automation:       handler.NewAutomationHandler(automation, audit),
		BulkSwitch:       handler.NewBulkSwitchHandler(bulkSwitch, audit),
		Campaign:         handler.NewCampaignHandler(campaignRepo, audit),
		ImportJob:        handler.NewImportJobHandler(importRunner, importRepo, audit),
		Workflow:         handler.NewWorkflowHandler(workflowRunner, workflowRepo, campaignRepo, audit),
		WorkflowTemplate: handler.NewWorkflowTemplateHandler(repository.NewWorkflowTemplateRepository(db), audit),
		BulkRun:          handler.NewBulkRunHandler(runner, runRepo, limitRepo, kocSvc, campaignRepo, audit),
		Target:           handler.NewTargetCollabHandler(ttShop, repository.NewTargetInviteRepository(db), audit, cfg.TTSTargetInviteMaxCreators),
		Content:          handler.NewContentHandler(ttShop, tiktokPublic, kocRepo, collabRepo),
	}, collector)

	importRunner.Resume(ctx)   // an import that was running when the server stopped goes on with the rows still queued
	runner.Resume(ctx)         // a run that was running when the server stopped goes on (a step that was going out becomes "unknown")
	workflowRunner.Resume(ctx) // after the two above: a workflow follows the jobs they continue (it finds them by their tags)

	// Tự động kích hoạt sidecar Zalo Bridge nếu đang chạy local mà chưa mở
	cleanupBridge := bridge.EnsureBridgeProcess(ctx)
	defer cleanupBridge()

	// Log the personal Zalo account in, and again whenever the bridge has lost its session (it may restart on its own, even
	// on another machine); the API must not wait for the bridge.
	go bridge.KeepConnected(ctx, 30*time.Second)
	go service.NewGmailPoller(gmailSvc, kocRepo, convRepo, msgRepo).Run(ctx, 2*time.Minute)
	scheduler.New(outreach, mailer, cfg.SMTPUser, jobs).Start(ctx)

	errCh := make(chan error, 1)
	go func() {
		log.Printf("API listening on http://127.0.0.1:%s", cfg.Port)
		errCh <- app.Listen(":" + cfg.Port)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Println("Shutting down...")
		return app.ShutdownWithTimeout(10 * time.Second)
	}
}
