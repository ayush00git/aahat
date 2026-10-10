package httpapi

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
)

func TestPlacesSearch(t *testing.T) {
	env := newEnv(t, nil)
	tests := []struct {
		q    string
		want []string // names, in order
	}{
		{"bhi", []string{"Bhiyari", "Bhiyara"}},  // in both lakes: listed once
		{"YAM", []string{"Yamling"}},             // case-insensitive; outside places are searchable
		{"उदय", []string{"Udaipur"}},             // Hindi name
		{"pur", []string{"Udaipur", "Hamirpur"}}, // substring; covered Udaipur first, then region-wide Hamirpur; the school "..., Udaipur" is not a settlement
		{"ch", []string{"Chhatru", "Kurched"}},   // prefix matches before substring matches
		{"zzz", []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.q, func(t *testing.T) {
			r := env.do(t, "GET", "/places/search?q="+url.QueryEscape(tc.q), nil)
			expectStatus(t, r, 200)
			res := decode[[]placeResult](t, r)
			var got []string
			for _, p := range res {
				got = append(got, *p.Name)
				if p.OSM == "" || p.Lat == 0 || p.Lon == 0 {
					t.Errorf("incomplete result %+v", p)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
	expectError(t, env.do(t, "GET", "/places/search?q=+", nil), 400)
}

func TestPlaceThreats(t *testing.T) {
	env := newEnv(t, nil)

	for _, path := range []string{"/places/node%2F8624123539/threats", "/places/node/8624123539/threats"} {
		t.Run(path, func(t *testing.T) {
			r := env.do(t, "GET", path, nil)
			expectStatus(t, r, 200)
			got := decode[threatsResponse](t, r)
			if got.Safe || !got.Known || *got.Name != "Bhiyari" || len(got.Threats) != 2 {
				t.Fatalf("got %+v", got)
			}
			first, second := got.Threats[0], got.Threats[1]
			// Values straight from the fixture's impacts rows.
			if first.LakeID != "gepang-gath" || *first.ArrivalMinFast != 117.1 || *first.ArrivalMinExpected != 146.4 ||
				first.Status != "in_flood_path" || *first.FloodDepthM.Expected != 7.8 || *first.FloodDepthM.Severe != 17.8 ||
				*first.HeightAboveFloodM.Severe != -1.2 || first.RiskLevel != "very_high" || *first.RiskScore != 61.6 {
				t.Errorf("first threat = %+v", first)
			}
			if second.LakeID != "samudra-tapu" || *second.ArrivalMinFast != 244.2 || second.Status != "at_risk" {
				t.Errorf("second threat = %+v", second)
			}
			if first.LakeNameHi == "" {
				t.Error("missing Hindi lake name")
			}
		})
	}

	t.Run("outside everywhere, but the flood passes nearby", func(t *testing.T) {
		got := decode[threatsResponse](t, env.do(t, "GET", "/places/node/11028063847/threats", nil))
		if !got.Safe || !got.Known || len(got.Threats) != 0 || got.Lon == nil || got.Lat == nil {
			t.Fatalf("got %+v", got)
		}
		// Yamling is 605 m and 741 m from the two lakes' flood paths: both are "nearby".
		if len(got.Nearby) != 2 || *got.Nearby[0].LateralM != 605 || got.Nearby[0].Status != "outside" {
			t.Errorf("nearby = %+v", got.Nearby)
		}
	})
	t.Run("unknown place", func(t *testing.T) {
		got := decode[threatsResponse](t, env.do(t, "GET", "/places/node/1/threats", nil))
		if !got.Safe || got.Known || got.Threats == nil || got.Nearby == nil || got.Lon != nil {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("bad osm", func(t *testing.T) {
		expectError(t, env.do(t, "GET", "/places/village/threats", nil), 400)
		expectError(t, env.do(t, "GET", "/places/node/abc/threats", nil), 400)
	})
}

// Threats are ordered by arrival, not by the lakes' order in the index.
func TestPlaceThreatsSortedByArrival(t *testing.T) {
	env := newEnv(t, func(key string, b []byte) []byte {
		if key == "lakes/gepang-gath/impacts.json" { // make the first lake in the index the farther one
			return bytes.Replace(b, []byte(`"arrival_min_fast": 117.1`), []byte(`"arrival_min_fast": 300`), 1)
		}
		return b
	})
	got := decode[threatsResponse](t, env.do(t, "GET", "/places/node/8624123539/threats", nil))
	if len(got.Threats) != 2 || got.Threats[0].LakeID != "samudra-tapu" || got.Threats[1].LakeID != "gepang-gath" {
		t.Errorf("got %+v", got.Threats)
	}
}

func TestSearchCoversRegionAndRanksCoveredFirst(t *testing.T) {
	env := newEnv(t, nil)
	got := decode[[]placeResult](t, env.do(t, "GET", "/places/search?q=hamir", nil))
	if len(got) != 1 || *got[0].Name != "Hamirpur" || got[0].Covered {
		t.Fatalf("hamirpur: %+v", got)
	}
	got = decode[[]placeResult](t, env.do(t, "GET", "/places/search?q=bhiyar", nil))
	if len(got) != 2 || *got[0].Name != "Bhiyari" || !got[0].Covered || got[1].Covered {
		t.Fatalf("covered first, analysed place not duplicated: %+v", got)
	}
	got = decode[[]placeResult](t, env.do(t, "GET", "/places/search?q=हमीर", nil))
	if len(got) != 1 || got[0].OSM != "node/1522750710" {
		t.Fatalf("hindi: %+v", got)
	}
}

func TestThreatsForUncoveredPlaceNameItButStayUnknown(t *testing.T) {
	env := newEnv(t, nil)
	got := decode[threatsResponse](t, env.do(t, "GET", "/places/node/1522750710/threats", nil))
	if got.Known || !got.Safe || got.Name == nil || *got.Name != "Hamirpur" || got.Lat == nil || len(got.Threats) != 0 {
		t.Fatalf("got %+v", got)
	}
}

// searchIndex stands in for the region index in the spelling tests.
const searchIndex = `{"places": [
	{"osm": "node/1", "name": "Sisul", "name_hi": null, "place": "hamlet", "lon": 75.7, "lat": 33.1},
	{"osm": "node/2", "name": "Sisu Ser", "name_hi": null, "place": "hamlet", "lon": 77.6, "lat": 31.4},
	{"osm": "node/3", "name": "Sissu", "name_hi": null, "place": "village", "lon": 77.1, "lat": 32.4},
	{"osm": "node/4", "name": "Manali", "name_hi": null, "place": "hamlet", "lon": 77.3, "lat": 31.6},
	{"osm": "node/5", "name": "Manali", "name_hi": "मनाली", "place": "town", "lon": 77.1, "lat": 32.2},
	{"osm": "node/6", "name": "Kalang", "name_hi": null, "place": "hamlet", "lon": 77.2, "lat": 32.1},
	{"osm": "node/7", "name": "Keylong", "name_hi": "केलांग", "place": "town", "lon": 77.0, "lat": 32.5},
	{"osm": "node/8", "name": "Koksar", "name_hi": null, "place": "hamlet", "lon": 77.2, "lat": 32.4},
	{"osm": "node/9", "name": "Old Manali", "name_hi": null, "place": "village", "lon": 77.1, "lat": 32.2},
	{"osm": "node/10", "name": "Wangtu", "name_hi": null, "place": "village", "lon": 78.0, "lat": 31.5}
]}`

func TestPlacesSearchSpellings(t *testing.T) {
	env := newEnv(t, func(key string, b []byte) []byte {
		if key == "places/index.json" {
			return []byte(searchIndex)
		}
		return b
	})
	tests := []struct {
		q    string
		want []string // osm numbers, in order
	}{
		{"Sissu", []string{"3", "1", "2"}},   // exact first, then the names that start with it
		{"Sisu", []string{"3", "1", "2"}},    // a doubled letter counts once
		{"sisu ser", []string{"2"}},          // spaces and case do not matter
		{"सिस्सू", []string{"3"}},            // no Hindi name in the index: matched through its sound
		{"manali", []string{"5", "4", "9"}},  // exact: town before hamlet; then the name that contains it
		{"Manaali", []string{"5", "4", "9"}}, // aa = a
		{"मनाली", []string{"5", "4"}},        // the Hindi name, and the hamlet that has none
		{"Keylong", []string{"7"}},
		{"Kyelang", []string{"7", "6"}}, // vowels differ: found by consonants, town first
		{"केलांग", []string{"7"}},
		{"Khoksar", []string{"8"}}, // kh = k when nothing else matches
		{"कोकसर", []string{"8"}},   // Hindi drops the inherent vowel
		{"vangtu", []string{"10"}}, // w = v
		{"qwxzkj", nil},
		{"zzz", nil},
		{"ज्ञ", nil},
		{"k", []string{"7", "6", "8"}}, // one character: prefix matches, as before
	}
	for _, tc := range tests {
		t.Run(tc.q, func(t *testing.T) {
			r := env.do(t, "GET", "/places/search?q="+url.QueryEscape(tc.q), nil)
			expectStatus(t, r, 200)
			var got []string
			for _, p := range decode[[]placeResult](t, r) {
				if !strings.HasPrefix(p.OSM, "node/") || p.Covered {
					continue // the lakes' own settlements (fixture names) are not under test here
				}
				got = append(got, strings.TrimPrefix(p.OSM, "node/"))
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
