package httpapi

import (
	"cmp"
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
	// Covered is true when at least one monitored lake's analysis includes the place.
	Covered bool `json:"covered"`
	// District and State come from the region-wide places index; null where it has none.
	District *string `json:"district"`
	State    *string `json:"state"`
}

// searchPlaces finds settlements by English or Hindi name across every lake's
// impacts and the region-wide index, including ones outside any flood path (so a
// villager can learn that they are safe). Names are compared by their search keys
// (data.LatinKey and friends), so "Sisu" and "सिस्सू" find Sissu.
//
// Order: exact name, then names that start with the query, then names that contain
// it; within each, places covered by a lake's analysis first, then larger places
// (town, village, hamlet), then the order of the files.
func (s *server) searchPlaces(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	all, err := s.catalog.AllImpacts(r.Context())
	if err != nil {
		s.fail(w, r, err, "lake index not published yet")
		return
	}
	places, err := s.catalog.Places(r.Context())
	if err != nil {
		s.fail(w, r, err, "")
		return
	}
	// The analysed settlements, by OSM id. An impacts row has no district or place kind;
	// those come from the index when it lists the place too.
	covered := map[string]*data.Impact{}
	var coveredOrder []string
	for _, li := range all {
		for i := range li.Impacts {
			im := &li.Impacts[i]
			if im.Kind != data.KindSettlement {
				continue
			}
			if _, seen := covered[im.OSM]; !seen {
				covered[im.OSM] = im
				coveredOrder = append(coveredOrder, im.OSM)
			}
		}
	}
	inIndex := make(map[string]*data.Place, len(covered))
	for i := range places {
		if _, ok := covered[places[i].OSM]; ok {
			inIndex[places[i].OSM] = &places[i]
		}
	}

	type hit struct {
		res  placeResult
		tier int
		size int
	}
	query := data.NewSearchQuery(q)
	var hits []hit
	// A looser pass (see data.NewSearchQuery) runs only when the stricter ones found nothing.
	for pass := 0; pass < query.Passes() && len(hits) == 0; pass++ {
		for _, osm := range coveredOrder {
			im := covered[osm]
			tier := query.Match(pass, data.NewSearchKeys(data.Str(im.Name), data.Str(im.NameHi)))
			if tier == data.NoMatch {
				continue
			}
			h := hit{tier: tier, size: data.PlaceSize(""), res: placeResult{
				OSM: im.OSM, Name: im.Name, NameHi: im.NameHi, Lon: im.Lon, Lat: im.Lat, Covered: true}}
			if p := inIndex[osm]; p != nil {
				h.res.District, h.res.State, h.size = p.District, p.State, data.PlaceSize(p.Place)
			}
			hits = append(hits, h)
		}
		for i := range places {
			p := &places[i]
			if _, ok := covered[p.OSM]; ok {
				continue
			}
			tier := query.Match(pass, p.Keys)
			if tier == data.NoMatch {
				continue
			}
			name := p.Name
			hits = append(hits, hit{tier: tier, size: data.PlaceSize(p.Place), res: placeResult{
				OSM: p.OSM, Name: &name, NameHi: p.NameHi, Lon: p.Lon, Lat: p.Lat, District: p.District, State: p.State}})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int {
		if c := cmp.Compare(a.tier, b.tier); c != 0 {
			return c
		}
		if a.res.Covered != b.res.Covered {
			if a.res.Covered {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.size, b.size)
	})
	out := make([]placeResult, 0, min(len(hits), maxSearchResults))
	for _, h := range hits[:min(len(hits), maxSearchResults)] {
		out = append(out, h.res)
	}
	writeJSON(w, http.StatusOK, out)
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
	LateralM           *float64               `json:"lateral_m"` // the place's distance from the river line
}

// nearbyLateralM: a flood path this close to a place counts as "nearby" even when the place's
// mapped point sits above the estimated flood level (riverside houses, fields and roads may not).
const nearbyLateralM = 2000

type threatsResponse struct {
	OSM    string  `json:"osm"`
	Name   *string `json:"name"`
	NameHi *string `json:"name_hi"`
	// Known is false when no lake's analysis covers this place at all.
	Known   bool     `json:"known"`
	Safe    bool     `json:"safe"`
	Threats []threat `json:"threats"`
	// Nearby: lakes whose flood passes within nearbyLateralM of the place without reaching its
	// mapped point. They do not make the place unsafe, but the app should mention them.
	Nearby []threat `json:"nearby"`
	Lon    *float64 `json:"lon"`
	Lat    *float64 `json:"lat"`
	// District and State come from the region-wide places index; null where it has none.
	District *string `json:"district"`
	State    *string `json:"state"`
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
	resp := threatsResponse{OSM: osm, Threats: []threat{}, Nearby: []threat{}}
	for _, li := range all {
		for _, im := range li.Impacts {
			if im.OSM != osm {
				continue
			}
			if !resp.Known {
				lon, lat := im.Lon, im.Lat
				resp.Known, resp.Name, resp.NameHi, resp.Lon, resp.Lat = true, im.Name, im.NameHi, &lon, &lat
			}
			if im.Status == data.StatusOutside {
				if im.LateralM != nil && *im.LateralM <= nearbyLateralM {
					resp.Nearby = append(resp.Nearby, newThreat(li.Lake, im))
				}
			} else {
				resp.Threats = append(resp.Threats, newThreat(li.Lake, im))
			}
			break // one row per lake (the nearest, as the file is nearest-first)
		}
	}
	place, err := s.catalog.Place(r.Context(), osm)
	if err != nil {
		s.fail(w, r, err, "")
		return
	}
	if place != nil {
		resp.District, resp.State = place.District, place.State
		if !resp.Known { // not analysed for any lake: still say where it is, from the region index
			name, lon, lat := place.Name, place.Lon, place.Lat
			resp.Name, resp.NameHi, resp.Lon, resp.Lat = &name, place.NameHi, &lon, &lat
		}
	}
	sortThreats(resp.Threats)
	sortThreats(resp.Nearby)
	resp.Safe = len(resp.Threats) == 0
	writeJSON(w, http.StatusOK, resp)
}

func newThreat(l data.Lake, im data.Impact) threat {
	t := threat{
		LakeID: l.ID, LakeName: l.Name, LakeNameHi: l.NameHi,
		Status: im.Status, Km: im.Km, LateralM: im.LateralM,
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
