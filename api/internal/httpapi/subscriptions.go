package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/data"
)

func (s *server) createSubscription(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PlaceOSM  string          `json:"place_osm"`
		PlaceName string          `json:"place_name"`
		Phone     string          `json:"phone"`
		Lang      string          `json:"lang"`
		Channel   string          `json:"channel"`
		Push      json.RawMessage `json:"push_subscription"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	sub := alert.Subscription{
		PlaceOSM: in.PlaceOSM, PlaceName: in.PlaceName, Phone: in.Phone,
		Lang: in.Lang, Channel: in.Channel, Push: in.Push,
	}
	if err := sub.Normalize(); err != nil {
		s.fail(w, r, err, "")
		return
	}
	known, err := s.knownPlace(r, sub.PlaceOSM)
	if err != nil {
		s.fail(w, r, err, "lake index not published yet")
		return
	}
	if !known {
		writeError(w, http.StatusNotFound, "place_osm "+sub.PlaceOSM+" is not a place Aahat knows; find it with GET /places/search")
		return
	}
	sub.ID = alert.NewID("sub")
	sub.CreatedAt = time.Now().UTC()
	if err := s.store.CreateSubscription(r.Context(), sub); err != nil {
		s.fail(w, r, err, "")
		return
	}
	sub.Push = nil // the browser already has it; don't echo keys back
	writeJSON(w, http.StatusCreated, sub)
}

// knownPlace reports whether an OSM id can be subscribed to: a settlement of the region-wide
// places index, or a place where people are (settlement, school, health facility) in a lake's
// impacts. Anything else could never receive an alert.
func (s *server) knownPlace(r *http.Request, osm string) (bool, error) {
	place, err := s.catalog.Place(r.Context(), osm)
	if err != nil || place != nil {
		return place != nil, err
	}
	all, err := s.catalog.AllImpacts(r.Context())
	if err != nil {
		return false, err
	}
	for _, li := range all {
		for _, im := range li.Impacts {
			if im.OSM == osm && data.IsPeoplePlace(im.Kind) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *server) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteSubscription(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, r, err, "subscription not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := s.store.ListSubscriptions(r.Context(), r.URL.Query().Get("place_osm"))
	if err != nil {
		s.fail(w, r, err, "")
		return
	}
	for i := range subs {
		subs[i].Push = nil // push keys stay server-side
	}
	writeJSON(w, http.StatusOK, subs)
}
