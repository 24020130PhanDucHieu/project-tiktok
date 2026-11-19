package repository_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

func newTemplate(name string) *domain.WorkflowTemplate {
	return &domain.WorkflowTemplate{
		Name: name, CreatedBy: "a@x.vn",
		Nodes: []domain.WorkflowNode{{ID: "import-1", Type: domain.NodeImport, X: 1, Y: 2}, {ID: "w", Type: domain.NodeWait, WaitMinutes: 60}},
		Edges: []domain.WorkflowEdge{{ID: "e1", From: "import-1", Port: domain.PortOut, To: "w"}},
	}
}

func TestTemplatesAreKeptAndListedMostRecentlyChangedFirst(t *testing.T) {
	repo := repository.NewWorkflowTemplateRepository(testDB(t))
	ctx := context.Background()
	first, second := newTemplate("Một"), newTemplate("Hai")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, first.ID.Hex(), "Một (sửa)", first.Nodes, first.Edges, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	list, err := repo.List(ctx)
	if err != nil || len(list) < 2 || list[0].Name != "Một (sửa)" {
		t.Fatalf("the template that was changed last comes first: %v %v", list, err)
	}
	got, err := repo.Find(ctx, first.ID.Hex())
	if err != nil || len(got.Nodes) != 2 || got.Nodes[1].WaitMinutes != 60 || len(got.Edges) != 1 || got.CreatedBy != "a@x.vn" {
		t.Fatalf("the drawing comes back whole: %+v %v", got, err)
	}
}

func TestTheFiftyFirstTemplateIsRefused(t *testing.T) {
	repo := repository.NewWorkflowTemplateRepository(testDB(t))
	ctx := context.Background()
	existing, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(existing); i < domain.MaxWorkflowTemplates; i++ {
		if err := repo.Create(ctx, newTemplate("m"+strconv.Itoa(i))); err != nil {
			t.Fatalf("template %d: %v", i, err)
		}
	}
	if err := repo.Create(ctx, newTemplate("thừa")); err != repository.ErrTooManyTemplates {
		t.Fatalf("past the limit: %v", err)
	}
}

func TestUpdateAndDeleteOfATemplateThatIsGoneAreNotFound(t *testing.T) {
	repo := repository.NewWorkflowTemplateRepository(testDB(t))
	ctx := context.Background()
	gone := domain.NewID().Hex()
	if err := repo.Update(ctx, gone, "x", newTemplate("x").Nodes, nil, time.Now()); err == nil {
		t.Fatal("updating nothing is a NotFound fault")
	}
	if err := repo.Delete(ctx, gone); err == nil {
		t.Fatal("deleting nothing is a NotFound fault")
	}
	if _, err := repo.Find(ctx, "not-an-id"); err == nil {
		t.Fatal("a malformed id is a NotFound fault")
	}
}
