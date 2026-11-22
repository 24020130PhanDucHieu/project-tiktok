package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// accountDir is the stored Zalo accounts, as ZaloService sees them.
type accountDir struct {
	def  domain.ID
	rows map[domain.ID]*domain.ZaloAccount
}

func newAccountDir(labels ...string) (*accountDir, []domain.ID) {
	d := &accountDir{rows: map[domain.ID]*domain.ZaloAccount{}}
	var ids []domain.ID
	for i, l := range labels {
		a := &domain.ZaloAccount{ID: domain.NewID(), Label: l, IsDefault: i == 0}
		d.rows[a.ID] = a
		ids = append(ids, a.ID)
		if i == 0 {
			d.def = a.ID
		}
	}
	return d, ids
}

func (d *accountDir) Default(context.Context) (*domain.ZaloAccount, error) {
	return d.rows[d.def], nil
}
func (d *accountDir) Get(_ context.Context, id domain.ID) (*domain.ZaloAccount, error) {
	if a := d.rows[id]; a != nil {
		return a, nil
	}
	return nil, domain.NotFound("Không tìm thấy tài khoản Zalo.")
}

// accountRun is a run environment with two Zalo accounts, A (the default) and B.
func accountRun(limit int) (*runEnv, domain.ID, domain.ID) {
	e := newRunEnv(limit)
	dir, ids := newAccountDir("A", "B")
	e.svc.zalo.SetAccounts(dir)
	return e, ids[0], ids[1]
}

func (e *runEnv) startAs(account string, steps []domain.CampaignStep, targets ...domain.RunTarget) (*domain.BulkRun, error) {
	e.sw.on = true // a run turns the switch off whenever it ends: each start needs it on again
	run, err := e.r.Start(context.Background(), "admin@t.t", "c", steps, targets, RunOptions{ZaloAccountID: account})
	e.r.Wait()
	if err != nil {
		return nil, err
	}
	return e.runs.byID(run.ID), nil
}

func (m *memRuns) byID(id domain.ID) *domain.BulkRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneRun(m.find(id))
}

func TestARunSendsThroughTheAccountItWasStartedWith(t *testing.T) {
	e, _, b := accountRun(30)
	run, err := e.startAs(b.Hex(), twoSteps(), e.addKoc("K1", "0976451571"), e.addKoc("K2", "0976451572"))
	if err != nil {
		t.Fatal(err)
	}
	if run.ZaloAccountID != b.Hex() {
		t.Fatalf("the run is filed under %q, want %s", run.ZaloAccountID, b.Hex())
	}
	if len(e.line.accts) != 4 {
		t.Fatalf("%d sends, want 4", len(e.line.accts))
	}
	for _, a := range e.line.accts {
		if a != b.Hex() {
			t.Fatalf("a message went out through %q, want account B %s: %v", a, b.Hex(), e.line.accts)
		}
	}
}

func TestARunWithoutAnAccountUsesTheDefaultAccount(t *testing.T) {
	e, a, _ := accountRun(30)
	run, err := e.startAs("", twoSteps(), e.addKoc("K1", "0976451571"))
	if err != nil {
		t.Fatal(err)
	}
	if run.ZaloAccountID != a.Hex() {
		t.Fatalf("the run is filed under %q, want the default account %s", run.ZaloAccountID, a.Hex())
	}
	for _, got := range e.line.accts {
		if got != a.Hex() {
			t.Fatalf("sent through %q, want the default account", got)
		}
	}
}

func TestARunForAnAccountThatDoesNotExistIsRefused(t *testing.T) {
	e, _, _ := accountRun(30)
	for _, bad := range []string{domain.NewID().Hex(), "not-an-id"} {
		_, err := e.startAs(bad, twoSteps(), e.addKoc("K1", "0976451571"))
		var fault *domain.Fault
		if !errors.As(err, &fault) || fault.Kind != domain.FaultInvalid {
			t.Fatalf("account %q: err = %v, want an invalid-request fault", bad, err)
		}
	}
	if len(e.runs.runs) != 0 || len(e.line.calls) != 0 {
		t.Fatalf("a refused run stored %d runs and sent %v", len(e.runs.runs), e.line.calls)
	}
}

func TestWithoutStoredAccountsARunStaysTheOldSingleAccount(t *testing.T) {
	e := newRunEnv(30) // no account directory
	run, err := e.startAs("", twoSteps(), e.addKoc("K1", "0976451571"))
	if err != nil {
		t.Fatal(err)
	}
	if run.ZaloAccountID != "" || run.Status != domain.RunDone {
		t.Fatalf("account %q status %s", run.ZaloAccountID, run.Status)
	}
}

func TestEachAccountHasItsOwnDailyLimit(t *testing.T) {
	e, a, b := accountRun(1)                    // one KOC a day per account
	e.runs.extraBy = map[string]int{a.Hex(): 1} // account A already started its one KOC today

	runA, err := e.startAs(a.Hex(), twoSteps(), e.addKoc("K1", "0976451571"))
	if err != nil {
		t.Fatal(err)
	}
	if runA.Status != domain.RunStopped || runA.StopReason != "daily_koc_limit_reached" {
		t.Fatalf("account A is at its limit: %s %s", runA.Status, runA.StopReason)
	}
	if len(e.line.calls) != 0 {
		t.Fatalf("account A sent %v past its limit", e.line.calls)
	}

	runB, err := e.startAs(b.Hex(), twoSteps(), e.addKoc("K2", "0976451572"))
	if err != nil {
		t.Fatal(err)
	}
	if runB.Status != domain.RunDone || len(e.line.calls) != 2 {
		t.Fatalf("account B has its own count and is not held back by A: %s %v", runB.Status, e.line.calls)
	}
}

func TestTheMessageCapIsCountedPerAccount(t *testing.T) {
	e, a, b := accountRun(1000) // a KOC limit high enough that the message cap is what stops account A
	// account A used up the day's messages; B has sent none
	started := e.clock
	for i := 0; i < MaxCampaignMessagesPerDay/2; i++ {
		e.runs.runs = append(e.runs.runs, &domain.BulkRun{ID: domain.NewID(), Status: domain.RunDone, ZaloAccountID: a.Hex(),
			Targets: []domain.RunTarget{{KocID: "x", Status: domain.TargetSent, StepsSent: 2, StartedAt: &started}}})
	}
	runA, err := e.startAs(a.Hex(), twoSteps(), e.addKoc("K1", "0976451571"))
	if err != nil {
		t.Fatal(err)
	}
	if runA.StopReason != "daily_message_cap_reached" {
		t.Fatalf("account A: %s %s", runA.Status, runA.StopReason)
	}
	runB, err := e.startAs(b.Hex(), twoSteps(), e.addKoc("K2", "0976451572"))
	if err != nil {
		t.Fatal(err)
	}
	if runB.Status != domain.RunDone {
		t.Fatalf("account B must not share A's cap: %s %s", runB.Status, runB.StopReason)
	}
}

func TestARunWaitsForItsOwnAccountsLineNotAnotherOnes(t *testing.T) {
	e, _, b := accountRun(30)
	e.line.down = map[string]bool{b.Hex(): true} // B is logged out, A is fine
	run, err := e.startAs(b.Hex(), twoSteps(), e.addKoc("K1", "0976451571"))
	if err != nil {
		t.Fatal(err)
	}
	if run.StopReason != "zalo_not_ready" || len(e.line.calls) != 0 {
		t.Fatalf("a run for a logged-out account must stop, not send through another: %s %v", run.StopReason, e.line.calls)
	}
}

// --- a KOC who already talks through another account

// boundTo makes every KOC's thread belong to a Zalo account ("" = it belongs to none yet).
type boundTo struct {
	seqConvs
	account string
}

func (c *boundTo) UpsertKocMessage(context.Context, domain.KocContact, time.Time) (*domain.Conversation, error) {
	return &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelZalo, ExternalContactID: "42", ZaloAccountID: c.account}, nil
}

func TestAKocWhoTalksThroughAnotherAccountIsSkippedNotSentFromTheWrongNumber(t *testing.T) {
	e, a, b := accountRun(30)
	e.svc.convs = &boundTo{account: a.Hex()} // this KOC's thread belongs to account A
	run, err := e.startAs(b.Hex(), twoSteps(), e.addKoc("K1", "0976451571"))
	if err != nil {
		t.Fatal(err)
	}
	if got := statuses(run); len(got) != 1 || got[0] != "skipped:on_other_account" {
		t.Fatalf("statuses = %v, want skipped:on_other_account", got)
	}
	if len(e.line.calls) != 0 {
		t.Fatalf("a KOC of account A was messaged from account B: %v", e.line.calls)
	}
	ck := e.svc.kocs.(*claimKocs)
	if ck.claims != 1 || ck.releases != 1 {
		t.Fatalf("claims %d releases %d: the claim must go back when nothing was sent", ck.claims, ck.releases)
	}
}

func TestAKocOnTheSameAccountIsSent(t *testing.T) {
	e, _, b := accountRun(30)
	e.svc.convs = &boundTo{account: b.Hex()}
	run, err := e.startAs(b.Hex(), twoSteps(), e.addKoc("K1", "0976451571"))
	if err != nil {
		t.Fatal(err)
	}
	if got := statuses(run); len(got) != 1 || got[0] != "sent" {
		t.Fatalf("statuses = %v", got)
	}
}

func TestAThreadWithNoAccountYetIsAnsweredThroughTheRunsAccount(t *testing.T) {
	e, _, b := accountRun(30)
	e.svc.convs = &boundTo{account: ""}
	run, err := e.startAs(b.Hex(), twoSteps(), e.addKoc("K1", "0976451571"))
	if err != nil {
		t.Fatal(err)
	}
	if got := statuses(run); len(got) != 1 || got[0] != "sent" {
		t.Fatalf("statuses = %v", got)
	}
	for _, got := range e.line.accts {
		if got != b.Hex() {
			t.Fatalf("a thread with no account of its own went out through %q, want the run's account B", got)
		}
	}
}
