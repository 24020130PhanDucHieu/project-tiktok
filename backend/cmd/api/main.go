// Command api starts the KocFlow HTTP API.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/database"
	"github.com/tiktoday/kocflow-api/internal/handler"
	"github.com/tiktoday/kocflow-api/internal/repository"
	"github.com/tiktoday/kocflow-api/internal/router"
	"github.com/tiktoday/kocflow-api/internal/scheduler"
	"github.com/tiktoday/kocflow-api/internal/service"
)

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

	// services
	mailer := service.NewSMTPMailer(cfg)
	bridge := service.NewBridgeClient(cfg)
	zalo := service.NewZaloService(cfg, bridge)
	outreach := service.NewOutreachService(kocRepo, convRepo, msgRepo, mailer,
		service.NewJTService(cfg), service.NewTikTokService(cfg), zalo)
	authSvc := service.NewAuthService(userRepo, cfg)
	kocSvc := service.NewKocService(kocRepo, convRepo, outreach)

	app := router.New(cfg, authSvc, router.Handlers{
		Auth:  handler.NewAuthHandler(authSvc),
		Koc:   handler.NewKocHandler(kocSvc, outreach),
		Inbox: handler.NewInboxHandler(kocRepo, convRepo, msgRepo, outreach),
		Zalo:  handler.NewZaloHandler(bridge, convRepo, msgRepo, outreach),
	})

	// Tự động kích hoạt sidecar Zalo Bridge nếu đang chạy local mà chưa mở
	cleanupBridge := bridge.EnsureBridgeProcess(ctx)
	defer cleanupBridge()

	// Reconnect the personal Zalo account in the background; the API must not wait for the bridge.
	go bridge.InitializeFromEnv(ctx)
	scheduler.New(outreach, mailer, cfg.SMTPUser).Start(ctx)

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
