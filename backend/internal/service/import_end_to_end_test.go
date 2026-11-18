package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// flakyVerifier is TikTok Shop with a bad moment: for a username it first answers the listed errors, then the truth.
type flakyVerifier struct {
	known  map[string]*TargetCreator
	before map[string][]error
	tried  map[string]int
}

func (v *flakyVerifier) ResolveCreator(_ context.Context, username string) (*TargetCreator, error) {
	if v.tried == nil {
		v.tried = map[string]int{}
	}
	n := v.tried[username]
	v.tried[username]++
	if errs := v.before[username]; n < len(errs) {
		return nil, errs[n]
	}
	if c, ok := v.known[username]; ok {
		return c, nil
	}
	return nil, ErrCreatorNotFound
}

// A whole import from the first row to the last, with the real row logic (ImportOne), the real worker and the real
// translation of what TikTok says into what a row ends as: nothing in the chain is faked but the stores and the clock.
func TestAWholeImportEndsEveryRowTheRightWay(t *testing.T) {
	dir := &bulkDirectory{existing: map[string]*domain.Koc{
		"dup":    {TiktokID: "dup", Status: domain.KocActive},
		"collab": {TiktokID: "collab", DaHopTac: true, Status: domain.KocActive},
	}}
	th := &bulkThreads{}
	creator := func(u string) *TargetCreator { return &TargetCreator{Username: u, Nickname: "Tên " + u} }
	v := &flakyVerifier{
		known: map[string]*TargetCreator{"ok1": creator("ok1"), "slow": creator("slow"), "blip": creator("blip"), "ok2": creator("ok2")},
		before: map[string][]error{
			"slow": {&ErrRateLimited{RetryAfter: 40 * time.Second}}, // TikTok limits once
			"blip": {errors.New("context deadline exceeded")},       // TikTok does not answer once
		},
	}
	svc := newBulkService(dir, th, nil).WithCreatorVerifier(v)

	store := &memImports{}
	runner := NewImportRunner(svc, store)
	var slept time.Duration
	runner.sleep = func(_ context.Context, d time.Duration) error { slept += d; return nil }

	ids := []string{"ok1", "dup", "collab", "ghost", "slow", "blip", "ok2"}
	rows := make([]domain.ImportRow, len(ids))
	for i, id := range ids {
		rows[i] = domain.ImportRow{Line: i + 2, TiktokID: id}
	}
	if _, err := runner.Start(context.Background(), "a@x.vn", "TikToday", rows); err != nil {
		t.Fatal(err)
	}
	runner.Wait()

	job := store.only()
	want := []string{domain.RowAdded, domain.RowDuplicate, domain.RowCollaborated, domain.RowNotListed, domain.RowAdded, domain.RowAdded, domain.RowAdded}
	if job.Status != domain.ImportDone || !sameStrings(rowStatuses(job), want) {
		t.Fatalf("%s %v (reason %q)", job.Status, rowStatuses(job), job.StopReason)
	}
	if len(dir.created) != 4 || th.ensured != 4 {
		t.Fatalf("four KOCs saved, each with its thread: %d saved, %d threads", len(dir.created), th.ensured)
	}
	if slept != ImportRateWait+ImportRetryWait || len(store.kinds) != 2 || store.kinds[0] != "rate_limited" || store.kinds[1] != "unavailable" {
		t.Fatalf("one long wait for the rate limit and one short for the bad answer: %v %v", slept, store.kinds)
	}
	if v.tried["slow"] != 2 || v.tried["blip"] != 2 || v.tried["ok1"] != 1 || v.tried["dup"] != 0 || v.tried["collab"] != 0 {
		t.Fatalf("a known row is never asked about, a waited-for row is asked twice: %v", v.tried)
	}
	if job.Rows[3].Reason == "" || job.Rows[1].Reason != "Trùng ID kênh" {
		t.Fatalf("reasons are kept: %+v", job.Rows)
	}
}

func TestAWholeImportStopsAtOnceWhenTheShopIsNotConnected(t *testing.T) {
	v := &flakyVerifier{before: map[string][]error{"r1": {ErrTTShopNotConnected}}}
	svc := newBulkService(&bulkDirectory{}, &bulkThreads{}, nil).WithCreatorVerifier(v)
	store := &memImports{}
	runner := NewImportRunner(svc, store)
	runner.sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := runner.Start(context.Background(), "a", "TikToday", []domain.ImportRow{{Line: 2, TiktokID: "r1"}, {Line: 3, TiktokID: "r2"}}); err != nil {
		t.Fatal(err)
	}
	runner.Wait()
	job := store.only()
	if job.Status != domain.ImportStopped || job.StopReason != "tiktok_not_connected" || !sameStrings(rowStatuses(job), []string{"queued", "queued"}) || v.tried["r2"] != 0 {
		t.Fatalf("%s %s %v", job.Status, job.StopReason, rowStatuses(job))
	}
}

// failingThreads is an inbox whose threads cannot be made.
type failingThreads struct{ bulkThreads }

func (failingThreads) EnsureKocContact(context.Context, domain.KocContact) (*domain.Conversation, error) {
	return nil, errors.New("db down")
}

func TestAKocThatWasSavedIsAddedEvenWhenItsInboxThreadCannotBeMade(t *testing.T) {
	dir := &bulkDirectory{}
	v := &flakyVerifier{known: map[string]*TargetCreator{"abc": {Username: "abc", Nickname: "Abc"}}}
	svc := (&KocService{kocs: dir, convs: failingThreads{}}).WithCreatorVerifier(v)
	store := &memImports{}
	runner := NewImportRunner(svc, store)
	runner.sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := runner.Start(context.Background(), "a", "TikToday", []domain.ImportRow{{Line: 2, TiktokID: "abc"}}); err != nil {
		t.Fatal(err)
	}
	runner.Wait()
	job := store.only()
	if len(dir.created) != 1 || job.Rows[0].Status != domain.RowAdded || job.Rows[0].Reason != "" {
		t.Fatalf("the KOC is in the directory, so the row says so: saved=%d %v %q", len(dir.created), rowStatuses(job), job.Rows[0].Reason)
	}
	if job.Status != domain.ImportDone {
		t.Fatalf("one missing thread does not stop the job: %s %s", job.Status, job.StopReason)
	}
}
