package repository_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

func newRun(by string, n int) *domain.BulkRun {
	run := &domain.BulkRun{CampaignName: "c", By: by, Steps: []domain.CampaignStep{{Kind: domain.StepText, Text: "a"}}}
	for i := 0; i < n; i++ {
		run.Targets = append(run.Targets, domain.RunTarget{KocID: "k" + strconv.Itoa(i), Status: domain.TargetQueued})
	}
	return run
}

func TestOnlyOneRunRunsAtATime(t *testing.T) {
	repo := repository.NewBulkRunRepository(testDB(t))
	ctx := context.Background()
	first := newRun("a", 2)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newRun("b", 1)); !isConflict(err) {
		t.Fatalf("a second running run is a conflict: %v", err)
	}
	if run, err := repo.Running(ctx); err != nil || run == nil || run.ID != first.ID {
		t.Fatalf("the running one is found: %v %v", run, err)
	}
	if err := repo.Finish(ctx, first.ID, domain.RunDone, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if run, _ := repo.Running(ctx); run != nil {
		t.Fatal("nothing runs after Finish")
	}
	if err := repo.Create(ctx, newRun("b", 1)); err != nil {
		t.Fatalf("a new run may start once the other ended: %v", err)
	}
}

func TestFinishTurnsTheQueuedIntoNotSentWithTheReason(t *testing.T) {
	repo := repository.NewBulkRunRepository(testDB(t))
	ctx := context.Background()
	run := newRun("a", 3)
	_ = repo.Create(ctx, run)
	now := time.Now()
	if err := repo.SetTarget(ctx, run.ID, 0, domain.RunTarget{KocID: "k0", Status: domain.TargetSent, StepsSent: 3, StartedAt: &now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetNextAt(ctx, run.ID, &now); err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(ctx, run.ID, domain.RunStopped, "switch_off", now); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Recent(ctx, 5)
	if len(got) != 1 || got[0].Status != domain.RunStopped || got[0].StopReason != "switch_off" || got[0].NextAt != nil || got[0].FinishedAt == nil {
		t.Fatalf("run: %+v", got)
	}
	ts := got[0].Targets
	if ts[0].Status != domain.TargetSent || ts[1].Status != domain.TargetNotSent || ts[1].Reason != "switch_off" || ts[2].Status != domain.TargetNotSent {
		t.Fatalf("only the queued ones change: %+v", ts)
	}
}

func TestStartedSinceCountsKocsAndMessages(t *testing.T) {
	repo := repository.NewBulkRunRepository(testDB(t))
	ctx := context.Background()
	run := newRun("a", 3)
	_ = repo.Create(ctx, run)
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	_ = repo.SetTarget(ctx, run.ID, 0, domain.RunTarget{KocID: "k0", Status: domain.TargetSent, StepsSent: 9, StartedAt: &now})
	_ = repo.SetTarget(ctx, run.ID, 1, domain.RunTarget{KocID: "k1", Status: domain.TargetUnknown, StepsSent: 2, StartedAt: &now})
	_ = repo.SetTarget(ctx, run.ID, 2, domain.RunTarget{KocID: "k2", Status: domain.TargetSent, StepsSent: 9, StartedAt: &old})
	kocs, msgs, err := repo.StartedSince(ctx, now.Add(-time.Hour))
	if err != nil || kocs != 2 || msgs != 11 {
		t.Fatalf("2 KOCs and 11 messages today: %d %d %v", kocs, msgs, err)
	}
}

func TestTheDailyLimitDefaultsTo30AndOnlyOffersFourValues(t *testing.T) {
	repo := repository.NewBulkLimitRepository(testDB(t))
	ctx := context.Background()
	if repo.Get(ctx) != 30 {
		t.Fatal("30 by default")
	}
	if err := repo.Set(ctx, 25); err == nil {
		t.Fatal("25 is not offered")
	}
	if err := repo.Set(ctx, 50); err != nil || repo.Get(ctx) != 50 {
		t.Fatal("50 is stored")
	}
}
