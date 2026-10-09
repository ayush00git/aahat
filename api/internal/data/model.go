package data

import (
	"cmp"
	"encoding/json"
	"slices"
)

// Lake is the part of an index entry the API interprets. Raw keeps the whole
// entry so it can be passed through unchanged.
type Lake struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	NameHi string          `json:"name_hi"`
	Risk   *LakeRisk       `json:"risk"`
	Raw    json.RawMessage `json:"-"`
}

type LakeRisk struct {
	Score *float64 `json:"score"`
	Level string   `json:"level"`
}

// Index is lakes/index.json.
type Index struct {
	GeneratedAt string
	Lakes       []Lake
}

// Impact statuses (overall, worst case over both scenarios).
const (
	StatusInFloodPath = "in_flood_path"
	StatusAtRisk      = "at_risk"
	StatusOutside     = "outside"
)

// Per-scenario statuses.
const (
	OutcomeFlooded = "flooded"
	OutcomeMargin  = "margin"
	OutcomeOutside = "outside"
)

// Flood scenarios, as named in downstream.json and impacts.json.
const (
	ScenarioExpected = "expected"
	ScenarioSevere   = "severe"
)

// Impact kinds.
const (
	KindSettlement = "settlement"
	KindBridge     = "bridge"
	KindRoad       = "road"
	KindHydro      = "hydro"
	KindSchool     = "school"
	KindHealth     = "health"
)

// Impact is one row of lakes/<id>/impacts.json. Numbers are pointers because
// the pipeline writes null where it has no value, and the API must not turn a
// missing value into a zero.
type Impact struct {
	Name               *string    `json:"name"`
	NameHi             *string    `json:"name_hi"`
	Kind               string     `json:"kind"`
	Subkind            string     `json:"subkind"`
	OSM                string     `json:"osm"`
	Lon                float64    `json:"lon"`
	Lat                float64    `json:"lat"`
	Km                 *float64   `json:"km"`
	LateralM           *float64   `json:"lateral_m"`
	ArrivalMinExpected *float64   `json:"arrival_min_expected"`
	ArrivalMinFast     *float64   `json:"arrival_min_fast"`
	Status             string     `json:"status"`
	Scenarios          *Scenarios `json:"scenarios"`

	Raw json.RawMessage `json:"-"`
}

type Scenarios struct {
	Expected *Outcome `json:"expected"`
	Severe   *Outcome `json:"severe"`
}

// Outcome is an asset's result under one flood scenario.
type Outcome struct {
	Status            string   `json:"status"`
	HeightAboveFloodM *float64 `json:"height_above_flood_m"`
	FloodDepthM       *float64 `json:"flood_depth_m"`
	RoadKmFlooded     *float64 `json:"road_km_flooded"`
}

// Outcome returns the result under the named scenario, or nil.
func (i Impact) Outcome(scenario string) *Outcome {
	if i.Scenarios == nil {
		return nil
	}
	switch scenario {
	case ScenarioExpected:
		return i.Scenarios.Expected
	case ScenarioSevere:
		return i.Scenarios.Severe
	}
	return nil
}

// AffectedUnder reports whether the asset is flooded or within the margin
// under the given scenario. The severe flood is the larger one, so anything
// hit by the expected flood counts as hit by the severe one too.
func (i Impact) AffectedUnder(scenario string) bool {
	if i.Status == StatusOutside {
		return false
	}
	if i.Scenarios == nil {
		// No per-scenario detail: fall back to the overall status, which
		// errs on the side of warning.
		return true
	}
	hit := func(o *Outcome) bool {
		return o != nil && (o.Status == OutcomeFlooded || o.Status == OutcomeMargin)
	}
	switch scenario {
	case ScenarioExpected:
		return hit(i.Scenarios.Expected)
	case ScenarioSevere:
		return hit(i.Scenarios.Expected) || hit(i.Scenarios.Severe)
	}
	return false
}

// IsPeoplePlace reports whether the kind is somewhere people are to be warned
// (as opposed to roads, bridges and hydro plants).
func IsPeoplePlace(kind string) bool {
	return kind == KindSettlement || kind == KindSchool || kind == KindHealth
}

// SortByArrival orders impacts by fast-flood arrival time, nearest first.
// Rows without an arrival time go last.
func SortByArrival(impacts []Impact) {
	slices.SortStableFunc(impacts, func(a, b Impact) int {
		return CompareArrival(a.ArrivalMinFast, b.ArrivalMinFast)
	})
}

// CompareArrival compares two optional arrival times, nil last.
func CompareArrival(a, b *float64) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return cmp.Compare(*a, *b)
}

// Str dereferences an optional string.
func Str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
