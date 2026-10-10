package httpapi

import (
	"context"
	"net/http"
	"time"
)

const (
	weatherLakeTimeout    = 8 * time.Second  // one lake, asked for directly
	weatherSummaryPerLake = 4 * time.Second  // each lake of the summary
	weatherSummaryTimeout = 20 * time.Second // the whole summary; after it, only cached outlooks
)

// lakeWeather returns the Open-Meteo outlook for a lake and the weather trigger level
// derived from it (see weather.Rule).
func (s *server) lakeWeather(w http.ResponseWriter, r *http.Request) {
	lake, err := s.catalog.Lake(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, "lake not found")
		return
	}
	if lake.Lat == nil || lake.Lon == nil {
		writeError(w, http.StatusNotFound, "lake has no coordinates")
		return
	}
	if s.weather == nil {
		writeError(w, http.StatusServiceUnavailable, "weather is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), weatherLakeTimeout)
	defer cancel()
	report, err := s.weather.Lake(ctx, lake.ID, *lake.Lat, *lake.Lon)
	if err != nil {
		s.log.WarnContext(r.Context(), "weather fetch failed", "lake", lake.ID, "err", err)
		writeError(w, http.StatusServiceUnavailable, "weather is unavailable right now")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

type weatherSummary struct {
	LakeID    string    `json:"lake_id"`
	Level     string    `json:"level"`
	Reasons   []string  `json:"reasons"`
	FetchedAt time.Time `json:"fetched_at"`
	Stale     bool      `json:"stale"`
}

// weatherAll returns the trigger level of every indexed lake, in index order. Lakes are
// fetched one after another; one whose outlook cannot be had is left out.
func (s *server) weatherAll(w http.ResponseWriter, r *http.Request) {
	idx, err := s.catalog.Index(r.Context())
	if err != nil {
		s.fail(w, r, err, "lake index not published yet")
		return
	}
	if s.weather == nil {
		writeError(w, http.StatusServiceUnavailable, "weather is not configured")
		return
	}
	all, cancel := context.WithTimeout(r.Context(), weatherSummaryTimeout)
	defer cancel()
	out := []weatherSummary{}
	for _, lake := range idx.Lakes {
		if lake.Lat == nil || lake.Lon == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(all, weatherSummaryPerLake)
		report, err := s.weather.Lake(ctx, lake.ID, *lake.Lat, *lake.Lon)
		cancel()
		if err != nil {
			s.log.WarnContext(r.Context(), "weather fetch failed", "lake", lake.ID, "err", err)
			continue
		}
		out = append(out, weatherSummary{
			LakeID: report.LakeID, Level: report.Trigger.Level, Reasons: report.Trigger.Reasons,
			FetchedAt: report.FetchedAt, Stale: report.Stale,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
