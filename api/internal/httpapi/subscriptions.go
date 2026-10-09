package httpapi

import (
	"net/http"
	"time"

	"github.com/ayush00git/aahat/api/internal/alert"
)

func (s *server) createSubscription(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PlaceOSM  string `json:"place_osm"`
		PlaceName string `json:"place_name"`
		Phone     string `json:"phone"`
		Lang      string `json:"lang"`
		Channel   string `json:"channel"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	sub := alert.Subscription{
		PlaceOSM: in.PlaceOSM, PlaceName: in.PlaceName, Phone: in.Phone,
		Lang: in.Lang, Channel: in.Channel,
	}
	if err := sub.Normalize(); err != nil {
		s.fail(w, r, err, "")
		return
	}
	sub.ID = alert.NewID("sub")
	sub.CreatedAt = time.Now().UTC()
	if err := s.store.CreateSubscription(r.Context(), sub); err != nil {
		s.fail(w, r, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, sub)
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
	writeJSON(w, http.StatusOK, subs)
}
