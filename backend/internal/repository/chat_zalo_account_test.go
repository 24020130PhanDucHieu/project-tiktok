package repository

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestAZaloThreadIsFoundUnderItsAccount(t *testing.T) {
	ctx := domain.WithZaloAccount(context.Background(), "acc-A")
	f := zaloThreadFilter(ctx, "uid-1")
	want := bson.M{"channel": domain.ChannelZalo, "externalContactId": "uid-1", "zaloAccountId": "acc-A"}
	if len(f) != len(want) || f["zaloAccountId"] != "acc-A" || f["externalContactId"] != "uid-1" || f["channel"] != domain.ChannelZalo {
		t.Fatalf("filter = %v, want %v", f, want)
	}
}

func TestTheSameKocOnTwoAccountsIsTwoThreads(t *testing.T) {
	a := zaloThreadFilter(domain.WithZaloAccount(context.Background(), "acc-A"), "uid-1")
	b := zaloThreadFilter(domain.WithZaloAccount(context.Background(), "acc-B"), "uid-1")
	if a["zaloAccountId"] == b["zaloAccountId"] {
		t.Fatal("two accounts look for the same thread")
	}
}

func TestWithoutAnAccountAnyThreadOfTheContactMatches(t *testing.T) {
	f := zaloThreadFilter(context.Background(), "uid-1")
	if _, has := f["zaloAccountId"]; has {
		t.Fatalf("filter = %v: no account named, so none may be required", f)
	}
}

func TestBindingAThreadStampsItsAccount(t *testing.T) {
	set := zaloContactSet(domain.WithZaloAccount(context.Background(), "acc-A"), domain.ZaloContact{Channel: domain.ChannelZalo, ExternalContactID: "uid-1", Phone: "0988", Name: "Linh"})
	if set["zaloAccountId"] != "acc-A" || set["externalContactId"] != "uid-1" || set["contactPhone"] != "0988" || set["contactName"] != "Linh" {
		t.Fatalf("set = %v", set)
	}
}

func TestBindingWithoutAnAccountLeavesTheStampedOneAlone(t *testing.T) {
	set := zaloContactSet(context.Background(), domain.ZaloContact{Channel: domain.ChannelZalo, ExternalContactID: "uid-1"})
	if _, has := set["zaloAccountId"]; has {
		t.Fatalf("set = %v: an unnamed account must not erase the stored one", set)
	}
}

func TestTheBackfillTouchesOnlyBoundThreadsWithoutAnAccount(t *testing.T) {
	f := backfillZaloAccountFilter()
	if f["zaloAccountId"].(bson.M)["$exists"] != false {
		t.Fatalf("filter = %v: must only take threads that have no account yet", f)
	}
	ext := f["externalContactId"].(bson.M)
	if ext["$exists"] != true || ext["$ne"] != "" {
		t.Fatalf("filter = %v: must only take threads bound to a Zalo user", f)
	}
}

func TestTheInboxCanBeFilteredByZaloAccount(t *testing.T) {
	q := listOpenQuery(domain.ConversationFilter{ZaloAccountID: "acc-A"})
	if q["zaloAccountId"] != "acc-A" {
		t.Fatalf("query = %v, want only the threads of acc-A", q)
	}
}

func TestWithoutAnAccountTheInboxShowsEveryAccount(t *testing.T) {
	q := listOpenQuery(domain.ConversationFilter{})
	if _, has := q["zaloAccountId"]; has {
		t.Fatalf("query = %v: no account asked for, none may be required", q)
	}
	if q["status"] != domain.ConversationOpen {
		t.Fatalf("query = %v: the existing conditions must stay", q)
	}
}

func TestARunsDailyCountIsKeptPerAccount(t *testing.T) {
	since := time.Now()
	f := startedSinceFilter(domain.WithZaloAccount(context.Background(), "acc-A"), since)
	if f["zaloAccountId"] != "acc-A" {
		t.Fatalf("filter = %v: only the runs of acc-A may count against acc-A", f)
	}
	if _, has := f["targets.startedAt"]; !has {
		t.Fatalf("filter = %v: the day must still bound the count", f)
	}
}

func TestWithoutAnAccountEveryRunCounts(t *testing.T) {
	f := startedSinceFilter(context.Background(), time.Now())
	if _, has := f["zaloAccountId"]; has {
		t.Fatalf("filter = %v: no account named, so all runs count as before", f)
	}
}
