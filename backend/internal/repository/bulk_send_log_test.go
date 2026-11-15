package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

func TestBulkSendLogReturnsTheNewestRowsFirst(t *testing.T) {
	db := testDB(t)
	repo := repository.NewBulkSendLogRepository(db)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	rows := []domain.BulkSendLog{
		{RunID: "r1", KocID: "a", KocName: "An", Status: "sent", By: "x@y.vn", At: now.Add(-2 * time.Hour)},
		{RunID: "r2", KocID: "b", KocName: "Bình", Status: "skipped", Reason: "no_phone", By: "x@y.vn", At: now},
	}
	if err := repo.Record(ctx, rows); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Recent(ctx, 10)
	if err != nil || len(got) != 2 || got[0].RunID != "r2" || got[1].RunID != "r1" || got[0].Reason != "no_phone" {
		t.Fatalf("newest first, fields kept: %+v %v", got, err)
	}
	if empty, _ := repository.NewBulkSendLogRepository(testDB(t)).Recent(ctx, 10); empty == nil {
		t.Fatal("an empty log is an empty list, not null")
	}
}

func TestTheBulkSwitchIsASeparateDocumentFromTheMasterSwitch(t *testing.T) {
	db := testDB(t)
	master, bulk := repository.NewAutomationRepository(db), repository.NewBulkSwitchRepository(db)
	ctx := context.Background()
	if err := bulk.SetEnabled(ctx, true, "admin@x.vn"); err != nil {
		t.Fatal(err)
	}
	if on, _ := master.Enabled(ctx); on {
		t.Fatal("turning the bulk switch on must not turn the master switch on")
	}
	if on, _ := bulk.Enabled(ctx); !on {
		t.Fatal("the bulk switch is on")
	}
}
