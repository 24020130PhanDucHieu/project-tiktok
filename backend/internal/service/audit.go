package service

import (
	"context"
	"log"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// AuditService records security-relevant actions without slowing requests down.
type AuditService struct{ repo AuditStore }

// NewAuditService builds an AuditService.
func NewAuditService(repo AuditStore) *AuditService {
	return &AuditService{repo: repo}
}

// Log stores an entry asynchronously (failures are logged, never returned). Safe on a nil receiver.
func (a *AuditService) Log(_ context.Context, actorID, actorEmail, action, targetType, targetID, detail, ip string) {
	if a == nil || a.repo == nil {
		return
	}
	entry := &domain.AuditLog{ActorID: actorID, ActorEmail: actorEmail, Action: action,
		TargetType: targetType, TargetID: targetID, Detail: detail, IP: ip}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.repo.Insert(ctx, entry); err != nil {
			log.Printf("audit log write failed: %v", err)
		}
	}()
}

// List returns audit entries.
func (a *AuditService) List(ctx context.Context, action, q string, page, limit int64) ([]domain.AuditLog, int64, error) {
	return a.repo.List(ctx, action, q, (page-1)*limit, limit)
}
