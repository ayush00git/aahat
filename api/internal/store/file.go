package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ayush00git/aahat/api/internal/alert"
)

// FileStore keeps everything in memory and, if it has a path, rewrites a
// JSON file after each change. Meant for local development and tests.
type FileStore struct {
	path   string
	mu     sync.RWMutex
	subs   map[string]alert.Subscription
	events map[string]alert.Event
}

type fileContents struct {
	Subscriptions []alert.Subscription `json:"subscriptions"`
	Events        []alert.Event        `json:"events"`
}

// NewMemoryStore returns a FileStore that never touches disk.
func NewMemoryStore() *FileStore {
	return &FileStore{subs: map[string]alert.Subscription{}, events: map[string]alert.Event{}}
}

// NewFileStore loads path if it exists and saves back to it on every change.
func NewFileStore(path string) (*FileStore, error) {
	s := NewMemoryStore()
	s.path = path
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var fc fileContents
	if err := json.Unmarshal(b, &fc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, sub := range fc.Subscriptions {
		s.subs[sub.ID] = sub
	}
	for _, ev := range fc.Events {
		s.events[ev.ID] = ev
	}
	return s, nil
}

func (s *FileStore) CreateSubscription(_ context.Context, sub alert.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs[sub.ID] = sub
	return s.save()
}

func (s *FileStore) DeleteSubscription(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[id]; !ok {
		return ErrNotFound
	}
	delete(s.subs, id)
	return s.save()
}

func (s *FileStore) ListSubscriptions(_ context.Context, placeOSM string) ([]alert.Subscription, error) {
	return s.filterSubs(func(sub alert.Subscription) bool {
		return placeOSM == "" || sub.PlaceOSM == placeOSM
	}), nil
}

func (s *FileStore) SubscriptionsForPlaces(_ context.Context, osms []string) ([]alert.Subscription, error) {
	return s.filterSubs(func(sub alert.Subscription) bool {
		return slices.Contains(osms, sub.PlaceOSM)
	}), nil
}

func (s *FileStore) filterSubs(keep func(alert.Subscription) bool) []alert.Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []alert.Subscription{}
	for _, sub := range s.subs {
		if keep(sub) {
			out = append(out, sub)
		}
	}
	sortSubs(out)
	return out
}

func (s *FileStore) PutEvent(_ context.Context, ev alert.Event) error {
	// Copy the slices: the caller keeps mutating its event while dispatching.
	ev.Recipients = slices.Clone(ev.Recipients)
	ev.AffectedPlaces = slices.Clone(ev.AffectedPlaces)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[ev.ID] = ev
	return s.save()
}

func (s *FileStore) GetEvent(_ context.Context, id string) (alert.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev, ok := s.events[id]
	if !ok {
		return alert.Event{}, ErrNotFound
	}
	return ev, nil
}

func (s *FileStore) ListEvents(_ context.Context, lakeID string) ([]alert.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []alert.Event{}
	for _, ev := range s.events {
		if lakeID == "" || ev.LakeID == lakeID {
			out = append(out, ev)
		}
	}
	sortEvents(out)
	return out, nil
}

// save writes the whole store atomically. Callers hold the write lock.
func (s *FileStore) save() error {
	if s.path == "" {
		return nil
	}
	fc := fileContents{Subscriptions: []alert.Subscription{}, Events: []alert.Event{}}
	for _, sub := range s.subs {
		fc.Subscriptions = append(fc.Subscriptions, sub)
	}
	for _, ev := range s.events {
		fc.Events = append(fc.Events, ev)
	}
	sortSubs(fc.Subscriptions)
	sortEvents(fc.Events)
	b, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func sortSubs(subs []alert.Subscription) {
	slices.SortFunc(subs, func(a, b alert.Subscription) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func sortEvents(evs []alert.Event) {
	slices.SortFunc(evs, func(a, b alert.Event) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}
