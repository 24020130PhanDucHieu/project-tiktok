package service

import (
	"context"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// bulkDirectory is the KOC store of a bulk import: it answers "already there?" from a table and records what is saved.
type bulkDirectory struct {
	KocDirectory
	existing map[string]*domain.Koc // by tiktok id
	created  []*domain.Koc
}

func (d *bulkDirectory) FindByIdentity(_ context.Context, tiktokID, _ string) (*domain.Koc, error) {
	return d.existing[tiktokID], nil
}
func (d *bulkDirectory) Create(_ context.Context, k *domain.Koc) error {
	d.created = append(d.created, k)
	return nil
}
func (d *bulkDirectory) Save(_ context.Context, k *domain.Koc) error {
	d.created = append(d.created, k)
	return nil
}

type bulkThreads struct {
	KocThreads
	ensured int
}

func (t *bulkThreads) EnsureKocContact(context.Context, domain.KocContact) (*domain.Conversation, error) {
	t.ensured++
	return &domain.Conversation{}, nil
}

// bulkVerifier is TikTok Shop's Creator Marketplace: it knows the creators in `known`, by exact username.
type bulkVerifier struct {
	known map[string]*TargetCreator
	asked []string
}

func (v *bulkVerifier) ResolveCreator(_ context.Context, username string) (*TargetCreator, error) {
	v.asked = append(v.asked, username)
	if c, ok := v.known[username]; ok {
		return c, nil
	}
	return nil, ErrCreatorNotFound
}

func newBulkService(dir *bulkDirectory, th *bulkThreads, v *bulkVerifier) *KocService {
	return (&KocService{kocs: dir, convs: th}).WithCreatorVerifier(v)
}

func TestBulkImportKeepsTheContactFieldsAndTakesTheProfileFromTheMarketplace(t *testing.T) {
	dir, th := &bulkDirectory{}, &bulkThreads{}
	v := &bulkVerifier{known: map[string]*TargetCreator{"abc": {Username: "abc", Nickname: "Abc Shop", AvatarURL: "https://a/b.jpg", Followers: 3100}}}
	fee := 500000.0

	report, err := newBulkService(dir, th, v).BulkImport(context.Background(), []BulkItem{{
		TiktokID: "@Abc", Email: " Abc@X.vn ", Recipient: "Lan", ShippingAddress: "1 A, Hà Nội", Phone: "0976451571", CastFee: &fee,
	}}, "TikToday")
	if err != nil || len(report.Imported) != 1 || len(dir.created) != 1 {
		t.Fatalf("one KOC is imported: %v %+v", err, report)
	}
	k := dir.created[0]
	if k.TiktokID != "abc" || k.Email != "abc@x.vn" || k.Recipient != "Lan" || k.ShippingAddress != "1 A, Hà Nội" || k.Phone != "0976451571" {
		t.Fatalf("the contact fields are kept (email lowercased and trimmed): %+v", k)
	}
	if k.DisplayName != "Abc Shop" || k.AvatarURL != "https://a/b.jpg" || k.Followers != 3100 {
		t.Fatalf("name, picture and followers come from TikTok Shop: %+v", k)
	}
	if k.CastFee != 500000 || k.CastType != "cast" || k.Category != "Chưa phân loại" {
		t.Fatalf("cast and folder: %+v", k)
	}
	if th.ensured != 1 {
		t.Fatalf("the conversation is made once, got %d", th.ensured)
	}
}

func TestBulkImportReportsAnUnlistedCreatorWithItsTikTokIDAndSavesNothing(t *testing.T) {
	dir, th := &bulkDirectory{}, &bulkThreads{}
	report, err := newBulkService(dir, th, &bulkVerifier{}).BulkImport(context.Background(), []BulkItem{{TiktokID: "ABC"}}, "TikToday")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.NotVerified) != 1 || report.NotVerified[0]["tiktokId"] != "abc" || report.NotVerified[0]["reason"] == "" {
		t.Fatalf("the row is reported with its id and why: %+v", report.NotVerified)
	}
	if len(dir.created) != 0 || len(report.Imported) != 0 {
		t.Fatalf("nothing is saved: %+v", dir.created)
	}
}

func TestBulkImportSkipsDuplicatesAndBlocksCollaborated(t *testing.T) {
	dir := &bulkDirectory{existing: map[string]*domain.Koc{
		"one": {TiktokID: "one", Status: domain.KocActive},
		"two": {TiktokID: "two", DaHopTac: true, Status: domain.KocActive},
	}}
	v := &bulkVerifier{}
	report, err := newBulkService(dir, &bulkThreads{}, v).BulkImport(context.Background(), []BulkItem{{TiktokID: "one"}, {TiktokID: "two"}}, "TikToday")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SkippedDuplicates) != 1 || report.SkippedDuplicates[0]["tiktokId"] != "one" {
		t.Fatalf("an active duplicate is skipped: %+v", report.SkippedDuplicates)
	}
	if len(report.BlockedCollaborated) != 1 || report.BlockedCollaborated[0]["tiktokId"] != "two" {
		t.Fatalf("a KOC that already worked with the shop is blocked: %+v", report.BlockedCollaborated)
	}
	if len(v.asked) != 0 {
		t.Fatalf("TikTok is not asked about rows that are refused anyway: %v", v.asked)
	}
}
