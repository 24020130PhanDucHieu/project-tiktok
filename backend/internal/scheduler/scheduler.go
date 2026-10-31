// Package scheduler runs the periodic background automation.
package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/tiktoday/kocflow-api/internal/service"
)

const (
	sweepInterval  = 4 * time.Hour
	digestInterval = 15 * time.Minute
	digestHourVN   = 8
)

// Scheduler owns the background jobs.
type Scheduler struct {
	outreach *service.OutreachService
	mailer   *service.SMTPMailer
	smtpUser string

	lastDigestDate string
}

// New builds a Scheduler.
func New(outreach *service.OutreachService, mailer *service.SMTPMailer, smtpUser string) *Scheduler {
	return &Scheduler{outreach: outreach, mailer: mailer, smtpUser: smtpUser}
}

// Start launches the jobs; they stop when ctx is cancelled.
//
//   - every 4 hours: follow-up / shipping / video / return sweep
//   - 08:00 Vietnam time: morning digest emailed to the SMTP account
func (s *Scheduler) Start(ctx context.Context) {
	go s.every(ctx, sweepInterval, func(ctx context.Context) {
		if _, err := s.outreach.RunStage2Sweep(ctx, false); err != nil {
			log.Printf("Background sweep error: %v", err)
		}
	})
	go s.every(ctx, digestInterval, s.morningDigest)
}

func (s *Scheduler) every(ctx context.Context, d time.Duration, job func(context.Context)) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			job(ctx)
		}
	}
}

func (s *Scheduler) morningDigest(ctx context.Context) {
	now := time.Now()
	vnHour := now.In(time.FixedZone("ICT", 7*3600)).Hour()
	today := now.UTC().Format("2006-01-02")
	if vnHour != digestHourVN || s.lastDigestDate == today {
		return
	}
	s.lastDigestDate = today
	digest, err := s.outreach.BuildMorningDigest(ctx)
	if err != nil || s.smtpUser == "" {
		return // SMTP not configured: skip silently
	}
	if err := s.mailer.SendKocEmail(s.smtpUser, "Booker & Chủ shop", "tiktoday", digest.Text); err != nil {
		log.Printf("Morning digest not sent: %v", err)
	}
}
