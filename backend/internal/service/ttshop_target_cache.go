package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Lists such as the invitations or the open-collaboration products are read every time a screen opens, a filter changes
// or an auto-refresh ticks. TikTok shares one rate limit between all apps of the group, so asking again for something
// that cannot have changed is the surest way to be refused. readCache keeps each answer for a short time, lets
// simultaneous identical requests share one call, and — when TikTok does say "too many requests" — returns the last
// answer we had instead of an error.
const (
	targetListFresh = 45 * time.Second // served without asking TikTok
	targetListStale = 2 * time.Hour    // still shown when TikTok is refusing
	openCollabFresh = 20 * time.Second
	readCacheMax    = 200
)

type cacheEntry[V any] struct {
	val V
	at  time.Time
}

// readCache is safe for concurrent use; the zero value is ready.
type readCache[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]cacheEntry[V]
	sf singleflight.Group
}

func (c *readCache[K, V]) get(k K) (V, time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[k]
	if !ok {
		var zero V
		return zero, 0, false
	}
	return e.val, time.Since(e.at), true
}

func (c *readCache[K, V]) put(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[K]cacheEntry[V]{}
	}
	if len(c.m) >= readCacheMax { // keys (filters, page tokens) are unbounded: never grow without limit
		for key, e := range c.m {
			if time.Since(e.at) > targetListStale {
				delete(c.m, key)
			}
		}
		if len(c.m) >= readCacheMax {
			c.m = map[K]cacheEntry[V]{}
		}
	}
	c.m[k] = cacheEntry[V]{val: v, at: time.Now()}
}

// forget drops everything (after a change the cached answers are out of date).
func (c *readCache[K, V]) forget() {
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}

// do returns the cached answer while it is fresh, otherwise loads it (one load for all simultaneous callers). When the
// load is refused with a rate limit, an answer younger than targetListStale is returned instead of the error.
func (c *readCache[K, V]) do(ctx context.Context, k K, fresh time.Duration, load func(context.Context) (V, error)) (V, error) {
	if v, age, ok := c.get(k); ok && age < fresh {
		return v, nil
	}
	res, err, _ := c.sf.Do(fmt.Sprint(k), func() (any, error) {
		v, err := load(context.WithoutCancel(ctx))
		if err == nil {
			c.put(k, v)
		}
		return v, err
	})
	if err == nil {
		return res.(V), nil
	}
	if IsRateLimited(err) {
		if v, age, ok := c.get(k); ok && age < targetListStale {
			return v, nil
		}
	}
	var zero V
	return zero, err
}

// SearchTargets lists the shop's invitations (read-only), through the cache described above.
func (s *TTShopService) SearchTargets(ctx context.Context, p TargetListParams) (*TargetPage, error) {
	if _, err := buildTargetSearchBody(p); err != nil { // refuse bad filters before they become cache keys
		return nil, err
	}
	return s.targets.do(ctx, p, targetListFresh, func(ctx context.Context) (*TargetPage, error) {
		return s.searchTargetsLive(ctx, p)
	})
}

type openCollabKey struct {
	keyword, pageToken string
	size               int
}

// OpenCollaborations lists the products the shop opened for open collaboration (read-only), through the cache.
func (s *TTShopService) OpenCollaborations(ctx context.Context, keyword, pageToken string, pageSize int) (*OpenCollabPage, error) {
	k := openCollabKey{keyword: keyword, pageToken: pageToken, size: pageSize}
	return s.openCollabs.do(ctx, k, openCollabFresh, func(ctx context.Context) (*OpenCollabPage, error) {
		return s.openCollaborationsLive(ctx, keyword, pageToken, pageSize)
	})
}
