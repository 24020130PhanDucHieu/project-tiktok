package service

import (
	"context"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// ReplyKocs, ReplyThreads and ReplyMsgs are what the reply lookup reads (*repository.KocRepository,
// *repository.ConversationRepository, *repository.MessageRepository). It only reads: it never creates a thread.
type ReplyKocs interface {
	FindActiveByID(ctx context.Context, id string) (*domain.Koc, error)
}
type ReplyThreads interface {
	FindByHandle(ctx context.Context, handle string) (*domain.Conversation, error)
}
type ReplyMsgs interface {
	InboundSince(ctx context.Context, convID domain.ID, since time.Time) (bool, error)
}

// ReplyLookup tells whether a KOC answered after a given moment. It looks at the same thread a campaign sends in (the one
// of the KOC's handle), so the workflow and the campaign never disagree about who answered.
type ReplyLookup struct {
	kocs    ReplyKocs
	threads ReplyThreads
	msgs    ReplyMsgs
}

// NewReplyLookup builds the lookup.
func NewReplyLookup(kocs ReplyKocs, threads ReplyThreads, msgs ReplyMsgs) *ReplyLookup {
	return &ReplyLookup{kocs: kocs, threads: threads, msgs: msgs}
}

// RepliedSince says whether the KOC wrote after `since`. known is false when nobody can tell: the KOC is gone or archived,
// or has no thread. A failing store is an error, not a guess.
func (l *ReplyLookup) RepliedSince(ctx context.Context, kocID string, since time.Time) (replied bool, known bool, err error) {
	koc, err := l.kocs.FindActiveByID(ctx, kocID)
	if err != nil || koc == nil {
		return false, false, err
	}
	conv, err := l.threads.FindByHandle(ctx, "@"+koc.TiktokID)
	if err != nil || conv == nil {
		return false, false, err
	}
	replied, err = l.msgs.InboundSince(ctx, conv.ID, since)
	if err != nil {
		return false, false, err
	}
	return replied, true, nil
}
