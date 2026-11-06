package service

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// KocStatsStore reads the numbers the dashboard shows (implemented by the repository package).
type KocStatsStore interface {
	StageStats(ctx context.Context) ([]domain.StageStat, error)
	Funnels(ctx context.Context) ([]domain.BatchFunnel, error)
	FindTasks(ctx context.Context, kind domain.TaskKind, limit int64) ([]domain.TaskKoc, error)
}

// StatsService builds dashboard figures from aggregations (no full collection reads).
type StatsService struct{ kocs KocStatsStore }

// NewStatsService builds a StatsService.
func NewStatsService(kocs KocStatsStore) *StatsService { return &StatsService{kocs: kocs} }

// Dashboard is the payload of GET /kocs/stats.
type Dashboard struct {
	Total    int64                       `json:"total"`
	ByStage  map[string]int64            `json:"byStage"`
	VideoGmv float64                     `json:"videoGmv"`
	Funnels  []domain.BatchFunnel        `json:"funnels"`
	Tasks    map[string][]domain.TaskKoc `json:"tasks"`
}

// dashboardTaskLimit is how many KOCs each task list shows.
const dashboardTaskLimit = 30

// Dashboard collects stage counts, per-campaign funnels and today's task lists in parallel.
func (s *StatsService) Dashboard(ctx context.Context) (*Dashboard, error) {
	d := &Dashboard{ByStage: map[string]int64{}, Tasks: map[string][]domain.TaskKoc{}}
	var stages []domain.StageStat
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { stages, err = s.kocs.StageStats(gctx); return })
	g.Go(func() (err error) { d.Funnels, err = s.kocs.Funnels(gctx); return })

	results := make([][]domain.TaskKoc, len(domain.AllTaskKinds))
	for i, kind := range domain.AllTaskKinds {
		i, kind := i, kind
		g.Go(func() (err error) { results[i], err = s.kocs.FindTasks(gctx, kind, dashboardTaskLimit); return })
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	for _, st := range stages {
		d.ByStage[st.Stage] = st.Count
		d.Total += st.Count
		d.VideoGmv += st.VideoGmv
	}
	for i, kind := range domain.AllTaskKinds {
		d.Tasks[string(kind)] = results[i]
	}
	return d, nil
}
