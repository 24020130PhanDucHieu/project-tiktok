package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// memSwitch is the bulk switch in memory.
type memSwitch struct {
	mu sync.Mutex
	on bool
}

func (s *memSwitch) Enabled(context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.on, nil
}
func (s *memSwitch) SetEnabled(_ context.Context, on bool, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.on = on
	return nil
}

// memRuns keeps runs in memory and behaves like the repository (one running run, Finish turns queued into not_sent).
type memRuns struct {
	mu          sync.Mutex
	runs        []*domain.BulkRun
	nextAtSet   int
	nextAtWaits []time.Time
	extraKocs   int // KOCs started today by other runs
	finishFails int // Finish fails this many times before it works
}

func (m *memRuns) Create(_ context.Context, run *domain.BulkRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.Status == domain.RunRunning {
			return ErrRunInProgress
		}
	}
	run.ID, run.CreatedAt, run.Status = domain.NewID(), time.Now(), domain.RunRunning
	m.runs = append(m.runs, cloneRun(run)) // a database keeps its own copy: what the runner changes is lost unless it is stored
	return nil
}
func (m *memRuns) find(id domain.ID) *domain.BulkRun {
	for _, r := range m.runs {
		if r.ID == id {
			return r
		}
	}
	return nil
}
func (m *memRuns) Running(context.Context) (*domain.BulkRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.Status == domain.RunRunning {
			return cloneRun(r), nil
		}
	}
	return nil, nil
}
func (m *memRuns) SetTarget(_ context.Context, id domain.ID, i int, t domain.RunTarget) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.find(id).Targets[i] = t
	return nil
}
func (m *memRuns) SetNextAt(_ context.Context, id domain.ID, at *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if at != nil {
		m.nextAtSet++
		m.nextAtWaits = append(m.nextAtWaits, *at)
	}
	m.find(id).NextAt = at
	return nil
}
func (m *memRuns) Finish(_ context.Context, id domain.ID, status, reason string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.finishFails > 0 {
		m.finishFails--
		return errors.New("db down")
	}
	r := m.find(id)
	r.Status, r.StopReason, r.FinishedAt, r.NextAt = status, reason, &at, nil
	for i := range r.Targets {
		if r.Targets[i].Status == domain.TargetQueued {
			r.Targets[i].Status, r.Targets[i].Reason = domain.TargetNotSent, reason
		}
	}
	return nil
}
func (m *memRuns) StartedSince(_ context.Context, since time.Time) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kocs, msgs := m.extraKocs, 0
	for _, r := range m.runs {
		for _, t := range r.Targets {
			if t.StartedAt != nil && !t.StartedAt.Before(since) {
				kocs++
				msgs += t.StepsSent
			}
		}
	}
	return kocs, msgs, nil
}
func (m *memRuns) only() *domain.BulkRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[0]
}

type fixedLimit int

func (l fixedLimit) Get(context.Context) int { return int(l) }

// runLine records Zalo calls and fails the calls listed in failOn (1-based).
type runLine struct {
	PersonalZalo
	connected bool
	calls     []string
	failOn    map[int]error
}

func (l *runLine) IsConnected(context.Context) bool { return l.connected }
func (l *runLine) record(c string) error {
	l.calls = append(l.calls, c)
	return l.failOn[len(l.calls)]
}
func (l *runLine) Send(_ context.Context, _ string, text string) error { return l.record(text) }
func (l *runLine) SendImage(_ context.Context, _ string, img ZaloImage) error {
	return l.record("image:" + img.Filename)
}

type runEnv struct {
	r      *CampaignRunner
	svc    *OutreachService
	line   *runLine
	runs   *memRuns
	sw     *memSwitch
	kocs   map[string]*domain.Koc
	sleeps []time.Duration
	clock  time.Time
	onNap  func(n int) // called at every nap slice with its 1-based number
	naps   int
}

func newRunEnv(limit int) *runEnv {
	e := &runEnv{line: &runLine{connected: true, failOn: map[int]error{}}, runs: &memRuns{}, sw: &memSwitch{on: true}, kocs: map[string]*domain.Koc{},
		clock: time.Date(2026, 11, 16, 10, 0, 0, 0, vnZone)}
	ck, msgs, convs := &claimKocs{marks: map[domain.ID]*time.Time{}}, &fakeOutreachMsgs{}, &seqConvs{}
	e.svc = &OutreachService{kocs: ck, convs: convs, msgs: msgs, zalo: NewZaloService(config.Config{}, e.line, nil), bulk: e.sw,
		clock: func() time.Time { return e.clock }}
	e.r = NewCampaignRunner(e.svc, e.runs, fakeImages{}, fixedLimit(limit), e.sw, func(_ context.Context, id string) (*domain.Koc, error) { return e.kocs[id], nil })
	e.r.sleep = func(_ context.Context, d time.Duration) error {
		e.naps++
		e.sleeps = append(e.sleeps, d)
		if e.onNap != nil {
			e.onNap(e.naps)
		}
		return nil
	}
	return e
}

func (e *runEnv) addKoc(name, phone string) domain.RunTarget {
	k := kocWith(name, phone)
	k.TiktokID = "u" + strconv.Itoa(len(e.kocs))
	e.kocs[k.ID.Hex()] = k
	return domain.RunTarget{KocID: k.ID.Hex()}
}

func twoSteps() []domain.CampaignStep { return []domain.CampaignStep{textStep("a"), textStep("b")} }

func (e *runEnv) start(steps []domain.CampaignStep, targets ...domain.RunTarget) *domain.BulkRun {
	if _, err := e.r.Start(context.Background(), "admin@t.t", "c", steps, targets); err != nil {
		panic(err)
	}
	e.r.Wait()
	return e.runs.only()
}

func statuses(run *domain.BulkRun) []string {
	var out []string
	for _, t := range run.Targets {
		s := t.Status
		if t.Reason != "" {
			s += ":" + t.Reason
		}
		out = append(out, s)
	}
	return out
}

func TestARunSendsToEveryKocThenTurnsTheSwitchOff(t *testing.T) {
	e := newRunEnv(30)
	run := e.start(twoSteps(), e.addKoc("A", "0976451571"), e.addKoc("B", "0976451572"), e.addKoc("C", "0976451573"))
	if run.Status != domain.RunDone || len(e.line.calls) != 6 {
		t.Fatalf("done, 3 KOCs x 2 steps: %s %v", run.Status, e.line.calls)
	}
	if on, _ := e.sw.Enabled(context.Background()); on {
		t.Fatal("the switch is back off when the run ends")
	}
	if e.runs.nextAtSet != 2 {
		t.Fatalf("a wait after the first two KOCs only: %d", e.runs.nextAtSet)
	}
	var total time.Duration
	for _, d := range e.sleeps {
		total += d
	}
	for _, d := range e.sleeps {
		if d > switchPollEvery {
			t.Fatalf("naps are cut into slices of at most %v: %v", switchPollEvery, d)
		}
	}
	if total < 2*kocPauseMin || total > 2*kocPauseMax+4*stepPauseMax+10*time.Second {
		t.Fatalf("two long waits of 3 to 8 minutes and short ones between the steps: %v", total)
	}
}

func TestStartIsRefusedWhileTheSwitchIsOff(t *testing.T) {
	e := newRunEnv(30)
	e.sw.on = false
	if _, err := e.r.Start(context.Background(), "a", "c", twoSteps(), []domain.RunTarget{e.addKoc("A", "0976451571")}); !errors.Is(err, ErrBulkOff) {
		t.Fatalf("the switch is off: %v", err)
	}
}

func TestStartIsRefusedOutsideTheWindowAndForBadInput(t *testing.T) {
	e := newRunEnv(30)
	target := e.addKoc("A", "0976451571")
	e.clock = time.Date(2026, 11, 16, 21, 0, 0, 0, vnZone)
	if _, err := e.r.Start(context.Background(), "a", "c", twoSteps(), []domain.RunTarget{target}); err == nil {
		t.Fatal("21:00 is outside the window")
	}
	e.clock = time.Date(2026, 11, 16, 10, 0, 0, 0, vnZone)
	if _, err := e.r.Start(context.Background(), "a", "c", nil, []domain.RunTarget{target}); err == nil {
		t.Fatal("no steps")
	}
	if _, err := e.r.Start(context.Background(), "a", "c", twoSteps(), nil); err == nil {
		t.Fatal("no KOCs")
	}
	if len(e.line.calls) != 0 {
		t.Fatal("nothing was sent")
	}
}

func TestASecondStartIsRefusedWhileOneRuns(t *testing.T) {
	e := newRunEnv(30)
	target := e.addKoc("A", "0976451571")
	e.runs.runs = append(e.runs.runs, &domain.BulkRun{ID: domain.NewID(), Status: domain.RunRunning})
	if _, err := e.r.Start(context.Background(), "a", "c", twoSteps(), []domain.RunTarget{target}); !errors.Is(err, ErrRunInProgress) {
		t.Fatalf("one at a time: %v", err)
	}
}

func TestTurningTheSwitchOffDuringTheLongPauseStopsTheRun(t *testing.T) {
	e := newRunEnv(30)
	e.onNap = func(int) {
		if len(e.line.calls) == 2 { // the first KOC is done: this nap is the long one
			e.sw.on = false
		}
	}
	run := e.start(twoSteps(), e.addKoc("A", "0976451571"), e.addKoc("B", "0976451572"), e.addKoc("C", "0976451573"))
	got := statuses(run)
	if run.Status != domain.RunStopped || run.StopReason != "switch_off" || len(e.line.calls) != 2 || got[0] != "sent" || got[1] != "not_sent:switch_off" || got[2] != "not_sent:switch_off" {
		t.Fatalf("stopped at once, the rest not sent: %s %v calls=%d", run.Status, got, len(e.line.calls))
	}
}

func TestTurningTheSwitchOffBetweenTheStepsLeavesAPartialKoc(t *testing.T) {
	e := newRunEnv(30)
	e.onNap = func(int) {
		if len(e.line.calls) == 1 {
			e.sw.on = false
		}
	}
	run := e.start(twoSteps(), e.addKoc("A", "0976451571"), e.addKoc("B", "0976451572"))
	got := statuses(run)
	if got[0] != "partial:stopped" || run.Targets[0].StepsSent != 1 || got[1] != "not_sent:switch_off" || run.StopReason != "switch_off" {
		t.Fatalf("%v %+v", got, run.Targets[0])
	}
}

func TestTwoZaloFaultsInARowStopTheRun(t *testing.T) {
	e := newRunEnv(30)
	e.line.failOn = map[int]error{1: errors.New("no"), 2: errors.New("no")}
	run := e.start(twoSteps(), e.addKoc("A", "0976451571"), e.addKoc("B", "0976451572"), e.addKoc("C", "0976451573"))
	got := statuses(run)
	if run.StopReason != "zalo_errors" || got[0] != "failed:no" || got[1] != "failed:no" || got[2] != "not_sent:zalo_errors" || len(e.line.calls) != 2 {
		t.Fatalf("%s %v calls=%d", run.StopReason, got, len(e.line.calls))
	}
}

func TestAFaultThenASuccessDoesNotStop(t *testing.T) {
	e := newRunEnv(30)
	e.line.failOn = map[int]error{1: errors.New("no")}
	run := e.start(twoSteps(), e.addKoc("A", "0976451571"), e.addKoc("B", "0976451572"), e.addKoc("C", "0976451573"))
	got := statuses(run)
	if run.Status != domain.RunDone || got[0] != "failed:no" || got[1] != "sent" || got[2] != "sent" {
		t.Fatalf("%s %v", run.Status, got)
	}
}

func TestTheDailyKocLimitStopsTheRun(t *testing.T) {
	e := newRunEnv(10)
	e.runs.extraKocs = 9
	run := e.start(twoSteps(), e.addKoc("A", "0976451571"), e.addKoc("B", "0976451572"), e.addKoc("C", "0976451573"))
	got := statuses(run)
	if run.StopReason != "daily_koc_limit_reached" || got[0] != "sent" || got[1] != "not_sent:daily_koc_limit_reached" || len(e.line.calls) != 2 {
		t.Fatalf("one more KOC fits: %s %v", run.StopReason, got)
	}
}

func TestTheRunStopsWhenTheWindowCloses(t *testing.T) {
	e := newRunEnv(30)
	e.onNap = func(int) {
		if len(e.line.calls) == 2 {
			e.clock = time.Date(2026, 11, 16, 20, 31, 0, 0, vnZone)
		}
	}
	run := e.start(twoSteps(), e.addKoc("A", "0976451571"), e.addKoc("B", "0976451572"))
	if run.StopReason != "outside_0830_2030_window" || len(e.line.calls) != 2 {
		t.Fatalf("%s calls=%d", run.StopReason, len(e.line.calls))
	}
}

func TestTheDailyMessageCapStopsTheRun(t *testing.T) {
	e := newRunEnv(50)
	e.runs.runs = append(e.runs.runs, &domain.BulkRun{ID: domain.NewID(), Status: domain.RunDone, Targets: []domain.RunTarget{{Status: domain.TargetSent, StepsSent: MaxCampaignMessagesPerDay - 1, StartedAt: ptrTime(e.clock.Add(-time.Hour))}}})
	r2 := &domain.BulkRun{CampaignName: "c", Steps: twoSteps(), Targets: []domain.RunTarget{e.addKoc("A", "0976451571")}, By: "a"}
	e.runs.runs = append(e.runs.runs, r2)
	r2.ID, r2.Status = domain.NewID(), domain.RunRunning
	r2.Targets[0].Status = domain.TargetQueued
	e.r.spawn(r2)
	e.r.Wait()
	if r2.StopReason != "daily_message_cap_reached" || len(e.line.calls) != 0 {
		t.Fatalf("599 + 2 steps is over 600: %q calls=%d", r2.StopReason, len(e.line.calls))
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestResumeMarksAStepInFlightUnknownAndGoesOn(t *testing.T) {
	e := newRunEnv(30)
	a, b := e.addKoc("A", "0976451571"), e.addKoc("B", "0976451572")
	started := e.clock
	a.Status, a.StepsSent, a.StartedAt = domain.TargetSending, 1, &started
	b.Status = domain.TargetQueued
	run := &domain.BulkRun{ID: domain.NewID(), Status: domain.RunRunning, Steps: twoSteps(), Targets: []domain.RunTarget{a, b}}
	e.runs.runs = append(e.runs.runs, run)
	e.r.Resume(context.Background())
	e.r.Wait()
	got := statuses(run)
	if got[0] != "unknown:restart_during_step" || got[1] != "sent" || len(e.line.calls) != 2 || run.Status != domain.RunDone {
		t.Fatalf("A is never sent to again, B goes on: %v calls=%v", got, e.line.calls)
	}
}

func TestResumeWithTheSwitchOffEndsTheRun(t *testing.T) {
	e := newRunEnv(30)
	e.sw.on = false
	run := &domain.BulkRun{ID: domain.NewID(), Status: domain.RunRunning, Steps: twoSteps(), Targets: []domain.RunTarget{{KocID: "x", Status: domain.TargetQueued}}}
	e.runs.runs = append(e.runs.runs, run)
	e.r.Resume(context.Background())
	e.r.Wait()
	if run.Status != domain.RunStopped || run.Targets[0].Status != domain.TargetNotSent || len(e.line.calls) != 0 {
		t.Fatalf("%s %v", run.Status, statuses(run))
	}
}

func TestASkippedKocDoesNotCostALongPause(t *testing.T) {
	e := newRunEnv(30)
	run := e.start(twoSteps(), e.addKoc("A", ""), e.addKoc("B", "0976451572"), e.addKoc("C", "0976451573"))
	got := statuses(run)
	if got[0] != "skipped:no_phone" || got[1] != "sent" || got[2] != "sent" || e.runs.nextAtSet != 1 {
		t.Fatalf("a long wait only after B: %v waits=%d", got, e.runs.nextAtSet)
	}
}

func TestAnUnknownKocIdIsReportedNotFound(t *testing.T) {
	e := newRunEnv(30)
	run := e.start(twoSteps(), domain.RunTarget{KocID: "ffffffffffffffffffffffff"}, e.addKoc("B", "0976451572"))
	got := statuses(run)
	if got[0] != "skipped:not_found" || got[1] != "sent" {
		t.Fatalf("%v", got)
	}
}

func TestEstimateMatchesTheSpecExample(t *testing.T) {
	e := newRunEnv(30)
	if d := e.r.Estimate(30, 9); d < 3*time.Hour || d > 3*time.Hour+30*time.Minute {
		t.Fatalf("30 KOCs x 9 steps takes about 3.5 hours: %v", d)
	}
}

func TestTheEndOfARunIsTriedAgainWhenTheStoreFails(t *testing.T) {
	e := newRunEnv(30)
	e.runs.finishFails = 2
	run := e.start(twoSteps(), e.addKoc("A", "0976451571"))
	if run.Status != domain.RunDone {
		t.Fatalf("a failed Finish must be retried or the run stays running for ever and blocks every later run: %s", run.Status)
	}
}

func TestAFaultThenASkippedKocThenAFaultIsNotTwoInARow(t *testing.T) {
	e := newRunEnv(30)
	e.line.failOn = map[int]error{1: errors.New("no"), 2: errors.New("no")}
	run := e.start(twoSteps(), e.addKoc("A", "0976451571"), e.addKoc("B", ""), e.addKoc("C", "0976451573"))
	got := statuses(run)
	if run.StopReason == "zalo_errors" || got[1] != "skipped:no_phone" || got[2] != "failed:no" {
		t.Fatalf("the skipped KOC in between breaks the row: %s %v", run.StopReason, got)
	}
}

func TestTheCallerOfStartGetsACopyNotTheRunTheGoroutineChanges(t *testing.T) {
	e := newRunEnv(30)
	run, err := e.r.Start(context.Background(), "a", "c", twoSteps(), []domain.RunTarget{e.addKoc("A", "0976451571")})
	if err != nil {
		t.Fatal(err)
	}
	e.r.Wait()
	if run.Targets[0].Status != domain.TargetQueued {
		t.Fatalf("what Start returns is a snapshot taken before the goroutine ran, got %s", run.Targets[0].Status)
	}
}
