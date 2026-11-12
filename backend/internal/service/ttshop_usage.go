package service

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// How many calls the app sends to TikTok Shop, hour by hour, read from the call log (which now survives a deploy).
// TikTok's rate limit is shared by the whole app group, so this tells how much of it is ours.

// UsageKind is the number of calls of one kind (method and path) in the period.
type UsageKind struct {
	Kind     string `json:"kind"`
	Calls    int    `json:"calls"`
	Rejected int    `json:"rejected"`
}

// UsageHour is the calls of one hour (UTC, "2026-11-12T17").
type UsageHour struct {
	Hour     string `json:"hour"`
	Calls    int    `json:"calls"`
	Rejected int    `json:"rejected"`
}

// UsageReport is what the call log says about the last hours.
type UsageReport struct {
	From     string      `json:"from"` // first call counted (UTC)
	To       string      `json:"to"`   // last call counted (UTC)
	Calls    int         `json:"calls"`
	Rejected int         `json:"rejected"` // refused with "too many requests"
	Hours    []UsageHour `json:"hours"`
	Kinds    []UsageKind `json:"kinds"` // most used first
	Complete bool        `json:"complete"`
}

var (
	usageLine = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}):\d{2}:\d{2}Z (GET|POST|PUT|DELETE) (\S+).* http=(\d+) code=(-?\d+)`)
	usageID   = regexp.MustCompile(`/\d{6,}`)
)

// usageKindOf turns "/affiliate_seller/202410/orders/search" into "POST /affiliate_seller/{id}/orders/search": the same
// call for any version or id counts as one kind.
func usageKindOf(method, path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return method + " " + usageID.ReplaceAllString(path, "/{id}")
}

// parseUsage counts the log lines (any order) made since the given time.
func parseUsage(lines []string, since time.Time) UsageReport {
	rep := UsageReport{Hours: []UsageHour{}, Kinds: []UsageKind{}}
	hours := map[string]*UsageHour{}
	kinds := map[string]*UsageKind{}
	var first, last string
	for _, l := range lines {
		m := usageLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		stamp := strings.SplitN(l, " ", 2)[0]
		at, err := time.Parse(time.RFC3339, stamp)
		if err != nil || at.Before(since) {
			continue
		}
		rejected := m[4] == "429" || m[5] == "36009002"
		h := hours[m[1]]
		if h == nil {
			h = &UsageHour{Hour: m[1]}
			hours[m[1]] = h
		}
		k := kinds[usageKindOf(m[2], m[3])]
		if k == nil {
			k = &UsageKind{Kind: usageKindOf(m[2], m[3])}
			kinds[k.Kind] = k
		}
		h.Calls++
		k.Calls++
		rep.Calls++
		if rejected {
			h.Rejected++
			k.Rejected++
			rep.Rejected++
		}
		if first == "" || stamp < first {
			first = stamp
		}
		if stamp > last {
			last = stamp
		}
	}
	rep.From, rep.To = first, last
	for _, h := range hours {
		rep.Hours = append(rep.Hours, *h)
	}
	sort.Slice(rep.Hours, func(i, j int) bool { return rep.Hours[i].Hour < rep.Hours[j].Hour })
	for _, k := range kinds {
		rep.Kinds = append(rep.Kinds, *k)
	}
	sort.Slice(rep.Kinds, func(i, j int) bool {
		if rep.Kinds[i].Calls != rep.Kinds[j].Calls {
			return rep.Kinds[i].Calls > rep.Kinds[j].Calls
		}
		return rep.Kinds[i].Kind < rep.Kinds[j].Kind
	})
	return rep
}

// readLogLines reads a log file, skipping a missing one. Lines are cut by the scanner at 1 MiB.
func readLogLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

// Usage counts the calls made to TikTok Shop in the last hours (1 to 168) from the log file and its previous copy.
// Complete is false when the log does not reach back that far (it was started later, or rotated).
func (s *TTShopService) Usage(hours int) UsageReport {
	if hours < 1 {
		hours = 24
	}
	if hours > 168 {
		hours = 168
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	var lines []string
	if p := s.logs.path(); p != "" {
		lines = append(readLogLines(p+".1"), readLogLines(p)...)
	} else {
		lines = s.logs.recent(0) // no file: only what this process remembers
	}
	rep := parseUsage(lines, since)
	if rep.From != "" {
		if first, err := time.Parse(time.RFC3339, rep.From); err == nil {
			// the log is complete for the period when its oldest line is not much newer than the period's start
			rep.Complete = first.Sub(since) < time.Hour
		}
	}
	return rep
}
