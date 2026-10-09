package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/data"
)

const maxSearchResults = 20

type placeResult struct {
	OSM    string  `json:"osm"`
	Name   *string `json:"name"`
	NameHi *string `json:"name_hi"`
	Lon    float64 `json:"lon"`
	Lat    float64 `json:"lat"`
}

// searchPlaces finds settlements by English or Hindi name across every lake's
// impacts, including ones outside any flood path (so a villager can learn
// that they are safe). Prefix matches come first.
func (s *server) searchPlaces(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	all, err := s.catalog.AllImpacts(r.Context())
	if err != nil {
		s.fail(w, r, err, "lake index not published yet")
		return
	}
	var prefix, contains []placeResult
	seen := map[string]bool{}
	for _, li := range all {
		for _, im := range li.Impacts {
			if im.Kind != data.KindSettlement || seen[im.OSM] {
				continue
			}
			name, nameHi := strings.ToLower(data.Str(im.Name)), strings.ToLower(data.Str(im.NameHi))
			res := placeResult{OSM: im.OSM, Name: im.Name, NameHi: im.NameHi, Lon: im.Lon, Lat: im.Lat}
			switch {
			case strings.HasPrefix(name, q) || strings.HasPrefix(nameHi, q):
				prefix = append(prefix, res)
			case strings.Contains(name, q) || strings.Contains(nameHi, q):
				contains = append(contains, res)
			default:
				continue
			}
			seen[im.OSM] = true
		}
	}
	out := append(prefix, contains...)
	if out == nil {
		out = []placeResult{}
	}
	writeJSON(w, http.StatusOK, out[:min(len(out), maxSearchResults)])
}

type scenarioPair[T any] struct {
	Expected T `json:"expected"`
	Severe   T `json:"severe"`
}

type threat struct {
	LakeID             string                 `json:"lake_id"`
	LakeName           string                 `json:"lake_name"`
	LakeNameHi         string                 `json:"lake_name_hi"`
	RiskLevel          string                 `json:"risk_level"`
	RiskScore          *float64               `json:"risk_score"`
	Status             string                 `json:"status"`
	Km                 *float64               `json:"km"`
	ArrivalMinFast     *float64               `json:"arrival_min_fast"`
	ArrivalMinExpected *float64               `json:"arrival_min_expected"`
	ScenarioStatus     scenarioPair[string]   `json:"scenario_status"`
	FloodDepthM        scenarioPair[*float64] `json:"flood_depth_m"`
	HeightAboveFloodM  scenarioPair[*float64] `json:"height_above_flood_m"`
}

type threatsResponse struct {
	OSM    string  `json:"osm"`
	Name   *string `json:"name"`
	NameHi *string `json:"name_hi"`
	// Known is false when no lake's analysis covers this place at all.
	Known   bool     `json:"known"`
	Safe    bool     `json:"safe"`
	Threats []threat `json:"threats"`
}

// placeThreats answers "is my village in a flood path, from which lake, and
// how many minutes do we have?" for one OSM place, nearest threat first.
func (s *server) placeThreats(w http.ResponseWriter, r *http.Request) {
	osm := r.PathValue("osm")
	if t := r.PathValue("type"); t != "" {
		osm = t + "/" + r.PathValue("num")
	}
	if !alert.ValidOSM(osm) {
		writeError(w, http.StatusBadRequest, "osm must look like node/123, way/123 or relation/123")
		return
	}
	all, err := s.catalog.AllImpacts(r.Context())
	if err != nil {
		s.fail(w, r, err, "lake index not published yet")
		return
	}
	resp := threatsResponse{OSM: osm, Threats: []threat{}}
	for _, li := range all {
		for _, im := range li.Impacts {
			if im.OSM != osm {
				continue
			}
			if !resp.Known {
				resp.Known, resp.Name, resp.NameHi = true, im.Name, im.NameHi
			}
			if im.Status == data.StatusOutside {
				continue
			}
			resp.Threats = append(resp.Threats, newThreat(li.Lake, im))
			break // one row per lake (the nearest, as the file is nearest-first)
		}
	}
	sortThreats(resp.Threats)
	resp.Safe = len(resp.Threats) == 0
	writeJSON(w, http.StatusOK, resp)
}

func newThreat(l data.Lake, im data.Impact) threat {
	t := threat{
		LakeID: l.ID, LakeName: l.Name, LakeNameHi: l.NameHi,
		Status: im.Status, Km: im.Km,
		ArrivalMinFast: im.ArrivalMinFast, ArrivalMinExpected: im.ArrivalMinExpected,
	}
	if l.Risk != nil {
		t.RiskLevel, t.RiskScore = l.Risk.Level, l.Risk.Score
	}
	if o := im.Outcome(data.ScenarioExpected); o != nil {
		t.ScenarioStatus.Expected, t.FloodDepthM.Expected, t.HeightAboveFloodM.Expected = o.Status, o.FloodDepthM, o.HeightAboveFloodM
	}
	if o := im.Outcome(data.ScenarioSevere); o != nil {
		t.ScenarioStatus.Severe, t.FloodDepthM.Severe, t.HeightAboveFloodM.Severe = o.Status, o.FloodDepthM, o.HeightAboveFloodM
	}
	return t
}

func sortThreats(ts []threat) {
	slices.SortStableFunc(ts, func(a, b threat) int {
		return data.CompareArrival(a.ArrivalMinFast, b.ArrivalMinFast)
	})
}
