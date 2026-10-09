// Package store keeps subscriptions and the alert log.
package store

import (
	"context"
	"errors"

	"github.com/ayush00git/aahat/api/internal/alert"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	CreateSubscription(ctx context.Context, s alert.Subscription) error
	DeleteSubscription(ctx context.Context, id string) error
	// ListSubscriptions returns subscriptions for one place, or all of them
	// when placeOSM is empty, oldest first.
	ListSubscriptions(ctx context.Context, placeOSM string) ([]alert.Subscription, error)
	SubscriptionsForPlaces(ctx context.Context, osms []string) ([]alert.Subscription, error)

	// PutEvent creates or replaces an event.
	PutEvent(ctx context.Context, ev alert.Event) error
	GetEvent(ctx context.Context, id string) (alert.Event, error)
	// ListEvents returns events for one lake, or all when lakeID is empty,
	// newest first.
	ListEvents(ctx context.Context, lakeID string) ([]alert.Event, error)
}
