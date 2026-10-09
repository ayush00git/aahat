package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ayush00git/aahat/api/internal/alert"
)

func TestFileStorePersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.json")
	s, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	s.CreateSubscription(ctx, alert.Subscription{ID: "sub_a", PlaceOSM: "node/1", CreatedAt: t0})
	s.CreateSubscription(ctx, alert.Subscription{ID: "sub_b", PlaceOSM: "node/2", CreatedAt: t0.Add(time.Second)})
	s.PutEvent(ctx, alert.Event{ID: "evt_old", LakeID: "x", CreatedAt: t0})
	s.PutEvent(ctx, alert.Event{ID: "evt_new", LakeID: "x", CreatedAt: t0.Add(time.Hour)})

	re, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	subs, _ := re.SubscriptionsForPlaces(ctx, []string{"node/2", "node/3"})
	if len(subs) != 1 || subs[0].ID != "sub_b" {
		t.Errorf("subscriptions for places = %+v", subs)
	}
	evs, _ := re.ListEvents(ctx, "x")
	if len(evs) != 2 || evs[0].ID != "evt_new" {
		t.Errorf("events = %+v (want newest first)", evs)
	}
	if err := re.DeleteSubscription(ctx, "sub_zzz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing = %v", err)
	}
	if _, err := re.GetEvent(ctx, "evt_zzz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("get missing = %v", err)
	}
}
