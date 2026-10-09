package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/ayush00git/aahat/api/internal/data"
)

func (s *server) listLakes(w http.ResponseWriter, r *http.Request) {
	b, err := s.catalog.IndexJSON(r.Context())
	if err != nil {
		s.fail(w, r, err, "lake index not published yet")
		return
	}
	writeRaw(w, "application/json; charset=utf-8", b)
}

// getLake returns the index entry, the latest risk assessment with its factor
// breakdown, and downstream.json without the per-station flood profiles.
func (s *server) getLake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	lake, err := s.catalog.Lake(ctx, id)
	if err != nil {
		s.fail(w, r, err, "lake not found")
		return
	}
	resp := struct {
		Lake       json.RawMessage `json:"lake"`
		Risk       json.RawMessage `json:"risk"`
		Downstream json.RawMessage `json:"downstream"`
	}{Lake: lake.Raw, Risk: json.RawMessage("null"), Downstream: json.RawMessage("null")}

	if b, err := s.catalog.LakeFile(ctx, id, "risk.json"); err == nil {
		var risk struct {
			Latest json.RawMessage `json:"latest"`
		}
		if err := json.Unmarshal(b, &risk); err != nil {
			s.fail(w, r, fmt.Errorf("parse risk.json: %w", err), "")
			return
		}
		if risk.Latest != nil {
			resp.Risk = risk.Latest
		}
	} else if !errors.Is(err, data.ErrNotFound) {
		s.fail(w, r, err, "")
		return
	}

	if b, err := s.catalog.LakeFile(ctx, id, "downstream.json"); err == nil {
		var ds map[string]json.RawMessage
		if err := json.Unmarshal(b, &ds); err != nil {
			s.fail(w, r, fmt.Errorf("parse downstream.json: %w", err), "")
			return
		}
		delete(ds, "scenarios") // per-station profiles; GET /lakes/{id}/downstream has them
		summary, _ := json.Marshal(ds)
		resp.Downstream = summary
	} else if !errors.Is(err, data.ErrNotFound) {
		s.fail(w, r, err, "")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *server) lakeFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := s.catalog.LakeFile(r.Context(), r.PathValue("id"), name)
		if err != nil {
			s.fail(w, r, err, "lake or "+name+" not found")
			return
		}
		writeRaw(w, "application/json; charset=utf-8", b)
	}
}

var (
	impactStatuses = []string{data.StatusInFloodPath, data.StatusAtRisk, data.StatusOutside}
	impactKinds    = []string{data.KindSettlement, data.KindBridge, data.KindRoad, data.KindHydro, data.KindSchool, data.KindHealth}
)

// lakeImpacts returns impacts.json rows, unchanged and in file order,
// optionally filtered by ?status=a,b and ?kind=a,b.
func (s *server) lakeImpacts(w http.ResponseWriter, r *http.Request) {
	statuses, err := listParam(r, "status", impactStatuses)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kinds, err := listParam(r, "kind", impactKinds)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	impacts, err := s.catalog.Impacts(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, "lake or impacts.json not found")
		return
	}
	out := []json.RawMessage{}
	for _, im := range impacts {
		if len(statuses) > 0 && !slices.Contains(statuses, im.Status) {
			continue
		}
		if len(kinds) > 0 && !slices.Contains(kinds, im.Kind) {
			continue
		}
		out = append(out, im.Raw)
	}
	writeJSON(w, http.StatusOK, out)
}

// listParam splits a comma-separated query parameter and checks each value.
func listParam(r *http.Request, name string, allowed []string) ([]string, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	var out []string
	for part := range strings.SplitSeq(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !slices.Contains(allowed, part) {
			return nil, fmt.Errorf("unknown %s %q (allowed: %s)", name, part, strings.Join(allowed, ", "))
		}
		out = append(out, part)
	}
	return out, nil
}

func (s *server) lakeLayer(w http.ResponseWriter, r *http.Request) {
	file, ok := data.LayerFile(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown layer")
		return
	}
	b, err := s.catalog.LakeFile(r.Context(), r.PathValue("id"), file)
	if err != nil {
		s.fail(w, r, err, "lake or layer not found")
		return
	}
	writeRaw(w, "application/geo+json", b)
}
