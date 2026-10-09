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
		{"bhi", []string{"Bhiyari"}},           // in both lakes: listed once
		{"YAM", []string{"Yamling"}},           // case-insensitive; outside places are searchable
		{"उदय", []string{"Udaipur"}},           // Hindi name
		{"pur", []string{"Udaipur"}},           // substring; the school "..., Udaipur" is not a settlement
		{"ch", []string{"Chhatru", "Kurched"}}, // prefix matches before substring matches
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
