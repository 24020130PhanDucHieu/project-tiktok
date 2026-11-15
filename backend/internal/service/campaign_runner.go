package service

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// Limits and pace of a bulk run. The numbers are cautious choices, not thresholds Zalo has published.
const (
	MaxRunKocs                = 50
	MaxCampaignMessagesPerDay = 600
	stepPauseMin              = 4 * time.Second
	stepPauseMax              = 15 * time.Second
	kocPauseMin               = 3 * time.Minute
	kocPauseMax               = 8 * time.Minute
	switchPollEvery           = 5 * time.Second
	zaloWaitAtMost            = 90 * time.Second // the bridge may still be logging in just after a restart
)

// ErrRunInProgress is what Start answers while another run is running.
var ErrRunInProgress = domain.Conflict("Đang có một đợt gửi chạy. Hãy chờ đợt đó xong hoặc dừng nó.")

// RunStore is where runs are kept (*repository.BulkRunRepository).
type RunStore interface {
	Create(ctx context.Context, run *domain.BulkRun) error
	Running(ctx context.Context) (*domain.BulkRun, error)
	SetTarget(ctx context.Context, runID domain.ID, index int, t domain.RunTarget) error
	SetNextAt(ctx context.Context, runID domain.ID, at *time.Time) error
	Finish(ctx context.Context, runID domain.ID, status, reason string, at time.Time) error
	StartedSince(ctx context.Context, since time.Time) (kocs, messages int, err error)
}

// DailyLimit says how many KOCs a day may be started (*repository.BulkLimitRepository).
type DailyLimit interface{ Get(ctx context.Context) int }

// BulkSwitch is the switch of "Gửi hàng loạt" (*repository.AutomationRepository). While a run runs it is the stop button.
type BulkSwitch interface {
	Enabled(ctx context.Context) (bool, error)
	SetEnabled(ctx context.Context, enabled bool, by string) error
}

// CampaignRunner sends a campaign to a list of KOCs in the background, slowly and unevenly, one KOC at a time.
type CampaignRunner struct {
	o       *OutreachService
	runs    RunStore
	images  ImageLoader
	limit   DailyLimit
	sw      BulkSwitch
	loadKoc func(ctx context.Context, id string) (*domain.Koc, error)

	// tests replace these
	sleep   func(ctx context.Context, d time.Duration) error
	between func(min, max time.Duration) time.Duration

	wg sync.WaitGroup
}

// NewCampaignRunner builds the runner. loadKoc returns the current active KOC for an id, or nil when there is none.
func NewCampaignRunner(o *OutreachService, runs RunStore, images ImageLoader, limit DailyLimit, sw BulkSwitch,
	loadKoc func(ctx context.Context, id string) (*domain.Koc, error)) *CampaignRunner {
	return &CampaignRunner{o: o, runs: runs, images: images, limit: limit, sw: sw, loadKoc: loadKoc,
		sleep: realSleep, between: randomBetween}
}

func realSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func randomBetween(min, max time.Duration) time.Duration {
	return min + time.Duration(rand.Int63n(int64(max-min)+1))
}

// Estimate is how long a run takes: about 9.5 seconds per step and 5.5 minutes between two KOCs.
func (r *CampaignRunner) Estimate(kocs, steps int) time.Duration {
	return time.Duration(kocs) * (time.Duration(steps)*9500*time.Millisecond + 330*time.Second)
}

// Wait blocks until every background run that was started has ended (for tests and for a clean shutdown).
func (r *CampaignRunner) Wait() { r.wg.Wait() }

// Start stores a run and begins it in the background. It refuses while the switch is off, outside the send window, when
// the steps or the number of KOCs are not acceptable, and while another run is running.
func (r *CampaignRunner) Start(ctx context.Context, by, campaignName string, steps []domain.CampaignStep, targets []domain.RunTarget) (*domain.BulkRun, error) {
	if !r.o.bulkOn(ctx) {
		return nil, ErrBulkOff
	}
	if len(targets) == 0 || len(targets) > MaxRunKocs {
		return nil, domain.Invalid("Mỗi đợt gửi cho 1 đến 50 KOC.")
	}
	if err := domain.ValidateSteps(steps); err != nil {
		return nil, err
	}
	if !IsWithinAllowedSendWindow(r.o.now()) {
		return nil, domain.Conflict("Ngoài khung giờ gửi (08:30 đến 20:30). Hãy gửi vào trong khung giờ.")
	}
	for i := range targets {
		targets[i].Status, targets[i].Reason, targets[i].StepsSent, targets[i].StartedAt = domain.TargetQueued, "", 0, nil
	}
	run := &domain.BulkRun{CampaignName: campaignName, Steps: steps, Targets: targets, By: by}
	if err := r.runs.Create(ctx, run); err != nil {
		return nil, err
	}
	snapshot := cloneRun(run) // the goroutine owns run from here on; the caller gets its own copy
	r.spawn(run)
	return snapshot, nil
}

func cloneRun(run *domain.BulkRun) *domain.BulkRun {
	cp := *run
	cp.Steps = append([]domain.CampaignStep(nil), run.Steps...)
	cp.Targets = append([]domain.RunTarget(nil), run.Targets...)
	return &cp
}

// Resume continues the run that was running when the server stopped. A step that was going out at that moment may or
// may not have reached Zalo: its KOC becomes "unknown" and is never sent to again.
func (r *CampaignRunner) Resume(ctx context.Context) {
	run, err := r.runs.Running(ctx)
	if err != nil {
		log.Printf("bulk run: cannot look for a run to resume: %v", err)
		return
	}
	if run == nil {
		return
	}
	for i, t := range run.Targets {
		if t.Status == domain.TargetSending {
			t.Status, t.Reason = domain.TargetUnknown, "restart_during_step"
			run.Targets[i] = t
			if err := r.runs.SetTarget(ctx, run.ID, i, t); err != nil {
				log.Printf("bulk run: cannot mark an unfinished step: %v", err)
			}
		}
	}
	if !r.switchOn(ctx) {
		_ = r.runs.Finish(ctx, run.ID, domain.RunStopped, "switch_off", time.Now())
		return
	}
	r.spawn(run)
}

func (r *CampaignRunner) spawn(run *domain.BulkRun) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.work(run)
	}()
}

func (r *CampaignRunner) switchOn(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	on, err := r.sw.Enabled(ctx)
	return err == nil && on
}

var errSwitchOff = errors.New("bulk switch is off")

// nap waits for d, in short slices so that turning the switch off ends the wait at once.
func (r *CampaignRunner) nap(ctx context.Context, d time.Duration) error {
	for d > 0 {
		slice := d
		if slice > switchPollEvery {
			slice = switchPollEvery
		}
		if err := r.sleep(ctx, slice); err != nil {
			return err
		}
		d -= slice
		if !r.switchOn(ctx) {
			return errSwitchOff
		}
	}
	return nil
}

// stopReason says why the run must not start the next KOC, or "" when it may.
func (r *CampaignRunner) stopReason(ctx context.Context, run *domain.BulkRun) string {
	if !r.switchOn(ctx) {
		return "switch_off"
	}
	if !IsWithinAllowedSendWindow(r.o.now()) {
		return "outside_0830_2030_window"
	}
	for waited := time.Duration(0); !r.o.zalo.Personal().IsConnected(ctx); waited += switchPollEvery {
		if waited >= zaloWaitAtMost {
			return "zalo_not_ready"
		}
		if err := r.nap(ctx, switchPollEvery); err != nil {
			return "switch_off"
		}
	}
	kocs, messages, err := r.runs.StartedSince(ctx, startOfVNDay(r.o.now()))
	if err != nil {
		return "store_unavailable"
	}
	if kocs >= r.limit.Get(ctx) {
		return "daily_koc_limit_reached"
	}
	if messages+len(run.Steps) > MaxCampaignMessagesPerDay {
		return "daily_message_cap_reached"
	}
	return ""
}

// work runs the queued KOCs of the run one after the other. Every way out turns the switch off.
func (r *CampaignRunner) work(run *domain.BulkRun) {
	ctx := context.Background()
	finished := false
	finish := func(status, reason string) {
		finished = true
		// a run that is never marked as ended would block every later run, so the end is tried again before giving up
		var err error
		for try := 1; try <= 5; try++ {
			if err = r.runs.Finish(ctx, run.ID, status, reason, time.Now()); err == nil {
				return
			}
			_ = r.sleep(ctx, time.Duration(try)*time.Second)
		}
		log.Printf("bulk run %s: cannot record the end: %v", run.ID, err)
	}
	defer func() {
		if p := recover(); p != nil {
			log.Printf("bulk run %s: panic: %v", run.ID, p)
			if !finished {
				finish(domain.RunStopped, "internal_error")
			}
		}
		if err := r.sw.SetEnabled(ctx, false, "system"); err != nil {
			log.Printf("bulk run %s: cannot turn the switch off: %v", run.ID, err)
		}
	}()

	faults := 0 // KOCs in a row that ended because of Zalo
	for i := range run.Targets {
		t := run.Targets[i]
		if t.Status != domain.TargetQueued {
			continue
		}
		if reason := r.stopReason(ctx, run); reason != "" {
			finish(domain.RunStopped, reason)
			return
		}
		koc, err := r.loadKoc(ctx, t.KocID)
		if err != nil {
			t.Status, t.Reason = domain.TargetFailed, "store_unavailable"
			run.Targets[i] = t
			_ = r.runs.SetTarget(ctx, run.ID, i, t)
			continue
		}
		if koc != nil {
			t.KocName, t.Handle = koc.DisplayName, "@"+koc.TiktokID
		}
		index := i
		hooks := SequenceHooks{
			Pause: func(ctx context.Context) error {
				return r.nap(ctx, r.between(stepPauseMin, stepPauseMax))
			},
			Progress: func(p domain.RunTarget) error {
				run.Targets[index] = p
				return r.runs.SetTarget(ctx, run.ID, index, p)
			},
		}
		out, fault := r.o.SendSequence(ctx, koc, run.Steps, r.images, t, hooks)
		run.Targets[i] = out
		if err := r.runs.SetTarget(ctx, run.ID, i, out); err != nil {
			log.Printf("bulk run %s: cannot store KOC %d: %v", run.ID, i, err)
		}

		switch {
		case out.Reason == "stopped":
			finish(domain.RunStopped, "switch_off")
			return
		case fault:
			faults++
			if faults >= 2 {
				finish(domain.RunStopped, "zalo_errors")
				return
			}
		default:
			faults = 0
		}

		if out.Status == domain.TargetSkipped || !hasQueued(run.Targets[i+1:]) {
			continue // nothing was sent to this KOC, or nobody is left: no long wait
		}
		wait := r.between(kocPauseMin, kocPauseMax)
		next := time.Now().Add(wait)
		_ = r.runs.SetNextAt(ctx, run.ID, &next)
		if err := r.nap(ctx, wait); err != nil {
			finish(domain.RunStopped, "switch_off")
			return
		}
		_ = r.runs.SetNextAt(ctx, run.ID, nil)
	}
	finish(domain.RunDone, "")
}

func hasQueued(ts []domain.RunTarget) bool {
	for _, t := range ts {
		if t.Status == domain.TargetQueued {
			return true
		}
	}
	return false
}
