// Package scheduler runs the periodic background automation.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tiktoday/kocflow-api/internal/metrics"
	"github.com/tiktoday/kocflow-api/internal/service"
)

const (
	sweepInterval = 4 * time.Hour
	// sweepFirstRunDelay lets the API finish starting (database, Zalo bridge) before the first sweep after a restart.
	sweepFirstRunDelay = 2 * time.Minute
	digestInterval     = 15 * time.Minute
	digestHourVN       = 8
)

// Scheduler owns the background jobs.
type Scheduler struct {
	outreach *service.OutreachService
	mailer   *service.SMTPMailer
	smtpUser string
	jobs     *metrics.Jobs

	lastDigestDate string
}

// New builds a Scheduler.
func New(outreach *service.OutreachService, mailer *service.SMTPMailer, smtpUser string, jobs *metrics.Jobs) *Scheduler {
	jobs.Register("followup-sweep", "4h")
	jobs.Register("morning-digest", "08:00 hằng ngày")
	return &Scheduler{outreach: outreach, mailer: mailer, smtpUser: smtpUser, jobs: jobs}
}

// Start launches the jobs; they stop when ctx is cancelled.
//
//   - every 4 hours: follow-up / shipping / video / return sweep
//   - 08:00 Vietnam time: morning digest emailed to the SMTP account
func (s *Scheduler) Start(ctx context.Context) {
	go s.every(ctx, sweepFirstRunDelay, sweepInterval, func(ctx context.Context) {
		start := time.Now()
		summary, err := s.outreach.RunStage2Sweep(ctx, false)
		err = sweepError(summary, err)
		s.jobs.Record("followup-sweep", start, err, summary)
		if err != nil {
			log.Printf("Background sweep error: %v", err)
		}
	})
	// no first run for the digest: its "already sent today" mark lives in memory, so running it right after a restart
	// inside the 08:00 hour would send it twice
	go s.every(ctx, 0, digestInterval, s.morningDigest)
}

// sweepError is the outcome to record for a sweep. The sweep keeps going when a KOC fails, so a sweep that skipped
// some KOCs must still show as failed (with the first cause), or the failures would pass unnoticed.
func sweepError(summary *service.SweepSummary, err error) error {
	if err == nil && summary != nil && summary.Failures > 0 {
		return fmt.Errorf("%d KOC chưa xử lý được, đầu tiên: %s", summary.Failures, summary.FirstError)
	}
	return err
}

// every runs job each d until ctx ends. A positive first also runs it once, that long after the start: without it the
// first run comes a whole interval later, and a server restarted more often than that (a deploy every few hours)
// would never run a 4-hourly job at all. A nil channel never fires, so first = 0 means "wait for the first tick".
func (s *Scheduler) every(ctx context.Context, first, d time.Duration, job func(context.Context)) {
	t := time.NewTicker(d)
	defer t.Stop()
	var firstRun <-chan time.Time
	if first > 0 {
		timer := time.NewTimer(first)
		defer timer.Stop()
		firstRun = timer.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-firstRun:
			job(ctx)
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
	start := time.Now()
	digest, err := s.outreach.BuildMorningDigest(ctx)
	if err == nil && s.smtpUser != "" {
		err = s.mailer.SendKocEmail(s.smtpUser, "Booker & Chủ shop", "tiktoday", digest.Text)
	}
	var stats any
	if digest != nil {
		stats = digest.Stats
	}
	s.jobs.Record("morning-digest", start, err, stats)
	if err != nil {
		log.Printf("Morning digest not sent: %v", err)
	}
}
