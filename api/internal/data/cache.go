package data

import (
	"context"
	"sync"
	"time"
)

// Cache keeps recently read files in memory for a fixed TTL. The data only
// changes when the pipeline republishes, so a short TTL saves most S3 reads
// while still picking up new runs within minutes.
type Cache struct {
	next  DataStore
	ttl   time.Duration
	now   func() time.Time
	mu    sync.Mutex
	items map[string]cacheItem
}

type cacheItem struct {
	body    []byte
	expires time.Time
}

// NewCache wraps next. A ttl of zero or less disables caching.
func NewCache(next DataStore, ttl time.Duration) *Cache {
	return &Cache{next: next, ttl: ttl, now: time.Now, items: map[string]cacheItem{}}
}

func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	if c.ttl <= 0 {
		return c.next.Get(ctx, key)
	}
	now := c.now()
	c.mu.Lock()
	it, ok := c.items[key]
	c.mu.Unlock()
	if ok && now.Before(it.expires) {
		return it.body, nil
	}

	body, err := c.next.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.items[key] = cacheItem{body: body, expires: now.Add(c.ttl)}
	c.mu.Unlock()
	return body, nil
}
