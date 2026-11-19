package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

func newWorkflow() *domain.Workflow {
	return &domain.Workflow{
		Name: "w", By: "a@x.vn", Source: "TikToday",
		Nodes: []domain.WorkflowNode{{ID: "i", Type: domain.NodeImport}, {ID: "w", Type: domain.NodeWait, WaitMinutes: 60}},
		Edges: []domain.WorkflowEdge{{ID: "e1", From: "i", Port: domain.PortOut, To: "w"}},
	}
}

func TestOnlyOneWorkflowRuns(t *testing.T) {
	repo := repository.NewWorkflowRepository(testDB(t))
	ctx := context.Background()
	first := newWorkflow()
	if err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newWorkflow()); !isConflict(err) {
		t.Fatalf("a second running workflow is a conflict: %v", err)
	}
	if wf, err := repo.Running(ctx); err != nil || wf == nil || wf.ID != first.ID {
		t.Fatalf("the running workflow is found: %v %v", wf, err)
	}
	if err := repo.Finish(ctx, first.ID, domain.WorkflowDone, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if wf, _ := repo.Running(ctx); wf != nil {
		t.Fatal("nothing runs after Finish")
	}
	if err := repo.Create(ctx, newWorkflow()); err != nil {
		t.Fatalf("a new workflow may start once the other ended: %v", err)
	}
	if latest, err := repo.Latest(ctx); err != nil || latest == nil || latest.ID == first.ID {
		t.Fatalf("Latest is the newest one, not the first: %v %v", latest, err)
	}
}

func TestSetStateReplacesTheStateOfTheSameNode(t *testing.T) {
	repo := repository.NewWorkflowRepository(testDB(t))
	ctx := context.Background()
	wf := newWorkflow()
	if err := repo.Create(ctx, wf); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Millisecond)
	for _, st := range []domain.NodeState{
		{NodeID: "i", Status: domain.NodeRunning},
		{NodeID: "w", Status: domain.NodeWaiting, WaitUntil: &until, Count: 3},
		{NodeID: "i", Status: domain.NodeDone, Count: 5},
	} {
		if err := repo.SetState(ctx, wf.ID, st); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.Find(ctx, wf.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.States) != 2 {
		t.Fatalf("one state per block: %+v", got.States)
	}
	byNode := map[string]domain.NodeState{}
	for _, s := range got.States {
		byNode[s.NodeID] = s
	}
	if byNode["i"].Status != domain.NodeDone || byNode["i"].Count != 5 || byNode["w"].Status != domain.NodeWaiting || byNode["w"].WaitUntil == nil {
		t.Fatalf("the later state of a block wins and the others are untouched: %+v", got.States)
	}
}

func TestRequestStopOnlyReachesARunningWorkflow(t *testing.T) {
	repo := repository.NewWorkflowRepository(testDB(t))
	ctx := context.Background()
	wf := newWorkflow()
	if err := repo.Create(ctx, wf); err != nil {
		t.Fatal(err)
	}
	if found, err := repo.RequestStop(ctx, wf.ID.Hex()); err != nil || !found {
		t.Fatalf("a running workflow is asked to stop: %v %v", found, err)
	}
	if stop, err := repo.StopRequested(ctx, wf.ID); err != nil || !stop {
		t.Fatalf("the request is kept: %v %v", stop, err)
	}
	if err := repo.Finish(ctx, wf.ID, domain.WorkflowStopped, "stopped_by_user", time.Now()); err != nil {
		t.Fatal(err)
	}
	if found, err := repo.RequestStop(ctx, wf.ID.Hex()); err != nil || found {
		t.Fatalf("an ended workflow has nothing to stop: %v %v", found, err)
	}
}

func TestSaveTokensKeepsTheOrderOfTheKocs(t *testing.T) {
	repo := repository.NewWorkflowRepository(testDB(t))
	ctx := context.Background()
	wf := newWorkflow()
	if err := repo.Create(ctx, wf); err != nil {
		t.Fatal(err)
	}
	sent := time.Now().UTC().Truncate(time.Millisecond)
	tokens := []domain.WorkflowToken{{
		NodeID: "w", ReadyAt: sent.Add(time.Hour), ChildID: "c1",
		Kocs: []domain.TokenKoc{{KocID: "k3"}, {KocID: "k1", SentAt: &sent}, {KocID: "k2"}},
		Done: []domain.TokenKoc{{KocID: "k9", SentAt: &sent}},
	}}
	if err := repo.SaveTokens(ctx, wf.ID, tokens); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Find(ctx, wf.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tokens) != 1 || got.Tokens[0].ChildID != "c1" || len(got.Tokens[0].Done) != 1 {
		t.Fatalf("the token is kept: %+v", got.Tokens)
	}
	ids := []string{got.Tokens[0].Kocs[0].KocID, got.Tokens[0].Kocs[1].KocID, got.Tokens[0].Kocs[2].KocID}
	if ids[0] != "k3" || ids[1] != "k1" || ids[2] != "k2" || got.Tokens[0].Kocs[1].SentAt == nil {
		t.Fatalf("the KOCs keep their order and their send time: %v", ids)
	}
}

func TestAnUnknownWorkflowIsNotFound(t *testing.T) {
	repo := repository.NewWorkflowRepository(testDB(t))
	if _, err := repo.Find(context.Background(), "65a1b2c3d4e5f60718293a4b"); err == nil {
		t.Fatal("a missing workflow is a NotFound fault")
	}
	if _, err := repo.Find(context.Background(), "not-an-id"); err == nil {
		t.Fatal("a malformed id is a NotFound fault")
	}
}
