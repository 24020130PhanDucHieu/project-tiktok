package service

import (
	"context"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestPickByNameTakesTheChosenKocsInTheOrderChosenAndLeavesOutTheOnesThatMayNotBeContacted(t *testing.T) {
	a, b := pickKoc(1, "SHIPPING", "", nil), pickKoc(2, "REPLIED", "", nil)
	blocked := pickKoc(3, "SHIPPING", "", func(k *domain.Koc) { k.Dnc = true })
	archived := pickKoc(4, "SHIPPING", "", func(k *domain.Koc) { k.Status = domain.KocArchived })
	p := NewKocPicker(&fakeSearcher{kocs: []domain.Koc{a, b, blocked, archived}})

	got, err := p.PickBy(context.Background(), domain.PickSpec{KocIDs: []string{b.ID.Hex(), blocked.ID.Hex(), archived.ID.Hex(), domain.NewID().Hex(), a.ID.Hex()}, Stages: []string{"DORMANT"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ids := pickIDs(got); len(ids) != 2 || ids[0] != b.ID.Hex() || ids[1] != a.ID.Hex() {
		t.Fatalf("the chosen KOCs in the order chosen, without the blocked, archived and gone ones; the filters and the limit are not used: %v", ids)
	}
}

func TestPickBySourceKeepsOnlyKocsFromThatSource(t *testing.T) {
	a := pickKoc(1, "SHIPPING", "", func(k *domain.Koc) { k.Source = "TikToday" })
	b := pickKoc(2, "SHIPPING", "", func(k *domain.Koc) { k.Source = "TikTok" })
	got, err := NewKocPicker(&fakeSearcher{kocs: []domain.Koc{a, b}}).PickBy(context.Background(), domain.PickSpec{Source: "TikTok", Limit: 10})
	if err != nil || len(got) != 1 || got[0].KocID != b.ID.Hex() {
		t.Fatalf("only the source asked for: %v %v", pickIDs(got), err)
	}
}
