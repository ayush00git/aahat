package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/data"
	"github.com/ayush00git/aahat/api/internal/store"
	"github.com/ayush00git/aahat/api/internal/weather"
)

// fakeWeather is an Open-Meteo stand-in: wet at Gepang Gath, dry elsewhere, and it can fail.
type fakeWeather struct {
	calls   int
	failLat map[float64]bool
}

func (f *fakeWeather) Fetch(_ context.Context, lat, _ float64) ([]weather.Day, error) {
	f.calls++
	if f.failLat[lat] {
		return nil, errors.New("open-meteo is down")
	}
	v := func(x float64) *float64 { return &x }
	rain := 0.0
	if lat == 32.5237 { // gepang-gath in the fixture index
		rain = 62.5
	}
	return []weather.Day{
		{Date: "2026-10-07", PrecipitationMM: v(0), SnowfallCM: v(0), TmaxC: v(3), TminC: v(-6)},
		{Date: "2026-10-08", PrecipitationMM: v(1.5), SnowfallCM: v(0.7), TmaxC: v(2), TminC: v(-7)},
		{Date: "2026-10-09", PrecipitationMM: v(0), SnowfallCM: v(0), TmaxC: v(4), TminC: v(-5)},
		{Date: "2026-10-10", PrecipitationMM: v(rain), SnowfallCM: v(0), TmaxC: v(3), TminC: v(-6), Forecast: true},
		{Date: "2026-10-11", PrecipitationMM: v(0), SnowfallCM: v(0), TmaxC: v(3), TminC: v(-6), Forecast: true},
		{Date: "2026-10-12", PrecipitationMM: v(0), SnowfallCM: v(0), TmaxC: v(3), TminC: v(-6), Forecast: true},
	}, nil
}

// newWeatherEnv is newEnv with a weather service on a fake fetcher and a clock the test moves.
func newWeatherEnv(t *testing.T, edit func(string, []byte) []byte) (*testEnv, *fakeWeather, *time.Time) {
	t.Helper()
	local, err := data.NewLocalStore("testdata")
	if err != nil {
		t.Fatal(err)
	}
	var src data.DataStore = local
	if edit != nil {
		src = editStore{local, edit}
	}
	catalog := data.NewCatalog(src)
	st := store.NewMemoryStore()
	n := alert.NewLogNotifier(nil)
	fake := &fakeWeather{failLat: map[float64]bool{}}
	now := time.Date(2026, 10, 10, 6, 0, 0, 0, time.UTC)
	svc := weather.NewService(fake, weather.DefaultTTL)
	svc.Now = func() time.Time { return now }
	h := New(Config{Catalog: catalog, Store: st, Alerts: alert.NewService(catalog, st, st, alert.NewDispatcher(n)), Weather: svc})
	return &testEnv{handler: h, notifier: n, store: st}, fake, &now
}

func TestLakeWeather(t *testing.T) {
	env, fake, now := newWeatherEnv(t, nil)

	r := env.do(t, "GET", "/lakes/gepang-gath/weather", nil)
	expectStatus(t, r, 200)
	got := decode[weather.Report](t, r)
	if got.LakeID != "gepang-gath" || got.Stale || got.Source != "Open-Meteo (open-meteo.com)" || !got.FetchedAt.Equal(*now) {
		t.Fatalf("report = %+v", got)
	}
	if len(got.Days) != 6 || got.Days[2].Forecast || !got.Days[3].Forecast || *got.Days[3].PrecipitationMM != 62.5 || *got.Days[1].SnowfallCM != 0.7 {
		t.Errorf("days = %+v", got.Days)
	}
	if got.Trigger.Level != "high" || len(got.Trigger.Reasons) == 0 || got.Trigger.Rule != weather.Rule {
		t.Errorf("trigger = %+v", got.Trigger)
	}
	// The documented field names, checked on the wire.
	var wire struct {
		Days []map[string]any `json:"days"`
	}
	json.Unmarshal(r.body, &wire)
	for _, k := range []string{"date", "precipitation_mm", "snowfall_cm", "tmax_c", "tmin_c", "forecast"} {
		if _, ok := wire.Days[0][k]; !ok {
			t.Errorf("day has no %q: %s", k, r.body)
		}
	}

	// Served from the cache for three hours, then stale while Open-Meteo is down.
	*now = now.Add(2 * time.Hour)
	env.do(t, "GET", "/lakes/gepang-gath/weather", nil)
	if fake.calls != 1 {
		t.Errorf("%d fetches, want 1", fake.calls)
	}
	*now = now.Add(2 * time.Hour)
	fake.failLat[32.5237] = true
	r = env.do(t, "GET", "/lakes/gepang-gath/weather", nil)
	expectStatus(t, r, 200)
	if stale := decode[weather.Report](t, r); !stale.Stale || !stale.FetchedAt.Equal(got.FetchedAt) || stale.Trigger.Level != "high" {
		t.Errorf("stale = %+v", stale)
	}

	expectError(t, env.do(t, "GET", "/lakes/nope/weather", nil), 404)
	expectError(t, env.do(t, "GET", "/lakes/..%2Flakes/weather", nil), 404)
}

func TestLakeWeatherUnavailable(t *testing.T) {
	env, fake, _ := newWeatherEnv(t, nil)
	fake.failLat[32.5237] = true
	expectError(t, env.do(t, "GET", "/lakes/gepang-gath/weather", nil), 503)

	// No weather service configured at all.
	expectError(t, newEnv(t, nil).do(t, "GET", "/lakes/gepang-gath/weather", nil), 503)
	expectError(t, newEnv(t, nil).do(t, "GET", "/weather", nil), 503)
}

func TestWeatherSummary(t *testing.T) {
	env, fake, _ := newWeatherEnv(t, nil)
	r := env.do(t, "GET", "/weather", nil)
	expectStatus(t, r, 200)
	got := decode[[]weatherSummary](t, r)
	if len(got) != 2 || got[0].LakeID != "gepang-gath" || got[0].Level != "high" || len(got[0].Reasons) == 0 ||
		got[1].LakeID != "samudra-tapu" || got[1].Level != "normal" || got[1].Reasons == nil || got[1].Stale || got[1].FetchedAt.IsZero() {
		t.Fatalf("summary = %+v", got)
	}

	// A lake whose outlook cannot be fetched and was never cached is left out.
	env, fake, _ = newWeatherEnv(t, nil)
	fake.failLat[32.5237] = true
	got = decode[[]weatherSummary](t, env.do(t, "GET", "/weather", nil))
	if len(got) != 1 || got[0].LakeID != "samudra-tapu" {
		t.Errorf("summary with a failure = %+v", got)
	}
}

func TestBarrier(t *testing.T) {
	env := newEnv(t, nil)

	r := env.do(t, "GET", "/lakes/gepang-gath/barrier", nil)
	expectStatus(t, r, 200)
	if !bytes.Equal(r.body, readFixture(t, "barrier/gepang-gath_latest.json")) {
		t.Error("the barrier file should pass through unchanged")
	}

	tests := []struct {
		name, path string
	}{
		{"lake without a scan", "/lakes/samudra-tapu/barrier"},
		{"unknown lake", "/lakes/nope/barrier"},
		{"a file exists but the lake is not in the index", "/lakes/unlisted/barrier"},
		{"traversal, encoded slash", "/lakes/..%2Fbarrier%2Fgepang-gath/barrier"},
		{"traversal into a lake directory", "/lakes/..%2Flakes%2Fgepang-gath%2Frisk.json%00/barrier"},
		{"traversal, encoded dots", "/lakes/%2e%2e/barrier"},
		{"suffix smuggled into the id", "/lakes/gepang-gath_latest.json/barrier"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expectError(t, env.do(t, "GET", tc.path, nil), 404)
		})
	}

	r = env.do(t, "GET", "/barrier", nil)
	expectStatus(t, r, 200)
	list := decode[[]struct {
		LakeID     string  `json:"lake_id"`
		AsOf       *string `json:"as_of"`
		Candidates []struct {
			Kind      string   `json:"kind"`
			NearDamKm *float64 `json:"near_dam_km"`
			AreaM2    float64  `json:"area_m2"` // not a field the API knows: passed through
		} `json:"candidates"`
	}](t, r)
	if len(list) != 1 || list[0].LakeID != "gepang-gath" || *list[0].AsOf != "2026-10-08" || len(list[0].Candidates) != 2 {
		t.Fatalf("list = %+v", list)
	}
	c := list[0].Candidates
	if c[0].Kind != "possible_barrier_lake" || c[0].NearDamKm != nil || c[1].Kind != "reservoir_level_change" || *c[1].NearDamKm != 0.4 || c[1].AreaM2 != 120500 {
		t.Errorf("candidates = %+v", c)
	}
}

// A scan without candidates lists as an empty array, and a broken file does not hide the others.
func TestBarrierListTolerance(t *testing.T) {
	env := newEnv(t, func(key string, b []byte) []byte {
		if key == "barrier/gepang-gath_latest.json" {
			return []byte(`{"as_of": "2026-10-09"}`)
		}
		return b
	})
	r := env.do(t, "GET", "/barrier", nil)
	expectStatus(t, r, 200)
	if got := string(bytes.TrimSpace(r.body)); got != `[{"lake_id":"gepang-gath","as_of":"2026-10-09","candidates":[]}]` {
		t.Errorf("got %s", got)
	}

	env = newEnv(t, func(key string, b []byte) []byte {
		if key == "barrier/gepang-gath_latest.json" {
			return []byte(`not json`)
		}
		return b
	})
	if got := decode[[]json.RawMessage](t, env.do(t, "GET", "/barrier", nil)); len(got) != 0 {
		t.Errorf("got %s", got)
	}
}

// Index fields the API does not interpret, such as "drain", reach GET /lakes/{id}.
func TestLakePassesDrainThrough(t *testing.T) {
	drain := `{"detected": true, "area_drop_pct": 41.5, "between": ["2026-09-28", "2026-10-08"]}`
	env := newEnv(t, func(key string, b []byte) []byte {
		if key == "lakes/index.json" {
			return bytes.Replace(b, []byte(`"id": "gepang-gath",`), []byte(`"id": "gepang-gath", "drain": `+drain+`,`), 1)
		}
		return b
	})
	got := decode[struct {
		Lake struct {
			Drain json.RawMessage `json:"drain"`
		} `json:"lake"`
	}](t, env.do(t, "GET", "/lakes/gepang-gath", nil))
	var want, have any
	json.Unmarshal([]byte(drain), &want)
	if err := json.Unmarshal(got.Lake.Drain, &have); err != nil {
		t.Fatalf("no drain in the lake: %v", err)
	}
	a, _ := json.Marshal(want)
	b, _ := json.Marshal(have)
	if !bytes.Equal(a, b) {
		t.Errorf("drain = %s, want %s", b, a)
	}
	if r := env.do(t, "GET", "/lakes", nil); !bytes.Contains(r.body, []byte(`"area_drop_pct": 41.5`)) {
		t.Error("GET /lakes dropped drain")
	}
}

func TestDistricts(t *testing.T) {
	env := newEnv(t, nil)
	str := func(s *string) string {
		if s == nil {
			return "<null>"
		}
		return *s
	}

	search := []struct {
		name, q, osm    string
		covered         bool
		district, state string
	}{
		{"covered place: district looked up in the places index", "bhiyari", "node/8624123539", true, "Lahaul and Spiti", "Himachal Pradesh"},
		{"uncovered place, null state", "hamir", "node/1522750710", false, "Hamirpur", "<null>"},
		{"index row without the fields", "bhiyara", "node/9000000001", false, "<null>", "<null>"},
		{"covered place missing from the index", "yam", "node/11028063847", true, "<null>", "<null>"},
	}
	for _, tc := range search {
		t.Run("search: "+tc.name, func(t *testing.T) {
			r := env.do(t, "GET", "/places/search?q="+tc.q, nil)
			got := decode[[]placeResult](t, r)
			if len(got) != 1 || got[0].OSM != tc.osm || got[0].Covered != tc.covered ||
				str(got[0].District) != tc.district || str(got[0].State) != tc.state {
				t.Fatalf("got %s", r.body)
			}
			// Always present, null when unknown.
			row := decode[[]map[string]any](t, r)[0]
			if _, ok := row["district"]; !ok {
				t.Error("no district key")
			}
			if _, ok := row["state"]; !ok {
				t.Error("no state key")
			}
		})
	}

	threats := []struct {
		name, osm       string
		known           bool
		district, state string
	}{
		{"covered", "node/8624123539", true, "Lahaul and Spiti", "Himachal Pradesh"},
		{"uncovered", "node/1522750710", false, "Hamirpur", "<null>"},
		{"covered, not in the index", "node/11028063847", true, "<null>", "<null>"},
		{"unknown place", "node/1", false, "<null>", "<null>"},
	}
	for _, tc := range threats {
		t.Run("threats: "+tc.name, func(t *testing.T) {
			r := env.do(t, "GET", "/places/"+tc.osm+"/threats", nil)
			expectStatus(t, r, 200)
			got := decode[threatsResponse](t, r)
			if got.Known != tc.known || str(got.District) != tc.district || str(got.State) != tc.state {
				t.Fatalf("got %s", r.body)
			}
			if _, ok := decode[map[string]any](t, r)["district"]; !ok {
				t.Error("no district key")
			}
		})
	}
}

// An index published before districts existed still works.
func TestDistrictsAbsentFromOldIndex(t *testing.T) {
	env := newEnv(t, func(key string, b []byte) []byte {
		if key == "places/index.json" {
			return []byte(`{"places":[{"osm":"node/8624123539","name":"Bhiyari","name_hi":null,"place":"village","lon":76.7,"lat":32.67}]}`)
		}
		return b
	})
	got := decode[threatsResponse](t, env.do(t, "GET", "/places/node/8624123539/threats", nil))
	if !got.Known || got.District != nil || got.State != nil || len(got.Threats) != 2 {
		t.Errorf("got %+v", got)
	}
}
