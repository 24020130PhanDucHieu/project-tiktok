package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

// The contact mark is how the 24 hour rule survives two senders at once: the store decides in one step, so a copy of
// the KOC read earlier can never lead to a second message.
func TestClaimContactMarksAKocOnlyOnceInTheWindow(t *testing.T) {
	db := testDB(t)
	repo := repository.NewKocRepository(db)
	ctx := context.Background()
	koc := domain.NewKoc()
	koc.TiktokID, koc.DisplayName = "claim_me", "Claim Me"
	if err := repo.Create(ctx, koc); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Truncate(time.Millisecond)

	ok, prev, err := repo.ClaimContact(ctx, koc.ID, t0.Add(-24*time.Hour), t0)
	if err != nil || !ok || prev != nil {
		t.Fatalf("the first claim wins and there was no mark before: ok=%v prev=%v err=%v", ok, prev, err)
	}
	if ok, _, _ := repo.ClaimContact(ctx, koc.ID, t0.Add(time.Second-24*time.Hour), t0.Add(time.Second)); ok {
		t.Fatal("a second claim within 24 hours must lose")
	}
	if err := repo.Save(ctx, koc); err == nil {
		t.Fatal("a copy read before the claim must not be saved over the mark (the claim bumps the version)")
	}
}

func TestReleaseContactPutsTheOldMarkBackOnlyWhileTheClaimIsStillOurs(t *testing.T) {
	db := testDB(t)
	repo := repository.NewKocRepository(db)
	ctx := context.Background()
	koc := domain.NewKoc()
	koc.TiktokID, koc.DisplayName = "release_me", "Release Me"
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Millisecond)
	koc.LastContactedAt = &old
	if err := repo.Create(ctx, koc); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)

	ok, prev, err := repo.ClaimContact(ctx, koc.ID, now.Add(-24*time.Hour), now)
	if err != nil || !ok || prev == nil || !prev.Equal(old) {
		t.Fatalf("a mark older than 24 hours can be claimed and is reported back: ok=%v prev=%v err=%v", ok, prev, err)
	}
	if err := repo.ReleaseContact(ctx, koc.ID, now.Add(time.Hour), prev); err != nil { // not our claim: nothing changes
		t.Fatal(err)
	}
	if ok, _, _ := repo.ClaimContact(ctx, koc.ID, now.Add(-24*time.Hour), now.Add(2*time.Second)); ok {
		t.Fatal("a release that does not match the claim must leave the mark in place")
	}
	if err := repo.ReleaseContact(ctx, koc.ID, now, prev); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindActiveByID(ctx, koc.ID.Hex())
	if err != nil || got == nil || got.LastContactedAt == nil || !got.LastContactedAt.Equal(old) {
		t.Fatalf("our own release restores the old mark: %+v %v", got, err)
	}
}
