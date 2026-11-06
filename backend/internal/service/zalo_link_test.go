package service

import (
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestMatchZaloOwnerPicksTheThreadWhosePhoneResolvesToTheSender(t *testing.T) {
	threads := []domain.Conversation{
		{ContactName: "a", ContactPhone: "0900000001"},
		{ContactName: "b", ContactPhone: "0900000002"},
		{ContactName: "c", ContactPhone: "0900000003"},
	}
	accounts := map[string]string{"0900000001": "u-1", "0900000002": "u-2", "0900000003": "u-3"}
	var asked []string
	got := matchZaloOwner(threads, "u-2", func(p string) string { asked = append(asked, p); return accounts[p] })
	if got == nil || got.ContactName != "b" {
		t.Fatalf("got %+v", got)
	}
	if len(asked) != 2 {
		t.Fatalf("it must stop at the first match, asked %v", asked)
	}
}

func TestMatchZaloOwnerFindsNobodyWhenNoPhoneBelongsToTheSender(t *testing.T) {
	threads := []domain.Conversation{{ContactPhone: "1"}, {ContactPhone: "2"}}
	if got := matchZaloOwner(threads, "stranger", func(p string) string { return "u-" + p }); got != nil {
		t.Fatalf("a stranger must not be linked to a KOC: %+v", got)
	}
	if got := matchZaloOwner(threads, "u-1", func(string) string { return "" }); got != nil {
		t.Fatal("phones without a Zalo account never match")
	}
}
