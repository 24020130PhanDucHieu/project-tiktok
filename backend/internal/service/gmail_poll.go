package service

import (
	"context"
	"log"
	"time"
	"unicode/utf8"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// maxInboundMailBytes caps the stored text of one email reply.
const maxInboundMailBytes = 4000

// truncateUTF8 cuts s to at most max bytes without splitting a multi-byte character: a Vietnamese letter takes 2 or
// 3 bytes, and a string cut in the middle of one is invalid UTF-8, which MongoDB refuses to store.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// GmailPoller copies KOC email replies into the matching chat thread.
type GmailPoller struct {
	gmail *GmailService
	kocs  MailKocs
	convs MailThreads
	msgs  MailMsgs
}

// NewGmailPoller builds a GmailPoller.
func NewGmailPoller(g *GmailService, kocs MailKocs, convs MailThreads, msgs MailMsgs) *GmailPoller {
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
	text := truncateUTF8(m.Text, maxInboundMailBytes)
	if err := p.msgs.Create(ctx, &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionInbound, Channel: domain.ChannelEmail,
		To: m.From, Subject: m.Subject, Text: text, Status: domain.MessageDelivered, SentAt: m.At, ExternalID: "gmail:" + m.ID}); err != nil {
		return err
	}
	return p.convs.BumpInbound(ctx, conv.ID, text, m.At)
}
