package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/campaignseed"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

func TestTheSeedIsStoredOnceAndItsPicturesAreThere(t *testing.T) {
	repo := repository.NewCampaignRepository(testDB(t))
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := repo.EnsureSeed(ctx, campaignseed.Campaigns()); err != nil {
			t.Fatal(err)
		}
	}
	list, err := repo.List(ctx)
	if err != nil || len(list) != 3 || list[0].Name != "Liên hệ KOC mới" {
		t.Fatalf("three campaigns after seeding twice, the first one first: %d %v", len(list), err)
	}
	var ids []domain.ID
	for _, s := range list[0].Steps {
		if s.Kind == domain.StepImage {
			ids = append(ids, s.ImageID)
		}
	}
	if ok, err := repo.ImagesExist(ctx, ids); !ok || err != nil || len(ids) != 2 {
		t.Fatalf("both pictures are stored: %v %v %v", ids, ok, err)
	}
	img, err := repo.Image(ctx, ids[0].Hex())
	if err != nil || img.Width != 968 || img.ContentType != "image/png" || len(img.Data) == 0 {
		t.Fatalf("the picture comes back whole: %+v %v", img, err)
	}
	if ok, _ := repo.ImagesExist(ctx, []domain.ID{domain.NewID()}); ok {
		t.Fatal("an unknown picture does not exist")
	}
}

func TestSaveStepsKeepsTheOrder(t *testing.T) {
	repo := repository.NewCampaignRepository(testDB(t))
	ctx := context.Background()
	if err := repo.EnsureSeed(ctx, campaignseed.Campaigns()); err != nil {
		t.Fatal(err)
	}
	list, _ := repo.List(ctx)
	steps := []domain.CampaignStep{{Kind: domain.StepText, Text: "b"}, {Kind: domain.StepText, Text: "a"}}
	if err := repo.SaveSteps(ctx, list[1].ID.Hex(), steps, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Find(ctx, list[1].ID.Hex())
	if err != nil || len(got.Steps) != 2 || got.Steps[0].Text != "b" || got.Steps[1].Text != "a" {
		t.Fatalf("order kept: %+v %v", got, err)
	}
	if _, err := repo.Find(ctx, domain.NewID().Hex()); err == nil {
		t.Fatal("an unknown campaign is NotFound")
	}
}
