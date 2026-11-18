package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// TikTok Shop limits how fast an app may call it (error 36009002, HTTP 429, "shared app-group rate limit"). The pacer
// keeps this process polite: a minimum gap between two calls, a ceiling per hour (sliding window), and a cool-down
// after TikTok says "too many requests" during which nothing is sent at all instead of retrying into the wall.
const (
	ttsMinGap         = 5 * time.Second // the gap before the next call is drawn between ttsMinGap and ttsMaxGap
	ttsMaxGap         = 10 * time.Second
	ttsMaxPerHour     = 4000
	ttsRateLimitCode  = 36009002
	ttsRateLimitPause = 45 * time.Second
	ttsRetryDelay     = 2 * time.Second // wait before the single retry of a read that was rate limited
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
	gapMax   time.Duration // when above gap, each gap is drawn at random between gap and gapMax
	perHour  int
}

func (p *pacer) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

func (p *pacer) limits() (time.Duration, int) {
	gap, gapMax, perHour := p.gap, p.gapMax, p.perHour
	if gap == 0 {
		gap, gapMax = ttsMinGap, ttsMaxGap
	}
	if gapMax > gap {
		gap += time.Duration(rand.Int63n(int64(gapMax-gap) + 1))
	}
	if perHour == 0 {
		perHour = ttsMaxPerHour
	}
	return gap, perHour
}

// openLocked says whether a call may be made at all right now: not while calls are paused after a refusal, and not
// once the hourly ceiling is used up (it also forgets the calls that left the hour). The caller holds p.mu.
func (p *pacer) openLocked(now time.Time, perHour int) error {
	if now.Before(p.pausedTo) {
		return &ErrRateLimited{RetryAfter: p.pausedTo.Sub(now)}
	}
	cut := now.Add(-time.Hour)
	i := 0
	for i < len(p.sent) && !p.sent[i].After(cut) {
		i++
	}
	p.sent = p.sent[i:]
	if len(p.sent) >= perHour {
		return &ErrRateLimited{RetryAfter: p.sent[0].Add(time.Hour).Sub(now)}
	}
	return nil
}

// reserve books the next slot, however far ahead it is: this is the call of a person waiting for an answer. It returns
// how long the caller must wait before sending, or an ErrRateLimited when calls are paused or the hourly ceiling is used up.
func (p *pacer) reserve() (time.Duration, error) {
	gap, perHour := p.limits()
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.clock()
	if err := p.openLocked(now, perHour); err != nil {
		return 0, err
	}
	start := now
	if p.next.After(start) {
		start = p.next
	}
	p.next = start.Add(gap)
	p.sent = append(p.sent, start)
	return start.Sub(now), nil
}

// reserveBackground is reserve for work nobody is waiting for (an import running in the background). It books the slot
// only when it is free right now; otherwise it books nothing and says how long to wait before asking again. A booking
// far ahead would make a person who asks in the meantime queue behind it: with nothing booked, that person takes the next
// slot first and the background work is asked again after it.
func (p *pacer) reserveBackground() (wait time.Duration, booked bool, err error) {
	gap, perHour := p.limits()
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.clock()
	if err := p.openLocked(now, perHour); err != nil {
		return 0, false, err
	}
	if p.next.After(now) {
		return p.next.Sub(now), false, nil
	}
	p.next = now.Add(gap)
	p.sent = append(p.sent, now)
	return 0, true, nil
}

// penalize pauses every call for d (called when TikTok answers 36009002).
func (p *pacer) penalize(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if until := p.clock().Add(d); until.After(p.pausedTo) {
		p.pausedTo = until
	}
}

type priorityKey struct{}

// WithBackgroundPriority marks the calls made with ctx as background work: they give way to the calls of a person who is
// waiting (see reserveBackground). The marker follows the context into the cache and the search of creators.
func WithBackgroundPriority(ctx context.Context) context.Context {
	return context.WithValue(ctx, priorityKey{}, true)
}

func isBackground(ctx context.Context) bool {
	v, _ := ctx.Value(priorityKey{}).(bool)
	return v
}

// sleepOrDone waits for d, or returns the context's error when it ends first.
func sleepOrDone(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// wait blocks until the caller may send, or returns early with the context's error / an ErrRateLimited. A call marked
// as background work waits for a free slot instead of booking one ahead.
func (p *pacer) wait(ctx context.Context) error {
	if isBackground(ctx) {
		for {
			d, booked, err := p.reserveBackground()
			if err != nil {
				return err
			}
			if booked {
				return nil
			}
			if err := sleepOrDone(ctx, d); err != nil {
				return err
			}
		}
	}
	d, err := p.reserve()
	if err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	return sleepOrDone(ctx, d)
}
