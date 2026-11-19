package service

import (
	"context"
	"errors"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// EmailStatus is how one email to one KOC went.
type EmailStatus string

const (
	EmailSent      EmailStatus = "sent"       // the email went out
	EmailNoAddress EmailStatus = "no_address" // the KOC has no email address
	EmailBlocked   EmailStatus = "blocked"    // the KOC is gone, archived, or must not be contacted
	EmailRecent    EmailStatus = "recent"     // the KOC was contacted in the last 24 hours (by any channel)
	EmailFailed    EmailStatus = "failed"     // the email could not be sent; the error says why
)

// EmailKocs is what the sender needs of the KOC store (*repository.KocRepository).
type EmailKocs interface {
	FindActiveByID(ctx context.Context, id string) (*domain.Koc, error)
	ClaimContact(ctx context.Context, id domain.ID, notAfter, at time.Time) (claimed bool, previous *time.Time, err error)
	ReleaseContact(ctx context.Context, id domain.ID, at time.Time, previous *time.Time) error
}

// SubjectMailer sends a plain-text email with a subject of its own (*SMTPMailer).
type SubjectMailer interface {
	Send(to, subject, text string) error
}

// EmailSender sends one email to one KOC for a workflow. It marks the KOC as contacted BEFORE the email goes out (the same
// mark a campaign uses), so a KOC contacted in the last 24 hours gets nothing, and a step that runs again after a restart
// cannot email the same KOC twice.
type EmailSender struct {
	kocs EmailKocs
	mail SubjectMailer
	now  func() time.Time
}

// NewEmailSender builds the sender.
func NewEmailSender(kocs EmailKocs, mail SubjectMailer, now func() time.Time) *EmailSender {
	if now == nil {
		now = time.Now
	}
	return &EmailSender{kocs: kocs, mail: mail, now: now}
}

// SendEmailTo emails one KOC. subject and text may hold {ten} and {kenh}; an empty subject becomes a default one. The error is
// set when the status is EmailFailed. When SMTP is not configured nothing was sent, so the mark is given back; for any other
// failure the mail may or may not have gone out, so the mark stays and the KOC is never emailed again by a retry.
func (s *EmailSender) SendEmailTo(ctx context.Context, kocID, subject, text string) (EmailStatus, error) {
	koc, err := s.kocs.FindActiveByID(ctx, kocID)
	if err != nil {
		return EmailFailed, err
	}
	if koc == nil || koc.Dnc || koc.DaHopTac {
		return EmailBlocked, nil
	}
	if koc.Email == "" {
		return EmailNoAddress, nil
	}
	now := s.now()
	claimed, previous, err := s.kocs.ClaimContact(ctx, koc.ID, now.Add(-dayDuration), now)
	if err != nil {
		return EmailFailed, err
	}
	if !claimed {
		return EmailRecent, nil
	}
	if subject == "" {
		subject = "Cơ hội hợp tác TikTok dành cho " + koc.DisplayName
	}
	if err := s.mail.Send(koc.Email, RenderTemplate(subject, koc), RenderTemplate(text, koc)); err != nil {
		if errors.Is(err, ErrEmailNotConfigured) {
			_ = s.kocs.ReleaseContact(ctx, koc.ID, now, previous) // nothing was sent: the KOC is not "contacted"
		}
		return EmailFailed, err
	}
	return EmailSent, nil
}
