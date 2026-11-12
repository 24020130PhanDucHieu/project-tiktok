package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Lists such as the invitations or the open-collaboration products are read every time a screen opens, a filter changes
// or an auto-refresh ticks. TikTok shares one rate limit between all apps of the group, so asking again for something
// that cannot have changed is the surest way to be refused. readCache keeps each answer for a short time, lets
// simultaneous identical requests share one call, returns the last answer instead of an error when TikTok says "too
// many requests", and — when a ReadCacheStore is attached — survives a restart, so the first screen opened after a
// deploy does not fire a burst of calls.
const (
	targetListFresh = 45 * time.Second // served without asking TikTok
	targetListStale = 2 * time.Hour    // still shown when TikTok is refusing (at least as long as the answer was fresh)
	openCollabFresh = 20 * time.Second
	readCacheMax    = 200
)

// ReadCacheStore keeps cached answers outside the process (implemented by the repository package).
type ReadCacheStore interface {
	Load(ctx context.Context, key string) (data []byte, at time.Time, ok bool, err error)
	Save(ctx context.Context, key string, data []byte, at time.Time) error
}

type cacheEntry[V any] struct {
	val V
	at  time.Time
}

// readCache is safe for concurrent use; the zero value is ready (memory only until persist is called).
type readCache[K comparable, V any] struct {
	mu     sync.Mutex
	m      map[K]cacheEntry[V]
	sf     singleflight.Group
	store  ReadCacheStore
	prefix string
	// dropped is when forget last ran: an answer older than that, including the copy in the store, is out of date
	dropped time.Time
}

// persist makes the cache survive restarts; prefix keeps the caches of different reads apart in the store.
func (c *readCache[K, V]) persist(store ReadCacheStore, prefix string) {
	c.mu.Lock()
	c.store, c.prefix = store, prefix
	c.mu.Unlock()
}

func (c *readCache[K, V]) storeKey(k K) string {
	sum := sha256.Sum256([]byte(fmt.Sprint(k)))
	return c.prefix + ":" + hex.EncodeToString(sum[:16])
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

// lookup is get, falling back to the store when the answer is not in memory (after a restart).
func (c *readCache[K, V]) lookup(ctx context.Context, k K) (V, time.Duration, bool) {
	if v, age, ok := c.get(k); ok {
		return v, age, true
	}
	c.mu.Lock()
	store := c.store
	c.mu.Unlock()
	var zero V
	if store == nil {
		return zero, 0, false
	}
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	data, at, ok, err := store.Load(lctx, c.storeKey(k))
	if err != nil || !ok {
		return zero, 0, false
	}
	var v V
	if json.Unmarshal(data, &v) != nil {
		return zero, 0, false
	}
	c.set(k, v, at)
	return v, time.Since(at), true
}

func (c *readCache[K, V]) set(k K, v V, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[K]cacheEntry[V]{}
	}
	if len(c.m) >= readCacheMax { // keys (filters, page tokens) are unbounded: never grow without limit
		for key, e := range c.m {
			if time.Since(e.at) > 24*time.Hour {
				delete(c.m, key)
			}
		}
		if len(c.m) >= readCacheMax {
			c.m = map[K]cacheEntry[V]{}
		}
	}
	c.m[k] = cacheEntry[V]{val: v, at: at}
}

func (c *readCache[K, V]) put(ctx context.Context, k K, v V) {
	now := time.Now()
	c.set(k, v, now)
	c.mu.Lock()
	store := c.store
	c.mu.Unlock()
	if store == nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	_ = store.Save(sctx, c.storeKey(k), data, now) // best effort: the memory copy is already in place
}

// forget drops everything (after a change the cached answers are out of date). The copy kept in the store for restarts
// is not deleted, but it is no longer taken as fresh: otherwise the screen opened right after sending an invitation
// would be given the list from before it. An old answer still stands in when TikTok refuses (see do).
func (c *readCache[K, V]) forget() {
	c.mu.Lock()
	c.m = nil
	c.dropped = time.Now()
	c.mu.Unlock()
}

// fresh reports whether an answer of this age may be served without asking TikTok.
func (c *readCache[K, V]) fresh(age, limit time.Duration) bool {
	c.mu.Lock()
	dropped := c.dropped
	c.mu.Unlock()
	if !dropped.IsZero() && age >= time.Since(dropped) {
		return false // made before the last change
	}
	return age < limit
}

// do returns the cached answer while it is younger than fresh, otherwise loads it (one load for all simultaneous
// callers). When the load is refused with a rate limit, the last answer is returned instead of the error.
func (c *readCache[K, V]) do(ctx context.Context, k K, fresh time.Duration, load func(context.Context) (V, error)) (V, error) {
	if v, age, ok := c.lookup(ctx, k); ok && c.fresh(age, fresh) {
		return v, nil
	}
	res, err, _ := c.sf.Do(fmt.Sprint(k), func() (any, error) {
		v, err := load(context.WithoutCancel(ctx))
		if err == nil {
			c.put(ctx, k, v)
		}
		return v, err
	})
	if err == nil {
		return res.(V), nil
	}
	if IsRateLimited(err) {
		stale := targetListStale
		if fresh > stale {
			stale = fresh
		}
		if v, age, ok := c.lookup(ctx, k); ok && age < stale {
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
	fresh := targetListFresh
	if p.Fresh { // an explicit refresh: ask TikTok now (still falls back to the last list when TikTok refuses)
		fresh = 0
	}
	p.Fresh = false // the cache key is the filter, not how it was asked
	return s.targets.do(ctx, p, fresh, func(ctx context.Context) (*TargetPage, error) {
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

// WithReadCache keeps the cached TikTok reads in a store so they survive a restart.
func (s *TTShopService) WithReadCache(store ReadCacheStore) *TTShopService {
	s.targets.persist(store, "targets")
	s.openCollabs.persist(store, "opencollabs")
	s.creators.persist(store, "creators")
	s.traffic.persist(store, "traffic")
	s.images.persist(store, "productimg")
	return s
}
