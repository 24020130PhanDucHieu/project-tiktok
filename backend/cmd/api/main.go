// Command api starts the KocFlow HTTP API.
package main

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/database"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/metrics"
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
	bridge := service.NewBridgeClient(cfg)
	zalo := service.NewZaloService(cfg, bridge)
	outreach := service.NewOutreachService(kocRepo, convRepo, msgRepo, mailer,
		service.NewJTService(cfg), service.NewTikTokService(cfg), zalo)
	audit := service.NewAuditService(auditRepo)
	gmailSvc := service.NewGmailService(cfg, db)
	authSvc := service.NewAuthService(userRepo, resetRepo, audit, mailer, cfg)
	kocSvc := service.NewKocService(kocRepo, convRepo, outreach)
	statsSvc := service.NewStatsService(kocRepo)
	collector := metrics.New()
	jobs := metrics.NewJobs()
	adminSvc := service.NewAdminService(userRepo, authSvc, audit, db, bridge, mailer, collector, jobs, version)

	if err := authSvc.BootstrapAdmin(ctx); err != nil {
		return err
	}
	// one-time back-fill of chat threads for KOCs that pre-date thread-on-create
	if all, err := kocRepo.Find(ctx, bson.M{"status": domain.KocActive}); err == nil {
		_ = convRepo.EnsureForKocs(ctx, all)
	}

	ttShop := service.NewTTShopService(cfg, db)

	app := router.New(cfg, authSvc, router.Handlers{
		Auth:   handler.NewAuthHandler(authSvc, audit),
		Koc:    handler.NewKocHandler(kocSvc, outreach, statsSvc, audit),
		Inbox:  handler.NewInboxHandler(kocRepo, convRepo, msgRepo, outreach, gmailSvc),
		Gmail:  handler.NewGmailHandler(gmailSvc, cfg.FrontendURL),
		Collab: handler.NewCollabHandler(kocRepo, repository.NewCollabRepository(db), convRepo, msgRepo).WithOpenCollabs(ttShop),
		Zalo:   handler.NewZaloHandler(bridge, convRepo, msgRepo, outreach, audit),
		Admin:  handler.NewAdminHandler(adminSvc, audit, statsSvc),
		TTShop: handler.NewTTShopHandler(ttShop, audit),
		Target: handler.NewTargetCollabHandler(ttShop, repository.NewTargetInviteRepository(db), audit, cfg.TTSTargetInvitesEnabled, cfg.TTSTargetInviteMaxCreators),
	}, collector)

	// Tự động kích hoạt sidecar Zalo Bridge nếu đang chạy local mà chưa mở
	cleanupBridge := bridge.EnsureBridgeProcess(ctx)
	defer cleanupBridge()

	// Reconnect the personal Zalo account in the background; the API must not wait for the bridge.
	go bridge.InitializeFromEnv(ctx)
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
