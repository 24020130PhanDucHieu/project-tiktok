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
