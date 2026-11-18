package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

func newImportJob(rows ...string) *domain.ImportJob {
	job := &domain.ImportJob{Source: "TikToday", By: "a@x.vn"}
	for i, id := range rows {
		job.Rows = append(job.Rows, domain.ImportRow{Line: i + 2, TiktokID: id, Status: domain.RowQueued})
	}
	return job
}

func TestOnlyOneImportRunsAtATime(t *testing.T) {
	repo := repository.NewImportJobRepository(testDB(t))
	ctx := context.Background()
	first := newImportJob("a", "b")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newImportJob("c")); !isConflict(err) {
		t.Fatalf("a second running job is a conflict: %v", err)
	}
	if job, err := repo.Running(ctx); err != nil || job == nil || job.ID != first.ID {
		t.Fatalf("the running job is found: %v %v", job, err)
	}
	if err := repo.Finish(ctx, first.ID, domain.ImportDone, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if job, _ := repo.Running(ctx); job != nil {
		t.Fatal("nothing runs after Finish")
	}
	if err := repo.Create(ctx, newImportJob("c")); err != nil {
		t.Fatalf("a new job may start once the other ended: %v", err)
	}
}

func TestARowAnswerIsKeptAndTheOthersAreUntouched(t *testing.T) {
	repo := repository.NewImportJobRepository(testDB(t))
	ctx := context.Background()
	job := newImportJob("a", "b", "c")
	_ = repo.Create(ctx, job)
	if err := repo.SetRow(ctx, job.ID, 1, domain.RowNotListed, "Không có trong TikTok"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetRow(ctx, job.ID, 0, domain.RowAdded, ""); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Find(ctx, job.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	r := got.Rows
	if r[0].Status != domain.RowAdded || r[0].Reason != "" || r[1].Status != domain.RowNotListed || r[1].Reason != "Không có trong TikTok" || r[2].Status != domain.RowQueued {
		t.Fatalf("rows: %+v", r)
	}
	if r[1].TiktokID != "b" || r[1].Line != 3 {
		t.Fatalf("what the sheet said is kept: %+v", r[1])
	}
}

func TestProgressWaitAndStopRequestRoundTrip(t *testing.T) {
	repo := repository.NewImportJobRepository(testDB(t))
	ctx := context.Background()
	job := newImportJob("a")
	_ = repo.Create(ctx, job)
	until := time.Now().Add(time.Minute).Truncate(time.Millisecond)
	if err := repo.SetProgress(ctx, job.ID, "a", &until, 2, "unavailable"); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Find(ctx, job.ID.Hex())
	if got.Current != "a" || got.WaitUntil == nil || got.WaitAttempt != 2 || got.WaitKind != "unavailable" {
		t.Fatalf("while waiting: %+v", got)
	}
	if err := repo.SetProgress(ctx, job.ID, "", nil, 0, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Find(ctx, job.ID.Hex())
	if got.Current != "" || got.WaitUntil != nil || got.WaitKind != "" {
		t.Fatalf("cleared: %+v", got)
	}
	if on, _ := repo.StopRequested(ctx, job.ID); on {
		t.Fatal("not asked to stop yet")
	}
	if found, err := repo.RequestStop(ctx, job.ID.Hex()); !found || err != nil {
		t.Fatalf("the running job takes the stop request: %v %v", found, err)
	}
	if on, _ := repo.StopRequested(ctx, job.ID); !on {
		t.Fatal("the stop request is kept")
	}
	_ = repo.Finish(ctx, job.ID, domain.ImportStopped, "stopped_by_user", time.Now())
	if found, _ := repo.RequestStop(ctx, job.ID.Hex()); found {
		t.Fatal("a job that ended cannot be asked to stop")
	}
}

func TestFinishKeepsTheQueuedRowsQueued(t *testing.T) {
	repo := repository.NewImportJobRepository(testDB(t))
	ctx := context.Background()
	job := newImportJob("a", "b")
	_ = repo.Create(ctx, job)
	_ = repo.SetRow(ctx, job.ID, 0, domain.RowAdded, "")
	_ = repo.Finish(ctx, job.ID, domain.ImportStopped, "rate_limited", time.Now())
	got, _ := repo.Find(ctx, job.ID.Hex())
	if got.Status != domain.ImportStopped || got.StopReason != "rate_limited" || got.FinishedAt == nil || got.Rows[1].Status != domain.RowQueued {
		t.Fatalf("%+v", got)
	}
}

func TestLatestIsTheNewestJobWhateverItsStatus(t *testing.T) {
	repo := repository.NewImportJobRepository(testDB(t))
	ctx := context.Background()
	if job, err := repo.Latest(ctx); err != nil || job != nil {
		t.Fatalf("nothing yet: %v %v", job, err)
	}
	first := newImportJob("a")
	_ = repo.Create(ctx, first)
	_ = repo.Finish(ctx, first.ID, domain.ImportDone, "", time.Now())
	time.Sleep(5 * time.Millisecond)
	second := newImportJob("b")
	_ = repo.Create(ctx, second)
	_ = repo.Finish(ctx, second.ID, domain.ImportStopped, "rate_limited", time.Now())
	got, err := repo.Latest(ctx)
	if err != nil || got == nil || got.ID != second.ID || got.Status != domain.ImportStopped {
		t.Fatalf("the newest job, ended or not: %+v %v", got, err)
	}
}
