package domain

import "testing"

func TestCollabRecompute(t *testing.T) {
	c := &Collab{}
	steps := []struct {
		mutate func()
		step   int
	}{
		{func() {}, 0},
		{func() { c.FeeSet, c.CastFee = true, 400000 }, 1},
		{func() { c.Mode = ModeTargeted }, 2},
		{func() { c.Samples = []SampleRequest{{Status: SampleApproved}} }, 3},
		{func() { c.CommissionRate = 12; c.Videos = []CollabVideo{{Views: 2000, Revenue: 636000}} }, 4},
	}
	for i, s := range steps {
		s.mutate()
		c.Recompute()
		if c.Step != s.step {
			t.Fatalf("stage %d: step=%d want %d", i, c.Step, s.step)
		}
	}
	// 636.000 - 12% commission (76.320) - 400.000 cast fee = 159.680 profit
	if c.Perf.Commission != 76320 || c.Perf.Profit != 159680 || c.Perf.CostPerMille != 200000 {
		t.Fatalf("perf: %+v", c.Perf)
	}
	c.Status, c.Outcome = CollabClosed, OutcomePosted
	if c.Owed() != 400000 {
		t.Fatal("unpaid posted collab must be owed")
	}
	c.FeePaid = true
	if c.Owed() != 0 {
		t.Fatal("paid collab owes nothing")
	}
}

func TestRealTikTokSalesReplaceTheTypedInFigures(t *testing.T) {
	c := Collab{FeeSet: true, Mode: "open", CastFee: 100000, CommissionRate: 10, ShareRevenue: 50000, ShareOrders: 1,
		Videos: []CollabVideo{{URL: "u", Views: 999, Orders: 99, Revenue: 9999999}},
		Real:   &CollabReal{Orders: 3, GMV: 372266, Commission: 28083, CommissionConfirmed: 12000, Videos: 2, Views: 1500, ViewsKnown: true}}
	c.Recompute()
	p := c.Perf
	if p.Source != "tiktok" || p.Orders != 4 || p.Videos != 2 || p.TotalViews != 1500 {
		t.Fatalf("typed-in video figures must be ignored once TikTok data exists: %+v", p)
	}
	if p.Revenue != 372266+50000 {
		t.Fatalf("revenue = TikTok GMV + revenue outside TikTok, got %v", p.Revenue)
	}
	if p.Commission != 28083+5000 || p.CommissionConfirmed != 12000 {
		t.Fatalf("commission = real commission + outside revenue at the agreed rate, got %v", p.Commission)
	}
	if p.Profit != p.Revenue-p.Commission-100000 {
		t.Fatalf("profit %v", p.Profit)
	}
	if c.Step != 4 {
		t.Fatalf("a real video moves the collaboration to the result step, got %d", c.Step)
	}
}

func TestWithoutRealDataTheTypedInFiguresStillCount(t *testing.T) {
	c := Collab{FeeSet: true, Mode: "open", CommissionRate: 10, Videos: []CollabVideo{{Views: 100, Orders: 2, Revenue: 1000}}}
	c.Recompute()
	if c.Perf.Source != "manual" || c.Perf.Orders != 2 || c.Perf.Revenue != 1000 || c.Perf.Commission != 100 {
		t.Fatalf("%+v", c.Perf)
	}
}

func TestUnknownViewsAreNotReportedAsZeroCost(t *testing.T) {
	c := Collab{FeeSet: true, Mode: "open", CastFee: 100000, Real: &CollabReal{Videos: 1, ViewsKnown: false}}
	c.Recompute()
	if c.Perf.TotalViews != 0 || c.Perf.CostPerMille != 0 {
		t.Fatalf("unknown views must not give a cost per mille: %+v", c.Perf)
	}
}
