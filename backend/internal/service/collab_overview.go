package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// Overview of all collaborations ("lần hợp tác") for the management tab: totals, a time series and the split by
// outcome and mode. Everything is attributed to the day the collaboration STARTED, because videos carry no date of
// their own; the page says so.

// Granularities of the time series.
const (
	GranDay   = "day"
	GranWeek  = "week"
	GranMonth = "month"
	GranYear  = "year"

	maxOverviewBuckets = 400
)

// OverviewTotals are sums over a set of collaborations.
type OverviewTotals struct {
	Collabs    int     `json:"collabs"`
	Active     int     `json:"active"`
	Closed     int     `json:"closed"`
	Posted     int     `json:"posted"` // closed with a posted video
	Videos     int     `json:"videos"`
	Views      float64 `json:"views"`
	Orders     float64 `json:"orders"`
	Revenue    float64 `json:"revenue"`
	Commission float64 `json:"commission"`
	CastFee    float64 `json:"castFee"`
	Profit     float64 `json:"profit"`
	Owed       float64 `json:"owed"` // cast fee of posted collaborations not yet paid
}

// OverviewBucket is one point of the time series.
type OverviewBucket struct {
	Key   string    `json:"key"`
	Label string    `json:"label"`
	Start time.Time `json:"start"`
	OverviewTotals
}

// CollabOverview is the whole response.
type CollabOverview struct {
	Granularity string           `json:"granularity"`
	From        string           `json:"from"`
	To          string           `json:"to"`
	Totals      OverviewTotals   `json:"totals"`
	Series      []OverviewBucket `json:"series"`
	Outcomes    map[string]int   `json:"outcomes"`
	Modes       map[string]int   `json:"modes"`
}

// ErrBadGranularity is returned for an unknown granularity.
var ErrBadGranularity = errors.New("Chỉ hỗ trợ day, week, month hoặc year")

// ParseOverviewDay reads a YYYY-MM-DD day as Vietnam time.
func ParseOverviewDay(v string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", v, vnZone)
}

func dayStart(t time.Time) time.Time {
	t = t.In(vnZone)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, vnZone)
}

// bucketStart is the first moment of the bucket that contains t.
func bucketStart(t time.Time, gran string) time.Time {
	d := dayStart(t)
	switch gran {
	case GranWeek:
		back := (int(d.Weekday()) + 6) % 7 // Monday = 0
		return d.AddDate(0, 0, -back)
	case GranMonth:
		return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, vnZone)
	case GranYear:
		return time.Date(d.Year(), 1, 1, 0, 0, 0, 0, vnZone)
	}
	return d
}

func nextBucket(start time.Time, gran string) time.Time {
	switch gran {
	case GranWeek:
		return start.AddDate(0, 0, 7)
	case GranMonth:
		return start.AddDate(0, 1, 0)
	case GranYear:
		return start.AddDate(1, 0, 0)
	}
	return start.AddDate(0, 0, 1)
}

func bucketLabel(start time.Time, gran string) (key, label string) {
	switch gran {
	case GranWeek:
		end := start.AddDate(0, 0, 6)
		return start.Format("2006-01-02"), start.Format("02/01") + "–" + end.Format("02/01")
	case GranMonth:
		return start.Format("2006-01"), start.Format("01/2006")
	case GranYear:
		return start.Format("2006"), start.Format("2006")
	}
	return start.Format("2006-01-02"), start.Format("02/01")
}

// DefaultOverviewRange is the window shown when the caller gives none: the last N buckets ending today.
func DefaultOverviewRange(now time.Time, gran string) (from, to time.Time) {
	to = dayStart(now)
	switch gran {
	case GranWeek:
		from = bucketStart(to, GranWeek).AddDate(0, 0, -7*11)
	case GranMonth:
		from = bucketStart(to, GranMonth).AddDate(0, -11, 0)
	case GranYear:
		from = bucketStart(to, GranYear).AddDate(-4, 0, 0)
	default:
		from = to.AddDate(0, 0, -29)
	}
	return from, to
}

func (t *OverviewTotals) add(c *domain.Collab) {
	t.Collabs++
	if c.Status == domain.CollabClosed {
		t.Closed++
		if c.Outcome == domain.OutcomePosted {
			t.Posted++
		}
	} else {
		t.Active++
	}
	t.Videos += c.Perf.Videos
	t.Views += c.Perf.TotalViews
	t.Orders += c.Perf.Orders
	t.Revenue += c.Perf.Revenue
	t.Commission += c.Perf.Commission
	t.CastFee += c.Perf.CastFee
	t.Profit += c.Perf.Profit
	t.Owed += c.Owed()
}

// BuildCollabOverview aggregates collaborations whose start day lies in [from, to] (both inclusive, Vietnam time).
// Each collaboration must already have Perf computed (Recompute).
func BuildCollabOverview(cols []domain.Collab, gran string, from, to time.Time) (*CollabOverview, error) {
	switch gran {
	case GranDay, GranWeek, GranMonth, GranYear:
	default:
		return nil, ErrBadGranularity
	}
	from, to = dayStart(from), dayStart(to)
	if to.Before(from) {
		from, to = to, from
	}

	out := &CollabOverview{Granularity: gran, From: from.Format("2006-01-02"), To: to.Format("2006-01-02"),
		Outcomes: map[string]int{}, Modes: map[string]int{}, Series: []OverviewBucket{}}
	index := map[string]int{}
	for s := bucketStart(from, gran); !s.After(to); s = nextBucket(s, gran) {
		if len(out.Series) >= maxOverviewBuckets {
			return nil, fmt.Errorf("khoảng thời gian quá dài (tối đa %d mốc)", maxOverviewBuckets)
		}
		key, label := bucketLabel(s, gran)
		index[key] = len(out.Series)
		out.Series = append(out.Series, OverviewBucket{Key: key, Label: label, Start: s})
	}

	end := to.AddDate(0, 0, 1)
	for i := range cols {
		c := &cols[i]
		started := c.StartedAt.In(vnZone)
		if started.Before(from) || !started.Before(end) {
			continue
		}
		out.Totals.add(c)
		key, _ := bucketLabel(bucketStart(started, gran), gran)
		if at, ok := index[key]; ok {
			out.Series[at].add(c)
		}
		if c.Status == domain.CollabClosed && c.Outcome != "" {
			out.Outcomes[c.Outcome]++
		}
		mode := c.Mode
		if mode == "" {
			mode = "none"
		}
		out.Modes[mode]++
	}
	return out, nil
}
