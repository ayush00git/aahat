package weather

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// DefaultTTL is how long an outlook is served before it is fetched again.
	DefaultTTL = 3 * time.Hour
	// retryAfter spaces out fetches for a lake after a failure, so an Open-Meteo outage does
	// not make every request wait for a timeout.
	retryAfter = time.Minute
)

// ErrUnavailable: the fetch failed and there is no earlier outlook to fall back on.
var ErrUnavailable = errors.New("weather unavailable")

// Report is the outlook and trigger for one lake.
type Report struct {
	LakeID    string    `json:"lake_id"`
	FetchedAt time.Time `json:"fetched_at"`
	// Stale is true when the outlook is older than the cache TTL because fetching a new one failed.
	Stale   bool    `json:"stale"`
	Source  string  `json:"source"`
	Days    []Day   `json:"days"`
	Trigger Trigger `json:"trigger"`
}

// Service caches outlooks in memory, per lake.
type Service struct {
	Fetcher Fetcher
	TTL     time.Duration
	Now     func() time.Time

	mu    sync.Mutex
	items map[string]item
}

type item struct {
	report  Report
	ok      bool      // report holds a fetched outlook
	retryAt time.Time // no fetch before this (set after a failure)
}

func NewService(f Fetcher, ttl time.Duration) *Service {
	return &Service{Fetcher: f, TTL: ttl, Now: time.Now, items: map[string]item{}}
}

// Lake returns the outlook for a lake: from the cache while it is fresh, else fetched. If the
// fetch fails, the last outlook is returned marked stale, or ErrUnavailable if there is none.
func (s *Service) Lake(ctx context.Context, id string, lat, lon float64) (Report, error) {
	now := s.Now()
	s.mu.Lock()
	it := s.items[id]
	s.mu.Unlock()
	if it.ok && now.Sub(it.report.FetchedAt) < s.TTL {
		return it.report, nil
	}

	err := errors.New("recent fetch failed")
	if !now.Before(it.retryAt) {
		var days []Day
		if days, err = s.Fetcher.Fetch(ctx, lat, lon); err == nil {
			r := Report{LakeID: id, FetchedAt: now.UTC(), Source: Source, Days: days, Trigger: Evaluate(days)}
			s.mu.Lock()
			s.items[id] = item{report: r, ok: true}
			s.mu.Unlock()
			return r, nil
		}
		s.mu.Lock()
		it = s.items[id] // another request may have fetched meanwhile
		it.retryAt = now.Add(retryAfter)
		s.items[id] = it
		s.mu.Unlock()
	}
	if it.ok {
		r := it.report
		r.Stale = true
		return r, nil
	}
	return Report{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
}
