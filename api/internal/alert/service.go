package alert

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/ayush00git/aahat/api/internal/data"
)

// SubscriptionFinder and EventSaver are the parts of the store the alert flow
// needs.
type SubscriptionFinder interface {
	SubscriptionsForPlaces(ctx context.Context, osms []string) ([]Subscription, error)
}

type EventSaver interface {
	PutEvent(ctx context.Context, ev Event) error
}

// Service runs triggers: plan, log, dispatch, log again.
type Service struct {
	Catalog    *data.Catalog
	Subs       SubscriptionFinder
	Events     EventSaver
	Dispatcher *Dispatcher
	Now        func() time.Time
	NewID      func(prefix string) string
}

func NewService(c *data.Catalog, subs SubscriptionFinder, events EventSaver, d *Dispatcher) *Service {
	return &Service{Catalog: c, Subs: subs, Events: events, Dispatcher: d, Now: time.Now, NewID: NewID}
}

type TriggerRequest struct {
	LakeID   string
	Scenario string
	Source   string
	Note     string
	DryRun   bool // build, log and voice the plan, but send nothing (drills)
	Sensor   *SensorReading
}

// Trigger builds the warning plan for a lake outburst, stores it, sends it and
// stores the delivery results. A data.ErrNotFound means the lake is unknown.
func (s *Service) Trigger(ctx context.Context, req TriggerRequest) (*Event, error) {
	if req.Scenario == "" {
		req.Scenario = data.ScenarioSevere
	}
	if req.Scenario != data.ScenarioExpected && req.Scenario != data.ScenarioSevere {
		return nil, invalid(`scenario must be "expected" or "severe"`)
	}
	if req.Source == "" {
		req.Source = SourceSimulation
	}
	switch req.Source {
	case SourceSimulation, SourceSensor:
	case SourceDrainCheck:
		// Raised by a machine with nobody watching: never sends on its own.
		if !req.DryRun {
			return nil, invalid(`source "satellite_drain_check" requires "dry_run": true`)
		}
	default:
		return nil, invalid(`source must be "simulation", "sensor" or "satellite_drain_check"`)
	}
	if utf8.RuneCountInString(req.Note) > MaxNoteLen {
		return nil, invalid("note must be at most %d characters", MaxNoteLen)
	}
	if req.LakeID == "" {
		return nil, invalid("lake_id is required")
	}

	lake, err := s.Catalog.Lake(ctx, req.LakeID)
	if err != nil {
		return nil, err
	}
	impacts, err := s.Catalog.Impacts(ctx, req.LakeID)
	if err != nil {
		return nil, fmt.Errorf("impacts for %s: %w", req.LakeID, err)
	}
	affected := AffectedPlaces(impacts, req.Scenario)
	osms := make([]string, len(affected))
	for i, im := range affected {
		osms[i] = im.OSM
	}
	subs, err := s.Subs.SubscriptionsForPlaces(ctx, osms)
	if err != nil {
		return nil, fmt.Errorf("subscriptions: %w", err)
	}
	recipients, places, err := BuildPlan(*lake, req.Scenario, affected, subs)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}

	ev := &Event{
		ID: s.NewID("evt"), LakeID: lake.ID, LakeName: lake.Name, LakeNameHi: lake.NameHi,
		Scenario: req.Scenario, Source: req.Source, Note: req.Note, Sensor: req.Sensor, DryRun: req.DryRun,
		CreatedAt: s.Now().UTC(), Recipients: recipients, AffectedPlaces: places,
		Summary: summarize(recipients),
	}
	// Log the plan before sending, so a crash mid-dispatch still leaves a record.
	if err := s.Events.PutEvent(ctx, *ev); err != nil {
		return nil, fmt.Errorf("save event: %w", err)
	}
	s.Dispatcher.Dispatch(ctx, ev)
	if err := s.Events.PutEvent(ctx, *ev); err != nil {
		return ev, fmt.Errorf("save delivery results: %w", err)
	}
	return ev, nil
}

// NewID returns a random id such as "sub_3f9a0c1d2e4b5a69".
func NewID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // never fails (crypto/rand panics instead)
	return prefix + "_" + hex.EncodeToString(b)
}
