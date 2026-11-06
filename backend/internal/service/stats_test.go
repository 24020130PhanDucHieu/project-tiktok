package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

type fakeKocStats struct {
	mu       sync.Mutex
	stages   []domain.StageStat
	funnels  []domain.BatchFunnel
	byKind   map[domain.TaskKind][]domain.TaskKoc
	limits   map[domain.TaskKind]int64
	taskErr  error
	stageErr error
}

func (f *fakeKocStats) StageStats(context.Context) ([]domain.StageStat, error) {
	return f.stages, f.stageErr
}
func (f *fakeKocStats) Funnels(context.Context) ([]domain.BatchFunnel, error) { return f.funnels, nil }
func (f *fakeKocStats) FindTasks(_ context.Context, kind domain.TaskKind, limit int64) ([]domain.TaskKoc, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.limits == nil {
		f.limits = map[domain.TaskKind]int64{}
	}
	f.limits[kind] = limit
	return f.byKind[kind], f.taskErr
}

func TestDashboardAddsUpStagesAndListsEveryTaskKind(t *testing.T) {
	f := &fakeKocStats{
		stages:  []domain.StageStat{{Stage: "AGREED", Count: 3, VideoGmv: 100}, {Stage: "DELIVERED", Count: 2, VideoGmv: 50.5}},
		funnels: []domain.BatchFunnel{{Campaign: "Thang 11", Pipeline: 5}},
		byKind: map[domain.TaskKind][]domain.TaskKoc{
			domain.TaskNeedShip: {{ID: "k1", TiktokID: "a"}},
			domain.TaskStalled:  {{ID: "k2", TiktokID: "b"}, {ID: "k3", TiktokID: "c"}},
		},
	}
	d, err := NewStatsService(f).Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.Total != 5 || d.VideoGmv != 150.5 || d.ByStage["AGREED"] != 3 || d.ByStage["DELIVERED"] != 2 {
		t.Fatalf("totals: %+v", d)
	}
	if len(d.Funnels) != 1 || d.Funnels[0].Campaign != "Thang 11" {
		t.Fatalf("funnels: %+v", d.Funnels)
	}
	for _, key := range []string{"needCall", "needApproval", "needShip", "delayedShipments", "stalled", "pendingReturn"} {
		if _, ok := d.Tasks[key]; !ok {
			t.Fatalf("task list %q is missing: %v", key, d.Tasks)
		}
	}
	if len(d.Tasks["stalled"]) != 2 || d.Tasks["needShip"][0].ID != "k1" {
		t.Fatalf("tasks: %+v", d.Tasks)
	}
	for kind, limit := range f.limits {
		if limit != 30 {
			t.Fatalf("every list shows 30 KOCs at most, %s asked for %d", kind, limit)
		}
	}
}

func TestDashboardFailsAsAWholeWhenAnyPartFails(t *testing.T) {
	boom := errors.New("database down")
	if _, err := NewStatsService(&fakeKocStats{taskErr: boom}).Dashboard(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("a failing task list must fail the dashboard, got %v", err)
	}
	if _, err := NewStatsService(&fakeKocStats{stageErr: boom}).Dashboard(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("a failing stage count must fail the dashboard, got %v", err)
	}
}
