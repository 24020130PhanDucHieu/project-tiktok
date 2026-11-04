package service

import (
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func collabAt(day string, mutate func(*domain.Collab)) domain.Collab {
	t, _ := time.ParseInLocation("2006-01-02 15:04", day, vnZone)
	c := domain.Collab{StartedAt: t, Status: domain.CollabActive, CastFee: 100000, CommissionRate: 10}
	if mutate != nil {
		mutate(&c)
	}
	c.Recompute()
	return c
}

func TestBuildCollabOverviewTotalsAndBuckets(t *testing.T) {
	cols := []domain.Collab{
		collabAt("2026-11-02 08:00", func(c *domain.Collab) { // Monday
			c.Videos = []domain.CollabVideo{{URL: "u", Views: 1000, Orders: 4, Revenue: 1000000}}
			c.Status, c.Outcome, c.Mode = domain.CollabClosed, domain.OutcomePosted, domain.ModeOpen
		}),
		collabAt("2026-11-04 23:30", nil), // still Wednesday in Vietnam (16:30 UTC)
		collabAt("2026-10-15 10:00", func(c *domain.Collab) { c.Status, c.Outcome = domain.CollabClosed, domain.OutcomeNoPost }),
		collabAt("2025-01-01 10:00", nil), // outside the window
	}
	from, _ := time.ParseInLocation("2006-01-02", "2026-10-01", vnZone)
	to, _ := time.ParseInLocation("2006-01-02", "2026-11-05", vnZone)

	day, err := BuildCollabOverview(cols, GranDay, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Series) != 36 {
		t.Fatalf("a day series must have one bucket per day in the range, got %d", len(day.Series))
	}
	if day.Totals.Collabs != 3 || day.Totals.Active != 1 || day.Totals.Closed != 2 || day.Totals.Posted != 1 {
		t.Fatalf("totals wrong: %+v", day.Totals)
	}
	// 1,000,000 revenue, 10% commission, 100,000 cast fee on the posted one; 100,000 cast fee on each other one.
	if day.Totals.Revenue != 1000000 || day.Totals.Commission != 100000 || day.Totals.CastFee != 300000 {
		t.Fatalf("money wrong: %+v", day.Totals)
	}
	if day.Totals.Profit != 1000000-100000-300000 || day.Totals.Orders != 4 || day.Totals.Views != 1000 {
		t.Fatalf("profit/orders/views wrong: %+v", day.Totals)
	}
	if day.Outcomes[domain.OutcomePosted] != 1 || day.Outcomes[domain.OutcomeNoPost] != 1 || day.Modes[domain.ModeOpen] != 1 || day.Modes["none"] != 2 {
		t.Fatalf("splits wrong: %+v %+v", day.Outcomes, day.Modes)
	}
	var wed OverviewBucket
	for _, b := range day.Series {
		if b.Key == "2026-11-04" {
			wed = b
		}
	}
	if wed.Collabs != 1 {
		t.Fatalf("a collaboration started 23:30 Vietnam time belongs to that day, got %+v", wed)
	}

	week, _ := BuildCollabOverview(cols, GranWeek, from, to)
	var thisWeek OverviewBucket
	for _, b := range week.Series {
		if b.Key == "2026-11-02" {
			thisWeek = b
		}
	}
	if thisWeek.Collabs != 2 {
		t.Fatalf("weeks start on Monday: expected 2 collaborations in the week of 02/11, got %+v", thisWeek)
	}

	month, _ := BuildCollabOverview(cols, GranMonth, from, to)
	if len(month.Series) != 2 || month.Series[0].Collabs != 1 || month.Series[1].Collabs != 2 {
		t.Fatalf("month series wrong: %+v", month.Series)
	}
	year, _ := BuildCollabOverview(cols, GranYear, from, to)
	if len(year.Series) != 1 || year.Series[0].Collabs != 3 {
		t.Fatalf("year series wrong: %+v", year.Series)
	}
}

func TestBuildCollabOverviewValidation(t *testing.T) {
	now := time.Now()
	if _, err := BuildCollabOverview(nil, "hour", now, now); err != ErrBadGranularity {
		t.Fatalf("unknown granularity must be refused, got %v", err)
	}
	if _, err := BuildCollabOverview(nil, GranDay, now.AddDate(-5, 0, 0), now); err == nil {
		t.Fatal("a range that would need too many buckets must be refused")
	}
	swapped, err := BuildCollabOverview(nil, GranDay, now, now.AddDate(0, 0, -3))
	if err != nil || len(swapped.Series) != 4 {
		t.Fatalf("from/to given the wrong way round must still work, got %v %d", err, len(swapped.Series))
	}
	for _, g := range []string{GranDay, GranWeek, GranMonth, GranYear} {
		from, to := DefaultOverviewRange(now, g)
		if _, err := BuildCollabOverview(nil, g, from, to); err != nil {
			t.Fatalf("the default range for %s must be valid: %v", g, err)
		}
	}
}
