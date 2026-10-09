package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestLakes(t *testing.T) {
	env := newEnv(t, nil)

	r := env.do(t, "GET", "/lakes", nil)
	expectStatus(t, r, 200)
	if !bytes.Equal(r.body, readFixture(t, "lakes/index.json")) {
		t.Error("GET /lakes should pass index.json through unchanged")
	}

	r = env.do(t, "GET", "/lakes/gepang-gath", nil)
	expectStatus(t, r, 200)
	got := decode[struct {
		Lake struct {
			ID   string `json:"id"`
			Risk struct {
				Level string `json:"level"`
			} `json:"risk"`
		} `json:"lake"`
		Risk struct {
			Score   float64           `json:"score"`
			Factors []json.RawMessage `json:"factors"`
		} `json:"risk"`
		Downstream map[string]json.RawMessage `json:"downstream"`
	}](t, r)
	if got.Lake.ID != "gepang-gath" || got.Lake.Risk.Level != "very_high" {
		t.Errorf("lake = %+v", got.Lake)
	}
	if got.Risk.Score != 61.6 || len(got.Risk.Factors) == 0 {
		t.Errorf("risk latest = %+v", got.Risk)
	}
	if _, ok := got.Downstream["path_km"]; !ok {
		t.Error("downstream summary has no path_km")
	}
	if _, ok := got.Downstream["scenarios"]; ok {
		t.Error("downstream summary should drop the per-station scenarios")
	}

	for _, path := range []string{"/lakes/gepang-gath/risk", "/lakes/gepang-gath/downstream"} {
		expectStatus(t, env.do(t, "GET", path, nil), 200)
	}
	for _, path := range []string{"/lakes/nope", "/lakes/nope/risk", "/lakes/..%2Flakes/risk"} {
		expectError(t, env.do(t, "GET", path, nil), 404)
	}
}

func TestImpactsFilter(t *testing.T) {
	env := newEnv(t, nil)
	tests := []struct {
		query string
		want  []string // osm ids, in file order
		code  int
	}{
		{"", []string{"way/42020631", "node/11028063847", "node/10928951156", "node/8624123539", "node/8624122591", "node/809962485", "node/809944575", "node/10224069653"}, 200},
		{"?status=in_flood_path", []string{"way/42020631", "node/8624123539"}, 200},
		{"?status=in_flood_path,at_risk&kind=settlement", []string{"node/10928951156", "node/8624123539", "node/8624122591", "node/809944575"}, 200},
		{"?kind=school", []string{"node/809962485"}, 200},
		{"?kind=road,school&status=outside", []string{}, 200},
		{"?status=flooded", nil, 400},
		{"?kind=temple", nil, 400},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			r := env.do(t, "GET", "/lakes/gepang-gath/impacts"+tc.query, nil)
			if tc.code != 200 {
				expectError(t, r, tc.code)
				return
			}
			expectStatus(t, r, 200)
			rows := decode[[]map[string]any](t, r)
			var got []string
			for _, row := range rows {
				got = append(got, row["osm"].(string))
				if row["scenarios"] == nil {
					t.Error("row lost its scenarios: rows should pass through unchanged")
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
	expectError(t, env.do(t, "GET", "/lakes/nope/impacts", nil), 404)
}

func TestLayers(t *testing.T) {
	env := newEnv(t, nil)
	tests := []struct {
		path string
		code int
	}{
		{"/lakes/gepang-gath/layers/outlet", 200},
		{"/lakes/gepang-gath/layers/outlet.geojson", 200},
		{"/lakes/gepang-gath/layers/flood_path", 404},   // allowed name, file not in fixture
		{"/lakes/samudra-tapu/layers/outlet", 404},      // ditto
		{"/lakes/gepang-gath/layers/impacts.json", 404}, // exists, but not a layer
		{"/lakes/gepang-gath/layers/..%2Findex.json", 404},
		{"/lakes/gepang-gath/layers/..%2F..%2Flakes%2Findex", 404},
		{"/lakes/gepang-gath/layers/%2e%2e", 404},
		{"/lakes/..%2Fgepang-gath/layers/outlet", 404},
		{"/lakes/nope/layers/outlet", 404},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			r := env.do(t, "GET", tc.path, nil)
			if tc.code != 200 {
				expectError(t, r, tc.code)
				return
			}
			expectStatus(t, r, 200)
			if ct := r.header.Get("Content-Type"); ct != "application/geo+json" {
				t.Errorf("content-type = %q", ct)
			}
		})
	}
}

func TestCORSAndFallbacks(t *testing.T) {
	env := newEnv(t, nil)

	r := env.do(t, "OPTIONS", "/subscriptions", nil, "Origin", "https://example.org", "Access-Control-Request-Method", "POST")
	expectStatus(t, r, http.StatusNoContent)
	if r.header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header on preflight")
	}
	r = env.do(t, "GET", "/health", nil)
	expectStatus(t, r, 200)
	if r.header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}

	r = env.do(t, "POST", "/lakes", "{}")
	expectError(t, r, http.StatusMethodNotAllowed)
	if r.header.Get("Allow") != "GET" {
		t.Errorf("Allow = %q", r.header.Get("Allow"))
	}
	expectError(t, env.do(t, "GET", "/nothing/here", nil), 404)
}
