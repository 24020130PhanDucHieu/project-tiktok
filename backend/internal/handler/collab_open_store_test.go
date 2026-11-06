package handler

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
)

// Fakes embed the interface and implement only what the route needs: any other call panics, which would show that
// the handler touches something the test did not expect.
type fakeCollabKocs struct {
	CollabKocStore
	koc   *domain.Koc
	saved int
}

func (f *fakeCollabKocs) FindActiveByID(context.Context, string) (*domain.Koc, error) {
	return f.koc, nil
}
func (f *fakeCollabKocs) Save(context.Context, *domain.Koc) error { f.saved++; return nil }

type fakeCollabStore struct {
	CollabStore
	active  *domain.Collab
	created []*domain.Collab
}

func (f *fakeCollabStore) Active(context.Context, domain.ID) (*domain.Collab, error) {
	return f.active, nil
}
func (f *fakeCollabStore) Create(_ context.Context, c *domain.Collab) error {
	// what the real repository does when it inserts: numbers it, makes it active, stamps the dates
	c.ID, c.Seq, c.Status = domain.NewID(), len(f.created)+1, domain.CollabActive
	c.Normalize()
	c.Recompute()
	f.created = append(f.created, c)
	return nil
}

type noConvs struct{ ConversationStore }

func (noConvs) FindByHandle(context.Context, string) (*domain.Conversation, error) { return nil, nil }

type noMsgs struct{ MessageStore }

func openCollabApp(kocs *fakeCollabKocs, collabs *fakeCollabStore) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/kocs/:id/collabs", NewCollabHandler(kocs, collabs, noConvs{}, noMsgs{}).Open)
	return app
}

func TestOpeningACollaborationNeedsAnExistingKoc(t *testing.T) {
	kocs, collabs := &fakeCollabKocs{}, &fakeCollabStore{}
	code, body := postJSON(openCollabApp(kocs, collabs), "/kocs/66f1a2b3c4d5e6f7a8b9c0d1/collabs", `{"castFee":300000}`)
	if code != 404 || len(collabs.created) != 0 {
		t.Fatalf("%d %v created %d", code, body, len(collabs.created))
	}
}

func TestOnlyOneCollaborationCanBeActivePerKoc(t *testing.T) {
	kocs := &fakeCollabKocs{koc: &domain.Koc{ID: domain.NewID(), TiktokID: "emanhthethao"}}
	collabs := &fakeCollabStore{active: &domain.Collab{ID: domain.NewID()}}
	code, _ := postJSON(openCollabApp(kocs, collabs), "/kocs/x/collabs", `{"castFee":300000}`)
	if code != 409 || len(collabs.created) != 0 || kocs.saved != 0 {
		t.Fatalf("a second active collaboration must be refused: %d created %d saved %d", code, len(collabs.created), kocs.saved)
	}
}

func TestOpeningACollaborationStoresItAndMirrorsTheFeeOnTheKoc(t *testing.T) {
	kocs := &fakeCollabKocs{koc: &domain.Koc{ID: domain.NewID(), TiktokID: "emanhthethao"}}
	collabs := &fakeCollabStore{}
	code, body := postJSON(openCollabApp(kocs, collabs), "/kocs/x/collabs", `{"castFee":300000}`)
	if code != 201 || len(collabs.created) != 1 {
		t.Fatalf("%d %v created %d", code, body, len(collabs.created))
	}
	c := collabs.created[0]
	if !c.FeeSet || c.CastFee != 300000 || c.KocID != kocs.koc.ID {
		t.Fatalf("collaboration: %+v", c)
	}
	if kocs.saved != 1 || kocs.koc.CastFee != 300000 {
		t.Fatalf("the KOC must mirror the fee: saved %d, fee %v", kocs.saved, kocs.koc.CastFee)
	}
}

func TestACastFeeOutOfRangeIsRefusedBeforeAnythingIsStored(t *testing.T) {
	kocs := &fakeCollabKocs{koc: &domain.Koc{ID: domain.NewID()}}
	collabs := &fakeCollabStore{}
	code, _ := postJSON(openCollabApp(kocs, collabs), "/kocs/x/collabs", `{"castFee":-5}`)
	if code != 400 || len(collabs.created) != 0 {
		t.Fatalf("%d created %d", code, len(collabs.created))
	}
}
