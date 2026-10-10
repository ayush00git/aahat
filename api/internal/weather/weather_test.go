package weather

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }

// outlook builds 3 past days (tmax each) and 3 forecast days (precipitation, tmax each).
func outlook(pastTmax [3]float64, precip [3]float64, tmax [3]float64) []Day {
	var days []Day
	for i, t := range pastTmax {
		days = append(days, Day{Date: "2026-10-0" + string(rune('7'+i)), TmaxC: f(t), PrecipitationMM: f(99)}) // past rain never counts
	}
	for i := range precip {
		days = append(days, Day{Date: "2026-10-1" + string(rune('0'+i)), PrecipitationMM: f(precip[i]), TmaxC: f(tmax[i]), Forecast: true})
	}
	return days
}

func TestEvaluate(t *testing.T) {
	cool := [3]float64{4, 4, 4}
	tests := []struct {
		name    string
		days    []Day
		level   string
		reasons int
	}{
		{"dry and steady", outlook(cool, [3]float64{0, 1, 2}, cool), LevelNormal, 0},
		{"just under every threshold", outlook(cool, [3]float64{19.9, 19.9, 0}, [3]float64{8.9, 4, 4}), LevelNormal, 0},
		{"one day at 20 mm", outlook(cool, [3]float64{20, 0, 0}, cool), LevelElevated, 1},
		{"total 40 mm without a 20 mm day", outlook(cool, [3]float64{14, 13, 13}, cool), LevelElevated, 1},
		{"heat: tmax 5 C above the past mean", outlook([3]float64{3, 4, 5}, [3]float64{0, 0, 0}, [3]float64{4, 9, 4}), LevelElevated, 1},
		{"heat just short", outlook([3]float64{3, 4, 5}, [3]float64{0, 0, 0}, [3]float64{4, 8.9, 4}), LevelNormal, 0},
		{"one day at 50 mm", outlook(cool, [3]float64{0, 50, 0}, cool), LevelHigh, 2}, // also the 40 mm total
		{"total 100 mm without a 50 mm day", outlook(cool, [3]float64{34, 33, 33}, cool), LevelHigh, 2},
		{"high stays high with heat", outlook(cool, [3]float64{60, 45, 0}, [3]float64{12, 4, 4}), LevelHigh, 3},
		{"no data", nil, LevelNormal, 0},
		{"nulls are ignored", []Day{{Date: "a"}, {Date: "b", Forecast: true}, {Date: "c", Forecast: true, TmaxC: f(30)}}, LevelNormal, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.days)
			if got.Level != tc.level || len(got.Reasons) != tc.reasons || got.Reasons == nil || got.Rule != Rule {
				t.Errorf("got %s %q, want %s with %d reasons", got.Level, got.Reasons, tc.level, tc.reasons)
			}
		})
	}
	if !strings.Contains(Rule, "not a published standard") {
		t.Error("the rule must say the thresholds are our own")
	}
}

const sample = `{"latitude":32.5,"longitude":77.25,"timezone":"Asia/Kolkata",
"daily_units":{"time":"iso8601","precipitation_sum":"mm"},
"daily":{"time":["2026-10-07","2026-10-08","2026-10-09","2026-10-10","2026-10-11","2026-10-12"],
"precipitation_sum":[0.0,1.2,0.0,22.5,3.0,null],
"temperature_2m_max":[2.1,3.0,2.4,1.9,4.2,5.0],
"temperature_2m_min":[-6.0,-5.5,-7.1,-6.4,-4.0,-3.2],
"snowfall_sum":[0.0,0.84,0.0,15.75,2.1,0.0]}}`

func TestClientFetch(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		if r.URL.Query().Get("latitude") == "0" {
			http.Error(w, `{"error":true,"reason":"bad"}`, http.StatusBadRequest)
			return
		}
		w.Write([]byte(sample))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	days, err := c.Fetch(context.Background(), 32.5237, 77.2218)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"latitude=32.5237", "longitude=77.2218", "past_days=3", "forecast_days=3", "timezone=Asia%2FKolkata",
		"daily=precipitation_sum%2Ctemperature_2m_max%2Ctemperature_2m_min%2Csnowfall_sum"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q lacks %q", query, want)
		}
	}
	if len(days) != 6 || days[2].Forecast || !days[3].Forecast {
		t.Fatalf("days = %+v", days)
	}
	d := days[3]
	if d.Date != "2026-10-10" || *d.PrecipitationMM != 22.5 || *d.SnowfallCM != 15.75 || *d.TmaxC != 1.9 || *d.TminC != -6.4 {
		t.Errorf("day = %+v", d)
	}
	if days[5].PrecipitationMM != nil {
		t.Error("a null must stay null")
	}
	if got := Evaluate(days); got.Level != LevelElevated {
		t.Errorf("trigger = %+v", got)
	}

	if _, err := c.Fetch(context.Background(), 0, 0); err == nil {
		t.Error("an error status should fail")
	}
}

type fakeFetcher struct {
	calls int
	err   error
	days  []Day
}

func (f *fakeFetcher) Fetch(context.Context, float64, float64) ([]Day, error) {
	f.calls++
	return f.days, f.err
}

func TestServiceCacheAndStale(t *testing.T) {
	now := time.Date(2026, 10, 10, 6, 0, 0, 0, time.UTC)
	fake := &fakeFetcher{days: outlook([3]float64{4, 4, 4}, [3]float64{0, 60, 0}, [3]float64{4, 4, 4})}
	svc := NewService(fake, DefaultTTL)
	svc.Now = func() time.Time { return now }
	get := func() (Report, error) { return svc.Lake(context.Background(), "gepang-gath", 32.5, 77.2) }

	first, err := get()
	if err != nil || first.Stale || first.LakeID != "gepang-gath" || first.Source != Source ||
		!first.FetchedAt.Equal(now) || first.Trigger.Level != LevelHigh || len(first.Days) != 6 {
		t.Fatalf("first = %+v, %v", first, err)
	}

	now = now.Add(3*time.Hour - time.Second) // still fresh: no fetch
	if r, _ := get(); fake.calls != 1 || r.Stale || !r.FetchedAt.Equal(first.FetchedAt) {
		t.Fatalf("fresh read fetched again (%d calls) or changed: %+v", fake.calls, r)
	}
	if svc.Lake(context.Background(), "samudra-tapu", 32.4, 77.5); fake.calls != 2 {
		t.Error("the cache is per lake")
	}

	now = now.Add(time.Second) // expired, and Open-Meteo is down: the old outlook, marked stale
	fake.err = errors.New("timeout")
	r, err := get()
	if err != nil || !r.Stale || !r.FetchedAt.Equal(first.FetchedAt) || r.Trigger.Level != LevelHigh || fake.calls != 3 {
		t.Fatalf("stale = %+v, %v (%d calls)", r, err, fake.calls)
	}
	if r, _ := get(); !r.Stale || fake.calls != 3 {
		t.Errorf("a failed fetch should not be retried at once (%d calls)", fake.calls)
	}

	now = now.Add(2 * time.Minute) // back up: fetched again
	fake.err, fake.days = nil, outlook([3]float64{4, 4, 4}, [3]float64{0, 0, 0}, [3]float64{4, 4, 4})
	r, err = get()
	if err != nil || r.Stale || !r.FetchedAt.Equal(now) || r.Trigger.Level != LevelNormal || fake.calls != 4 {
		t.Fatalf("refreshed = %+v, %v (%d calls)", r, err, fake.calls)
	}
}

func TestServiceUnavailableWithoutCache(t *testing.T) {
	fake := &fakeFetcher{err: errors.New("no route")}
	svc := NewService(fake, DefaultTTL)
	if _, err := svc.Lake(context.Background(), "x", 1, 2); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if _, err := svc.Lake(context.Background(), "x", 1, 2); !errors.Is(err, ErrUnavailable) || fake.calls != 1 {
		t.Fatalf("err = %v, calls = %d", err, fake.calls)
	}
}
