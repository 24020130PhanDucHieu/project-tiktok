// Package metrics keeps lightweight in-process request counters for the admin monitoring page.
package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Collector aggregates HTTP request statistics.
type Collector struct {
	Started  time.Time
	total    atomic.Int64
	status2  atomic.Int64
	status4  atomic.Int64
	status5  atomic.Int64
	totalNs  atomic.Int64
	mu       sync.Mutex
	recent   [256]int64 // ring buffer of latencies (ns) for percentiles
	recentN  int
	slowest  atomic.Int64
	inflight atomic.Int64
}

// New creates a Collector.
func New() *Collector { return &Collector{Started: time.Now()} }

// Middleware records every request.
func (c *Collector) Middleware() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		start := time.Now()
		c.inflight.Add(1)
		err := ctx.Next()
		c.inflight.Add(-1)
		d := time.Since(start).Nanoseconds()
		c.total.Add(1)
		c.totalNs.Add(d)
		switch code := ctx.Response().StatusCode(); {
		case code >= 500:
			c.status5.Add(1)
		case code >= 400:
			c.status4.Add(1)
		default:
			c.status2.Add(1)
		}
		if d > c.slowest.Load() {
			c.slowest.Store(d)
		}
		c.mu.Lock()
		c.recent[c.recentN%len(c.recent)] = d
		c.recentN++
		c.mu.Unlock()
		return err
	}
}

// Snapshot is a point-in-time view of the counters.
type Snapshot struct {
	UptimeSeconds int64   `json:"uptimeSeconds"`
	Requests      int64   `json:"requests"`
	Success       int64   `json:"success"`
	ClientErrors  int64   `json:"clientErrors"`
	ServerErrors  int64   `json:"serverErrors"`
	InFlight      int64   `json:"inFlight"`
	AvgMs         float64 `json:"avgMs"`
	P95Ms         float64 `json:"p95Ms"`
	SlowestMs     float64 `json:"slowestMs"`
}

// Snapshot returns the current numbers.
func (c *Collector) Snapshot() Snapshot {
	total := c.total.Load()
	s := Snapshot{
		UptimeSeconds: int64(time.Since(c.Started).Seconds()), Requests: total,
		Success: c.status2.Load(), ClientErrors: c.status4.Load(), ServerErrors: c.status5.Load(),
		InFlight: c.inflight.Load(), SlowestMs: float64(c.slowest.Load()) / 1e6,
	}
	if total > 0 {
		s.AvgMs = float64(c.totalNs.Load()) / float64(total) / 1e6
	}
	c.mu.Lock()
	n := min(c.recentN, len(c.recent))
	vals := make([]int64, n)
	copy(vals, c.recent[:n])
	c.mu.Unlock()
	if n > 0 {
		// insertion sort (n <= 256)
		for i := 1; i < n; i++ {
			for j := i; j > 0 && vals[j-1] > vals[j]; j-- {
				vals[j-1], vals[j] = vals[j], vals[j-1]
			}
		}
		s.P95Ms = float64(vals[(n*95)/100]) / 1e6
	}
	return s
}
