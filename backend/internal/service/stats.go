package service

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/sync/errgroup"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

// StatsService builds dashboard figures from MongoDB aggregations (no full collection reads).
type StatsService struct{ kocs *repository.KocRepository }

// NewStatsService builds a StatsService.
func NewStatsService(kocs *repository.KocRepository) *StatsService { return &StatsService{kocs: kocs} }

// Dashboard is the payload of GET /kocs/stats.
type Dashboard struct {
	Total    int64                           `json:"total"`
	ByStage  map[string]int64                `json:"byStage"`
	VideoGmv float64                         `json:"videoGmv"`
	Funnels  []repository.BatchFunnel        `json:"funnels"`
	Tasks    map[string][]repository.TaskKoc `json:"tasks"`
}

// Dashboard collects stage counts, per-campaign funnels and today's task lists in parallel.
func (s *StatsService) Dashboard(ctx context.Context) (*Dashboard, error) {
	d := &Dashboard{ByStage: map[string]int64{}, Tasks: map[string][]repository.TaskKoc{}}
	var stages []repository.StageStat
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { stages, err = s.kocs.StageStats(gctx); return })
	g.Go(func() (err error) { d.Funnels, err = s.kocs.Funnels(gctx); return })

	tasks := []struct {
		key  string
		cond bson.M
	}{
		{"needCall", bson.M{"callTaskPending": true}},
		{"needApproval", bson.M{"stage": domain.StageAwaitingApproval}},
		{"needShip", bson.M{"stage": domain.StageAgreed}},
		{"delayedShipments", bson.M{"shippingDelayedAlert": true}},
		{"stalled", bson.M{"$or": []bson.M{{"stage": domain.StageStalled}, {"stalledFlag": true}}}},
		{"pendingReturn", bson.M{"stage": bson.M{"$in": []string{domain.StageNotFit, domain.StageReturning}}}},
	}
	results := make([][]repository.TaskKoc, len(tasks))
	for i, t := range tasks {
		i, t := i, t
		g.Go(func() (err error) { results[i], err = s.kocs.FindTasks(gctx, t.cond, 30); return })
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	for _, st := range stages {
		d.ByStage[st.Stage] = st.Count
		d.Total += st.Count
		d.VideoGmv += st.VideoGmv
	}
	for i, t := range tasks {
		d.Tasks[t.key] = results[i]
	}
	return d, nil
}
