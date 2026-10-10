package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	toolSearchPlaces = "search_places"
	toolPlaceThreats = "place_threats"
	toolListLakes    = "list_lakes"
	toolLakeSummary  = "lake_summary"
	toolWhatToDo     = "what_to_do"

	maxPlacesShown = 8
)

var askTools = []Tool{
	{
		Name: toolSearchPlaces,
		Description: "Find villages and towns by name (English or Hindi; a prefix is enough, and spelling variants such as " +
			"Sisu for Sissu are matched). Returns up to 8 matches with " +
			"their osm id, district and whether our flood analysis covers them. Use the osm id with place_threats.",
		Schema: objectSchema("q", "Village or town name, or the start of it"),
	},
	{
		Name: toolPlaceThreats,
		Description: "Flood threat to one village from the monitored glacial lakes: whether it is covered by the analysis " +
			"(known), whether it is outside every flood path (safe), and for each threatening lake the risk level, " +
			"distance along the river (km) and minutes until the water could arrive (minutes_to_say is the figure to quote). " +
			"if_lake_bursts gives, for the expected flood and for the larger severe flood: " +
			"status (flooded = the flood reaches the village's mapped point; margin = the flood passes within 300 m and " +
			"the ground is less than 10 m above it, so houses near the river may be reached; outside = not reached); " +
			"river_water_depth_m = how deep the flood water would be in the river beside the village, measured from the " +
			"river bed (it is the depth in the river channel, NOT the depth of water in the village); " +
			"village_ground (below_flood_level, above_flood_level or at_flood_level) with village_ground_m = how many " +
			"metres the ground at the village's mapped point is below or above the level the flood water would reach. " +
			"Say it like: \"the river water would be about 6.2 m deep; the village ground is 1.2 m below the flood " +
			"level\". Only status says whether the village is reached: ground below the flood level with status margin " +
			"or outside lies behind higher ground. A missing value means it is not available.",
		Schema: objectSchema("osm", "OSM id of the place, like node/123456"),
	},
	{
		Name:        toolListLakes,
		Description: "All monitored glacial lakes with id, names, district, latest measured area and risk level and score.",
	},
	{
		Name: toolLakeSummary,
		Description: "One monitored lake: risk score and level, the date its data runs until, measured area, the values " +
			"of the risk factors (volume, growth, outlet slope, avalanche source area, glacier distance) and the first " +
			"road, bridge or village its flood would reach downstream.",
		Schema: objectSchema("lake_id", "Lake id from list_lakes, like gepang-gath"),
	},
	{
		Name:        toolWhatToDo,
		Description: "The fixed safety advice for a glacial lake flood and the emergency phone numbers. Use its wording.",
	},
}

func objectSchema(key, description string) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{key: map[string]any{"type": "string", "description": description}},
		"required":   []string{key},
	}
}

var (
	osmID  = regexp.MustCompile(`^(node|way|relation)/[0-9]{1,19}$`)
	lakeID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

// apiTools answers tool calls from the API's own routes, in process: the assistant sees exactly
// what the villager app and the dashboard see, with no second copy of the lookup logic.
type apiTools struct {
	api http.Handler
}

func (t apiTools) exec(lang string) Exec {
	return func(ctx context.Context, name string, args map[string]any) (string, error) {
		var v any
		var err error
		switch name {
		case toolSearchPlaces:
			v, err = t.searchPlaces(ctx, StringArg(args, "q"))
		case toolPlaceThreats:
			v, err = t.placeThreats(ctx, StringArg(args, "osm"))
		case toolListLakes:
			v, err = t.listLakes(ctx)
		case toolLakeSummary:
			v, err = t.lakeSummary(ctx, StringArg(args, "lake_id"))
		case toolWhatToDo:
			v = whatToDo(lang)
		default:
			err = fmt.Errorf("unknown tool %q", name)
		}
		if err != nil {
			return "", err
		}
		return forLang(encode(v), lang), nil
	}
}

// forLang prepares a tool result for a conversation in lang. An English conversation gets no
// Hindi names or labels (the "..._hi" fields): shown to the model, they end up in the answer.
// Numbers keep their exact text.
func forLang(result, lang string) string {
	if lang != LangEnglish || !strings.Contains(result, `_hi"`) {
		return result
	}
	dec := json.NewDecoder(strings.NewReader(result))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return result
	}
	return encode(dropHindi(v))
}

func dropHindi(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if strings.HasSuffix(k, "_hi") {
				delete(t, k)
			} else {
				t[k] = dropHindi(child)
			}
		}
	case []any:
		for i := range t {
			t[i] = dropHindi(t[i])
		}
	}
	return v
}

func encode(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "{}"
	}
	return string(bytes.TrimSpace(b.Bytes()))
}

// get calls a GET route of the API and decodes its JSON body into v.
func (t apiTools) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	rec := &recorder{header: http.Header{}, status: http.StatusOK}
	t.api.ServeHTTP(rec, req)
	if rec.status != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(rec.body.Bytes(), &e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("data not available (status %d)", rec.status)
	}
	return json.Unmarshal(rec.body.Bytes(), v)
}

type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) WriteHeader(status int)      { r.status = status }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }

type placeMatch struct {
	OSM      string  `json:"osm"`
	Name     *string `json:"name"`
	NameHi   *string `json:"name_hi,omitempty"`
	District *string `json:"district,omitempty"`
	Covered  bool    `json:"covered"`
}

func (t apiTools) searchPlaces(ctx context.Context, q string) (any, error) {
	if q == "" {
		return nil, errors.New("q is required")
	}
	var found []placeMatch
	if err := t.get(ctx, "/places/search?q="+url.QueryEscape(q), &found); err != nil {
		return nil, err
	}
	out := struct {
		Matches []placeMatch `json:"matches"`
		Note    string       `json:"note,omitempty"`
	}{Matches: found[:min(len(found), maxPlacesShown)]}
	if len(found) == 0 {
		out.Matches = []placeMatch{}
		out.Note = "no place with this name in the region index"
	}
	return out, nil
}

// scenarioPair holds a value for the expected flood and for the severe one.
type scenarioPair[T any] struct {
	Expected T `json:"expected"`
	Severe   T `json:"severe"`
}

// Where a place's ground lies relative to the flood level.
const (
	groundBelow = "below_flood_level"
	groundAbove = "above_flood_level"
	groundAt    = "at_flood_level"
)

// floodOutcome is what one flood scenario means for a place, in words the model can repeat.
type floodOutcome struct {
	Status string `json:"status,omitempty"` // flooded | margin | outside
	// RiverWaterDepthM is the API's flood_depth_m: the depth of the flood in the river at the
	// cross-section nearest the place, above the river bed. It is not the depth in the village.
	RiverWaterDepthM *float64 `json:"river_water_depth_m,omitempty"`
	// VillageGround and VillageGroundM are the API's height_above_flood_m (ground at the place's
	// mapped point minus the flood level; negative = below it) split into a side and a positive
	// distance, so the model never has to read a sign.
	VillageGround  string   `json:"village_ground,omitempty"`
	VillageGroundM *float64 `json:"village_ground_m,omitempty"`
}

func newFloodOutcome(status string, depth, height *float64) *floodOutcome {
	if status == "" && depth == nil && height == nil {
		return nil
	}
	o := &floodOutcome{Status: status, RiverWaterDepthM: depth}
	if height != nil {
		abs := math.Abs(*height)
		o.VillageGroundM = &abs
		switch {
		case *height < 0:
			o.VillageGround = groundBelow
		case *height > 0:
			o.VillageGround = groundAbove
		default:
			o.VillageGround = groundAt
		}
	}
	return o
}

// placeThreat is one lake's threat to a place, as GET /places/{osm}/threats reports it.
type placeThreat struct {
	LakeID             string   `json:"lake_id"`
	LakeName           string   `json:"lake_name"`
	LakeNameHi         string   `json:"lake_name_hi,omitempty"`
	RiskLevel          string   `json:"risk_level"`
	RiskScore          *float64 `json:"risk_score"`
	Status             string   `json:"status"`
	Km                 *float64 `json:"km"`
	ArrivalMinFast     *float64 `json:"arrival_min_fast"`
	ArrivalMinExpected *float64 `json:"arrival_min_expected"`
	// MinutesToSay is ArrivalMinFast rounded down, as the alerts say it: a warning never promises
	// more time than the data gives.
	MinutesToSay *int `json:"minutes_to_say"`
	// IfLakeBursts is built from the three per-scenario fields below, which are read from the API
	// and not passed on.
	IfLakeBursts *scenarioPair[*floodOutcome] `json:"if_lake_bursts,omitempty"`

	ScenarioStatus    *scenarioPair[string]   `json:"scenario_status,omitempty"`
	FloodDepthM       *scenarioPair[*float64] `json:"flood_depth_m,omitempty"`
	HeightAboveFloodM *scenarioPair[*float64] `json:"height_above_flood_m,omitempty"`
}

// describeScenarios turns the API's per-scenario fields into IfLakeBursts.
func (t *placeThreat) describeScenarios() {
	var status scenarioPair[string]
	var depth, height scenarioPair[*float64]
	if t.ScenarioStatus != nil {
		status = *t.ScenarioStatus
	}
	if t.FloodDepthM != nil {
		depth = *t.FloodDepthM
	}
	if t.HeightAboveFloodM != nil {
		height = *t.HeightAboveFloodM
	}
	t.ScenarioStatus, t.FloodDepthM, t.HeightAboveFloodM = nil, nil, nil
	pair := scenarioPair[*floodOutcome]{
		Expected: newFloodOutcome(status.Expected, depth.Expected, height.Expected),
		Severe:   newFloodOutcome(status.Severe, depth.Severe, height.Severe),
	}
	if pair.Expected != nil || pair.Severe != nil {
		t.IfLakeBursts = &pair
	}
}

type placeThreats struct {
	OSM      string        `json:"osm"`
	Name     *string       `json:"name"`
	NameHi   *string       `json:"name_hi,omitempty"`
	District *string       `json:"district,omitempty"`
	Known    bool          `json:"known"`
	Safe     bool          `json:"safe"`
	Threats  []placeThreat `json:"threats"`
	Nearby   []placeThreat `json:"nearby"`
	Note     string        `json:"note,omitempty"`
}

func (t apiTools) placeThreats(ctx context.Context, osm string) (*placeThreats, error) {
	if !osmID.MatchString(osm) {
		return nil, errors.New("osm must look like node/123, way/123 or relation/123 (find it with search_places)")
	}
	var pt placeThreats
	if err := t.get(ctx, "/places/"+osm+"/threats", &pt); err != nil {
		return nil, err
	}
	for _, list := range [][]placeThreat{pt.Threats, pt.Nearby} {
		for i := range list {
			if m := list[i].ArrivalMinFast; m != nil {
				floor := int(math.Floor(*m))
				list[i].MinutesToSay = &floor
			}
			list[i].describeScenarios()
		}
	}
	switch {
	case !pt.Known:
		pt.Note = "not covered: no monitored lake's flood analysis includes this place, so there is no data for it"
	case pt.Safe && len(pt.Nearby) > 0:
		pt.Note = "outside the estimated flood paths, but a flood path passes nearby (see nearby)"
	case pt.Safe:
		pt.Note = "outside the estimated flood path of every monitored lake"
	}
	return &pt, nil
}

func (t apiTools) listLakes(ctx context.Context) (any, error) {
	var idx struct {
		Lakes []map[string]any `json:"lakes"`
	}
	if err := t.get(ctx, "/lakes", &idx); err != nil {
		return nil, err
	}
	lakes := make([]map[string]any, 0, len(idx.Lakes))
	for _, l := range idx.Lakes {
		row := pick(l, "id", "name", "name_hi", "district", "basin")
		if latest, ok := l["latest"].(map[string]any); ok {
			row["latest"] = pick(latest, "year", "area_m2")
		}
		if risk, ok := l["risk"].(map[string]any); ok {
			row["risk"] = pick(risk, "level", "score", "data_until")
		}
		lakes = append(lakes, row)
	}
	return map[string]any{"lakes": lakes, "score_out_of": 100}, nil
}

func (t apiTools) lakeSummary(ctx context.Context, id string) (any, error) {
	if !lakeID.MatchString(id) {
		return nil, errors.New("lake_id must be an id from list_lakes, like gepang-gath")
	}
	var doc struct {
		Lake map[string]any `json:"lake"`
		Risk map[string]any `json:"risk"`
	}
	if err := t.get(ctx, "/lakes/"+id, &doc); err != nil {
		return nil, err
	}
	out := pick(doc.Lake, "id", "name", "name_hi", "district", "basin", "kind", "first", "latest")
	if doc.Risk == nil {
		out["risk"] = nil
		out["note"] = "no risk assessment published for this lake yet"
	} else {
		risk := pick(doc.Risk, "score", "level", "as_of_season", "data_until", "area_m2", "area_year")
		var factors []map[string]any
		if list, ok := doc.Risk["factors"].([]any); ok {
			for _, f := range list {
				if fm, ok := f.(map[string]any); ok {
					factors = append(factors, pick(fm, "key", "label", "label_hi", "value", "unit", "note"))
				}
			}
		}
		risk["factors"] = factors
		risk["score_out_of"] = 100
		out["risk"] = risk
	}
	if ds, ok := doc.Lake["downstream"].(map[string]any); ok {
		down := pick(ds, "path_km", "exposed_counts")
		for _, key := range []string{"first_exposed", "first_settlement"} {
			if first, ok := ds[key].(map[string]any); ok {
				row := pick(first, "name", "name_hi", "kind", "km", "arrival_min_expected", "arrival_min_fast", "status")
				if m, ok := first["arrival_min_fast"].(float64); ok {
					row["minutes_to_say"] = int(math.Floor(m))
				}
				down[key] = row
			}
		}
		out["downstream"] = down
	} else {
		out["downstream"] = nil
	}
	return out, nil
}

// pick copies the named keys that are present.
func pick(m map[string]any, keys ...string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}
