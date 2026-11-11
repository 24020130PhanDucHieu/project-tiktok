package service

import (
	"context"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// The ports of the service layer: what a service needs from the outside, written as the service wants to use it. The
// repository package implements them; the services never import it, so every use case can be tested with plain fakes.

// OutreachKocs is what outreach, follow-up and the daily sweep need from the KOC store.
type OutreachKocs interface {
	Save(ctx context.Context, koc *domain.Koc) error
	FindActiveByContact(ctx context.Context, tiktokID string, phones ...string) (*domain.Koc, error)
	FindActive(ctx context.Context) ([]*domain.Koc, error)
	FindDormantDue(ctx context.Context, now time.Time) ([]*domain.Koc, error)
	FindToChase(ctx context.Context) ([]*domain.Koc, error)
	FindInShipping(ctx context.Context) ([]*domain.Koc, error)
	FindDelivered(ctx context.Context) ([]*domain.Koc, error)
	FindNotFitAwaitingReturn(ctx context.Context) ([]*domain.Koc, error)
}

// OutreachConvs is what outreach needs from the conversation store.
type OutreachConvs interface {
	UpsertKocMessage(ctx context.Context, c domain.KocContact, at time.Time) (*domain.Conversation, error)
	Save(ctx context.Context, c *domain.Conversation) error
	// the field updates: use these instead of Save, which replaces what another writer just changed
	SetLastMessageAt(ctx context.Context, id domain.ID, text string, at time.Time) error
	SetZaloContact(ctx context.Context, id domain.ID, c domain.ZaloContact) error
	FindByExternal(ctx context.Context, externalID string) (*domain.Conversation, error)
	FindUnlinkedKocThreads(ctx context.Context, limit int64) ([]domain.Conversation, error)
	ReleaseZalo(ctx context.Context, id domain.ID) error
}

// OutreachMsgs is what outreach and the chatbot need from the message store.
type OutreachMsgs interface {
	Create(ctx context.Context, m *domain.Message) error
	Recent(ctx context.Context, convID domain.ID, n int64) ([]domain.Message, error)
	// CountProactiveSince counts the messages the system started on its own since the given time (the daily quota).
	CountProactiveSince(ctx context.Context, since time.Time) (int64, error)
}

// KocDirectory is what the KOC use cases need from the KOC store.
type KocDirectory interface {
	Search(ctx context.Context, q domain.KocQuery, skip, limit int64) ([]domain.Koc, int64, error)
	FindByIdentity(ctx context.Context, tiktokID, phone string) (*domain.Koc, error)
	FindActiveByID(ctx context.Context, id string) (*domain.Koc, error)
	FindQualifiedNotCollaborated(ctx context.Context) ([]*domain.Koc, error)
	FindRecentActive(ctx context.Context, limit int64) ([]domain.Koc, error)
	Create(ctx context.Context, koc *domain.Koc) error
	Save(ctx context.Context, koc *domain.Koc) error
	Archive(ctx context.Context, id string) (*domain.Koc, error)
}

// KocThreads is what the KOC use cases need from the conversation store.
type KocThreads interface {
	UpsertKocContact(ctx context.Context, c domain.KocContact) (*domain.Conversation, error)
	EnsureKocContact(ctx context.Context, c domain.KocContact) (*domain.Conversation, error)
}

// MailKocs, MailThreads and MailMsgs are what the Gmail poller needs.
type MailKocs interface {
	FindActiveByEmail(ctx context.Context, email string) ([]*domain.Koc, error)
}

// MailThreads is the conversation side of the Gmail poller.
type MailThreads interface {
	FindByHandle(ctx context.Context, handle string) (*domain.Conversation, error)
	BumpInbound(ctx context.Context, id domain.ID, text string, at time.Time) error
}

// MailMsgs is the message side of the Gmail poller.
type MailMsgs interface {
	ExistsExternal(ctx context.Context, externalID string) (bool, error)
	Create(ctx context.Context, m *domain.Message) error
}

// UserStore is what the account use cases (sign-in, profile, admin) need from the user store.
type UserStore interface {
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByID(ctx context.Context, id string) (*domain.User, error)
	Create(ctx context.Context, u *domain.User) error
	Patch(ctx context.Context, id domain.ID, ch domain.UserChange) error
	Rename(ctx context.Context, id domain.ID, name string) error
	SetPassword(ctx context.Context, id domain.ID, hash string, at time.Time) error
	ClearLock(ctx context.Context, id domain.ID) error
	RecordLoginSuccess(ctx context.Context, id domain.ID) error
	RecordLoginFailure(ctx context.Context, u *domain.User, max int, lock time.Duration) error
	List(ctx context.Context, q domain.UserQuery) ([]domain.User, int64, error)
	CountAdmins(ctx context.Context) (int64, error)
	CountActiveAdmins(ctx context.Context) (int64, error)
	Counts(ctx context.Context, activeSince time.Time) domain.UserCounts
	Delete(ctx context.Context, id domain.ID) error
}

// ResetStore is the password-reset code store.
type ResetStore interface {
	Create(ctx context.Context, p *domain.PasswordReset) error
	Active(ctx context.Context, userID domain.ID) (*domain.PasswordReset, error)
	Fail(ctx context.Context, id domain.ID) error
	MarkUsed(ctx context.Context, id domain.ID) (bool, error)
}

// AuditStore is the audit trail store.
type AuditStore interface {
	Insert(ctx context.Context, a *domain.AuditLog) error
	List(ctx context.Context, action, q string, skip, limit int64) ([]domain.AuditLog, int64, error)
}
