package repository_test

import (
	"context"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

func TestAddLabelAddsToASetAndKeepsTheOtherLabels(t *testing.T) {
	repo := repository.NewConversationRepository(testDB(t))
	ctx := context.Background()
	conv, err := repo.UpsertKocContact(ctx, domain.KocContact{Handle: "@label.test", Name: "Label Test"})
	if err != nil {
		t.Fatal(err)
	}
	first, second := "65a1b2c3d4e5f60718293a4b", "65a1b2c3d4e5f60718293a4c"
	if err := repo.SetLabels(ctx, conv.ID, []string{first}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // twice: a step that runs again after a restart changes nothing
		if err := repo.AddLabel(ctx, conv.ID, second); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.FindByHandle(ctx, "@label.test")
	if err != nil || got == nil {
		t.Fatalf("%v %v", got, err)
	}
	if len(got.Labels) != 2 || got.Labels[0] != first || got.Labels[1] != second {
		t.Fatalf("the old label stays, the new one is there once: %v", got.Labels)
	}
}
