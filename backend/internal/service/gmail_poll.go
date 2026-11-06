package service

import (
	"context"
	"log"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

// GmailPoller copies KOC email replies into the matching chat thread.
type GmailPoller struct {
	gmail *GmailService
	kocs  *repository.KocRepository
	convs *repository.ConversationRepository
	msgs  *repository.MessageRepository
}

// NewGmailPoller builds a GmailPoller.
func NewGmailPoller(g *GmailService, kocs *repository.KocRepository, convs *repository.ConversationRepository, msgs *repository.MessageRepository) *GmailPoller {
	return &GmailPoller{gmail: g, kocs: kocs, convs: convs, msgs: msgs}
}

// Run polls every interval until ctx ends. It does nothing when Google OAuth is not configured.
func (p *GmailPoller) Run(ctx context.Context, every time.Duration) {
	if !p.gmail.Configured() {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := p.PollOnce(ctx); err != nil {
				log.Printf("gmail poll: %v", err)
			}
		}
	}
}

// PollOnce ingests new replies for every linked account and returns how many were stored.
func (p *GmailPoller) PollOnce(ctx context.Context) error {
	accounts, err := p.gmail.Linked(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		mails, err := p.gmail.FetchInbound(ctx, a.UserID)
		if err != nil {
			log.Printf("gmail fetch %s: %v", a.Email, err)
			continue
		}
		for _, m := range mails {
			if err := p.ingest(ctx, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *GmailPoller) ingest(ctx context.Context, m InboundMail) error {
	if seen, err := p.msgs.ExistsExternal(ctx, "gmail:"+m.ID); err != nil || seen {
		return err
	}
	kocs, err := p.kocs.FindActiveByEmail(ctx, m.From)
	if err != nil || len(kocs) == 0 {
		return err // replies from unknown senders are ignored
	}
	conv, err := p.convs.FindByHandle(ctx, "@"+kocs[0].TiktokID)
	if err != nil || conv == nil {
		return err
	}
	text := m.Text
	if len(text) > 4000 {
		text = text[:4000]
	}
	if err := p.msgs.Create(ctx, &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionInbound, Channel: domain.ChannelEmail,
		To: m.From, Subject: m.Subject, Text: text, Status: domain.MessageDelivered, SentAt: m.At, ExternalID: "gmail:" + m.ID}); err != nil {
		return err
	}
	return p.convs.BumpInbound(ctx, conv.ID, text, m.At)
}
