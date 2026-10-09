package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/data"
)

func (s *server) trigger(w http.ResponseWriter, r *http.Request) {
	var in struct {
		LakeID   string `json:"lake_id"`
		Scenario string `json:"scenario"`
		Source   string `json:"source"`
		Note     string `json:"note"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.runTrigger(w, r, alert.TriggerRequest{
		LakeID: in.LakeID, Scenario: in.Scenario, Source: in.Source, Note: in.Note,
	})
}

func (s *server) runTrigger(w http.ResponseWriter, r *http.Request, req alert.TriggerRequest) {
	ev, err := s.alerts.Trigger(r.Context(), req)
	if err != nil {
		s.fail(w, r, err, "lake not found")
		return
	}
	s.log.InfoContext(r.Context(), "alert event",
		"event", ev.ID, "lake", ev.LakeID, "scenario", ev.Scenario, "source", ev.Source,
		"affected_places", len(ev.AffectedPlaces), "recipients", ev.Summary.Recipients,
		"sent", ev.Summary.Sent, "failed", ev.Summary.Failed)
	writeJSON(w, http.StatusCreated, ev)
}

const signatureHeader = "X-Aahat-Signature"

var sensorKinds = map[string]bool{"water_level_drop": true, "flow_surge": true, "manual": true}

// sensorWebhook accepts a reading from a field sensor, signed with the shared
// secret, and fires the severe-scenario alert for its lake.
func (s *server) sensorWebhook(w http.ResponseWriter, r *http.Request) {
	if len(s.webhookSecret) == 0 {
		writeError(w, http.StatusServiceUnavailable, "sensor webhook is not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	if !validSignature(s.webhookSecret, body, r.Header.Get(signatureHeader)) {
		writeError(w, http.StatusUnauthorized, "bad or missing "+signatureHeader)
		return
	}
	var reading alert.SensorReading
	if err := json.Unmarshal(body, &reading); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON body: %v", err))
		return
	}
	if reading.LakeID == "" || reading.SensorID == "" {
		writeError(w, http.StatusBadRequest, "lake_id and sensor_id are required")
		return
	}
	if !sensorKinds[reading.Kind] {
		writeError(w, http.StatusBadRequest, `kind must be "water_level_drop", "flow_surge" or "manual"`)
		return
	}
	s.runTrigger(w, r, alert.TriggerRequest{
		LakeID:   reading.LakeID,
		Scenario: data.ScenarioSevere,
		Source:   alert.SourceSensor,
		Note:     fmt.Sprintf("sensor %s reported %s", reading.SensorID, reading.Kind),
		Sensor:   &reading,
	})
}

// validSignature checks a hex HMAC-SHA256 of the raw body. A "sha256=" prefix
// (GitHub style) is accepted too.
func validSignature(secret, body []byte, header string) bool {
	got, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(header), "sha256="))
	if err != nil || len(got) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

func (s *server) listEvents(w http.ResponseWriter, r *http.Request) {
	evs, err := s.store.ListEvents(r.Context(), r.URL.Query().Get("lake_id"))
	if err != nil {
		s.fail(w, r, err, "")
		return
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *server) getEvent(w http.ResponseWriter, r *http.Request) {
	ev, err := s.store.GetEvent(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, "event not found")
		return
	}
	writeJSON(w, http.StatusOK, ev)
}
