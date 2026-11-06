package service

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// The list of invitations is read every time someone opens the management table or changes a filter. TikTok shares one
// rate limit between all apps of the group, so asking again for something that cannot have changed is the surest way to
// be refused. This cache keeps each list for a short time, lets simultaneous requests share one call, and — when TikTok
// does say "too many requests" — shows the last list we had instead of an error.
const (
	targetListFresh = 45 * time.Second // served without asking TikTok
	targetListStale = 2 * time.Hour    // still shown when TikTok is refusing
)

type targetListEntry struct {
	page *TargetPage
	at   time.Time
}

type targetListCache struct {
	mu sync.Mutex
	m  map[TargetListParams]targetListEntry
	sf singleflight.Group
}

func (c *targetListCache) get(p TargetListParams, now time.Time) (*TargetPage, time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[p]
	if !ok {
		return nil, 0, false
	}
	return e.page, now.Sub(e.at), true
}

func (c *targetListCache) put(p TargetListParams, page *TargetPage, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[TargetListParams]targetListEntry{}
	}
	if len(c.m) > 200 { // filters and page tokens are unbounded: never grow without limit
		for k, e := range c.m {
			if now.Sub(e.at) > targetListStale {
				delete(c.m, k)
			}
		}
		if len(c.m) > 200 {
			c.m = map[TargetListParams]targetListEntry{}
		}
	}
	c.m[p] = targetListEntry{page: page, at: now}
}

// forget drops every cached list (after an invitation was created or cancelled the lists are out of date).
func (c *targetListCache) forget() {
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}

// SearchTargets lists the shop's invitations (read-only), through the cache described above.
func (s *TTShopService) SearchTargets(ctx context.Context, p TargetListParams) (*TargetPage, error) {
	if _, err := buildTargetSearchBody(p); err != nil { // refuse bad filters before they become cache keys
		return nil, err
	}
	now := time.Now()
	if page, age, ok := s.targets.get(p, now); ok && age < targetListFresh {
		return page, nil
	}
	v, err, _ := s.targets.sf.Do(targetKey(p), func() (any, error) {
		page, err := s.searchTargetsLive(context.WithoutCancel(ctx), p)
		if err == nil {
			s.targets.put(p, page, time.Now())
		}
		return page, err
	})
	if err == nil {
		return v.(*TargetPage), nil
	}
	if IsRateLimited(err) {
		if page, age, ok := s.targets.get(p, time.Now()); ok && age < targetListStale {
			return page, nil
		}
	}
	return nil, err
}

func targetKey(p TargetListParams) string {
	return p.Status + "|" + p.AcceptStatus + "|" + p.SampleSetting + "|" + p.Keyword + "|" + p.KeywordType + "|" + p.PageToken
}
