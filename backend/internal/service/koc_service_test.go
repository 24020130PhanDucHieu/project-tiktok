package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

type fakeDirectory struct {
	KocDirectory
	gotQuery domain.KocQuery
	gotSkip  int64
	gotLimit int64
	existing *domain.Koc
	archived string
}

func (f *fakeDirectory) Search(_ context.Context, q domain.KocQuery, skip, limit int64) ([]domain.Koc, int64, error) {
	f.gotQuery, f.gotSkip, f.gotLimit = q, skip, limit
	return []domain.Koc{{TiktokID: "a"}}, 45, nil
}
func (f *fakeDirectory) FindByIdentity(context.Context, string, string) (*domain.Koc, error) {
	return f.existing, nil
}
func (f *fakeDirectory) Archive(_ context.Context, id string) (*domain.Koc, error) {
	f.archived = id
	if id == "missing" {
		return nil, nil
	}
	return &domain.Koc{TiktokID: "x"}, nil
}

func TestTheDirectoryListTurnsTheScreenFilterIntoAQuery(t *testing.T) {
	d := &fakeDirectory{}
	s := &KocService{kocs: d}
	res, err := s.List(context.Background(), ListFilter{Q: "  @LinH ", Category: "Tất cả", Stage: "ALL", Source: "ALL", Page: 3, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if d.gotQuery.Category != "" || d.gotQuery.Stage != "" || d.gotQuery.Source != "" {
		t.Fatalf("the "+"all"+" choices must not filter: %+v", d.gotQuery)
	}
	if d.gotQuery.Search != "  @LinH " || d.gotQuery.SearchID != "linh" {
		t.Fatalf("free text and its TikTok id form: %+v", d.gotQuery)
	}
	if d.gotSkip != 40 || d.gotLimit != 20 {
		t.Fatalf("page 3 of 20 skips 40: %d %d", d.gotSkip, d.gotLimit)
	}
	if res.Pagination.Total != 45 || res.Pagination.Pages != 3 || res.Pagination.Page != 3 {
		t.Fatalf("%+v", res.Pagination)
	}

	if _, err := s.List(context.Background(), ListFilter{Category: "Làm đẹp", Stage: "AGREED", Source: "Zalo", Limit: 1000}); err != nil {
		t.Fatal(err)
	}
	if d.gotQuery.Category != "Làm đẹp" || d.gotQuery.Stage != "AGREED" || d.gotQuery.Source != "Zalo" || d.gotLimit != 100 || d.gotSkip != 0 {
		t.Fatalf("real filters pass through and the limit is capped at 100: %+v %d %d", d.gotQuery, d.gotLimit, d.gotSkip)
	}
}

func TestAKocThatAlreadyWorkedWithTheShopCannotBeAddedAgain(t *testing.T) {
	d := &fakeDirectory{existing: &domain.Koc{TiktokID: "giadinhtraicay1202", DaHopTac: true, Status: domain.KocActive}}
	_, err := (&KocService{kocs: d}).Create(context.Background(), CreateInput{TiktokID: "@GiaDinhTraiCay1202"})
	if statusOf(t, err) != http.StatusConflict {
		t.Fatalf("expected 409, got %v", err)
	}
}

func TestAnActiveDuplicateIsRefusedByDedupe(t *testing.T) {
	d := &fakeDirectory{existing: &domain.Koc{TiktokID: "abc", Status: domain.KocActive}}
	_, err := (&KocService{kocs: d}).Create(context.Background(), CreateInput{TiktokID: "ABC", Phone: "0900000001"})
	if statusOf(t, err) != http.StatusConflict {
		t.Fatalf("expected 409, got %v", err)
	}
}

func TestArchivingAnUnknownKocIsNotFound(t *testing.T) {
	d := &fakeDirectory{}
	s := &KocService{kocs: d}
	if _, err := s.Archive(context.Background(), "missing"); statusOf(t, err) != http.StatusNotFound {
		t.Fatalf("expected 404, got %v", err)
	}
	if k, err := s.Archive(context.Background(), "ok"); err != nil || k == nil || d.archived != "ok" {
		t.Fatalf("%v %v", k, err)
	}
}
