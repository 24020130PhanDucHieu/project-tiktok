package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// TikTok Shop limits how fast an app may call it (error 36009002, HTTP 429, "shared app-group rate limit"). The pacer
// keeps this process polite: a minimum gap between two calls, a ceiling per hour (sliding window), and a cool-down
// after TikTok says "too many requests" during which nothing is sent at all instead of retrying into the wall.
const (
	ttsMinGap         = 250 * time.Millisecond
	ttsMaxPerHour     = 4000
	ttsRateLimitCode  = 36009002
	ttsRateLimitPause = 45 * time.Second
)

// ErrRateLimited means calls to TikTok Shop are paused because it asked us to slow down (or our own hourly ceiling was
// reached). Nothing was sent, so it is always safe to try again after RetryAfter.
type ErrRateLimited struct{ RetryAfter time.Duration }

func (e *ErrRateLimited) Error() string {
	secs := int(e.RetryAfter.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return fmt.Sprintf("TikTok Shop đang giới hạn tốc độ gọi, thử lại sau khoảng %d giây", secs)
}

// IsRateLimited reports whether err means "slow down": our own pause, or TikTok's 36009002.
func IsRateLimited(err error) bool {
	var rl *ErrRateLimited
	if errors.As(err, &rl) {
		return true
	}
	var rej *TTSRejected
	return errors.As(err, &rej) && rej.Code == ttsRateLimitCode
}

// pacer is safe for concurrent use; the zero value works with the defaults above.
type pacer struct {
	mu       sync.Mutex
	next     time.Time   // earliest start of the next call
	sent     []time.Time // start times within the last hour (sliding window)
	pausedTo time.Time   // no calls before this moment
	now      func() time.Time
	gap      time.Duration
	perHour  int
}

func (p *pacer) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

func (p *pacer) limits() (time.Duration, int) {
	gap, perHour := p.gap, p.perHour
	if gap == 0 {
		gap = ttsMinGap
	}
	if perHour == 0 {
		perHour = ttsMaxPerHour
	}
	return gap, perHour
}

// reserve books the next slot. It returns how long the caller must wait before sending, or an ErrRateLimited when
// calls are paused or the hourly ceiling is used up.
func (p *pacer) reserve() (time.Duration, error) {
	gap, perHour := p.limits()
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.clock()
	if now.Before(p.pausedTo) {
		return 0, &ErrRateLimited{RetryAfter: p.pausedTo.Sub(now)}
	}
	cut := now.Add(-time.Hour)
	i := 0
	for i < len(p.sent) && !p.sent[i].After(cut) {
		i++
	}
	p.sent = p.sent[i:]
	if len(p.sent) >= perHour {
		return 0, &ErrRateLimited{RetryAfter: p.sent[0].Add(time.Hour).Sub(now)}
	}
	start := now
	if p.next.After(start) {
		start = p.next
	}
	p.next = start.Add(gap)
	p.sent = append(p.sent, start)
	return start.Sub(now), nil
}

// penalize pauses every call for d (called when TikTok answers 36009002).
func (p *pacer) penalize(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if until := p.clock().Add(d); until.After(p.pausedTo) {
		p.pausedTo = until
	}
}

// wait blocks until the caller may send, or returns early with the context's error / an ErrRateLimited.
func (p *pacer) wait(ctx context.Context) error {
	d, err := p.reserve()
	if err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
