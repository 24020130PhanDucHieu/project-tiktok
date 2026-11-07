package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// fakeSweepKocs feeds the sweep two groups of KOCs and records which ones were saved.
type fakeSweepKocs struct {
	OutreachKocs
	chase, notFit []*domain.Koc
	saved         []string
}

func (f *fakeSweepKocs) FindDormantDue(context.Context, time.Time) ([]*domain.Koc, error) {
	return nil, nil
}
func (f *fakeSweepKocs) FindToChase(context.Context) ([]*domain.Koc, error)    { return f.chase, nil }
func (f *fakeSweepKocs) FindInShipping(context.Context) ([]*domain.Koc, error) { return nil, nil }
func (f *fakeSweepKocs) FindDelivered(context.Context) ([]*domain.Koc, error)  { return nil, nil }
func (f *fakeSweepKocs) FindNotFitAwaitingReturn(context.Context) ([]*domain.Koc, error) {
	return f.notFit, nil
}
func (f *fakeSweepKocs) Save(_ context.Context, k *domain.Koc) error {
	f.saved = append(f.saved, k.TiktokID)
	return nil
}

// flakyThreads fails for some KOCs, as a database would for a single bad record or a dropped connection.
type flakyThreads struct {
	OutreachConvs
	bad map[string]bool
}

func (f flakyThreads) UpsertKocMessage(_ context.Context, c domain.KocContact, _ time.Time) (*domain.Conversation, error) {
	if f.bad[c.Handle] {
		return nil, errors.New("connection reset by peer")
	}
	return &domain.Conversation{ID: domain.NewID()}, nil
}

func sweepKoc(id string) *domain.Koc {
	k := domain.NewKoc()
	k.TiktokID, k.DisplayName, k.Stage, k.Status = id, id, domain.StageContacted, domain.KocActive
	return k
}

// One KOC that fails used to stop the whole sweep, so every KOC after it was starved on each run and the same one
// failed again next time.
func TestOneFailingKocDoesNotStopTheSweep(t *testing.T) {
	kocs := &fakeSweepKocs{
		chase:  []*domain.Koc{sweepKoc("bad"), sweepKoc("good")},
		notFit: []*domain.Koc{sweepKoc("bad2"), sweepKoc("good2")},
	}
	svc := &OutreachService{kocs: kocs, convs: flakyThreads{bad: map[string]bool{"@bad": true, "@bad2": true}}, msgs: &fakeOutreachMsgs{}}

	sum, err := svc.RunStage2Sweep(context.Background(), true)
	if err != nil {
		t.Fatalf("a KOC that fails is reported, it does not fail the sweep: %v", err)
	}
	if sum.Failures != 2 || !strings.Contains(sum.FirstError, "bad") {
		t.Fatalf("both failures must be counted and the first one named: %+v", sum)
	}
	if sum.EmailReminded != 1 || sum.ReturnReminded != 1 {
		t.Fatalf("the KOCs after a failing one must still be served: %+v", sum)
	}
	got := strings.Join(kocs.saved, ",")
	if !strings.Contains(got, "good") || !strings.Contains(got, "good2") || strings.Contains(got, "bad") {
		t.Fatalf("only the KOCs that were served may be saved, saved: %s", got)
	}
}

func TestASweepWithoutFailuresReportsNone(t *testing.T) {
	kocs := &fakeSweepKocs{chase: []*domain.Koc{sweepKoc("good")}}
	svc := &OutreachService{kocs: kocs, convs: flakyThreads{}, msgs: &fakeOutreachMsgs{}}

	sum, err := svc.RunStage2Sweep(context.Background(), true)
	if err != nil || sum.Failures != 0 || sum.FirstError != "" || sum.EmailReminded != 1 {
		t.Fatalf("%+v %v", sum, err)
	}
}
