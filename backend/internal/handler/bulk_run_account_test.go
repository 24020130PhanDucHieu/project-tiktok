package handler

import (
	"context"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// startNotes is a run starter that notes the options of every run it is asked to start.
type startNotes struct {
	opts []service.RunOptions
	err  error
}

func (s *startNotes) Start(_ context.Context, _, name string, steps []domain.CampaignStep, targets []domain.RunTarget, opts ...service.RunOptions) (*domain.BulkRun, error) {
	if s.err != nil {
		return nil, s.err
	}
	var o service.RunOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	s.opts = append(s.opts, o)
	return &domain.BulkRun{ID: domain.NewID(), CampaignName: name, Steps: steps, Targets: targets, ZaloAccountID: o.ZaloAccountID}, nil
}
func (s *startNotes) Estimate(int, int) time.Duration { return time.Minute }

type noRuns struct{}

func (noRuns) Running(context.Context) (*domain.BulkRun, error)        { return nil, nil }
func (noRuns) Recent(context.Context, int64) ([]domain.BulkRun, error) { return nil, nil }

type fixedDailyLimit struct{}

func (fixedDailyLimit) Get(context.Context) int        { return 30 }
func (fixedDailyLimit) Set(context.Context, int) error { return nil }

type anyKoc struct{}

func (anyKoc) FindActive(_ context.Context, id string) (*domain.Koc, error) {
	k := domain.NewKoc()
	k.TiktokID, k.DisplayName = "u"+id, "KOC "+id
	return k, nil
}

type picturesExist struct{}

func (picturesExist) ImagesExist(context.Context, []domain.ID) (bool, error) { return true, nil }

func bulkApp(st *startNotes) *fiber.App {
	h := NewBulkRunHandler(st, noRuns{}, fixedDailyLimit{}, anyKoc{}, picturesExist{}, &fakeAudit{})
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/runs", h.Start)
	return app
}

func TestARunIsStartedForTheAccountThePersonChose(t *testing.T) {
	st := &startNotes{}
	body := `{"campaignName":"Mở đầu","steps":[{"kind":"text","text":"Chào {ten}"}],"kocIds":["1","2"],"confirm":true,"zaloAccountId":"acc-B"}`
	if code, raw := postJSON(bulkApp(st), "/runs", body); code != 201 {
		t.Fatalf("%d %v", code, raw)
	}
	if len(st.opts) != 1 || st.opts[0].ZaloAccountID != "acc-B" {
		t.Fatalf("opts = %+v, want the account the person chose", st.opts)
	}
}

func TestARunWithNoAccountChosenLeavesItToTheDefault(t *testing.T) {
	st := &startNotes{}
	body := `{"campaignName":"Mở đầu","steps":[{"kind":"text","text":"Chào"}],"kocIds":["1"],"confirm":true}`
	if code, raw := postJSON(bulkApp(st), "/runs", body); code != 201 {
		t.Fatalf("%d %v", code, raw)
	}
	if len(st.opts) != 1 || st.opts[0].ZaloAccountID != "" {
		t.Fatalf("opts = %+v", st.opts)
	}
}

func TestAnAccountProblemIsAnswerdToThePerson(t *testing.T) {
	st := &startNotes{err: domain.Conflict("Tài khoản Zalo \"B\" đã bị ngắt kết nối.")}
	body := `{"campaignName":"x","steps":[{"kind":"text","text":"Chào"}],"kocIds":["1"],"confirm":true,"zaloAccountId":"acc-B"}`
	code, raw := postJSON(bulkApp(st), "/runs", body)
	if code != 409 {
		t.Fatalf("%d %v, want 409", code, raw)
	}
}
