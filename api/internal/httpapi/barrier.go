package httpapi

import "net/http"

// lakeBarrier returns the latest barrier-lake scan of the river below a lake
// (barrier/<id>_latest.json), unchanged.
func (s *server) lakeBarrier(w http.ResponseWriter, r *http.Request) {
	b, err := s.catalog.Barrier(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, "lake or barrier scan not found")
		return
	}
	writeRaw(w, "application/json; charset=utf-8", b)
}

// listBarriers returns {lake_id, as_of, candidates} for every indexed lake that has a scan.
func (s *server) listBarriers(w http.ResponseWriter, r *http.Request) {
	scans, err := s.catalog.Barriers(r.Context())
	if err != nil {
		s.fail(w, r, err, "lake index not published yet")
		return
	}
	writeJSON(w, http.StatusOK, scans)
}
