package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ayush00git/aahat/api/internal/alert"
)

func TestSubscriptionValidation(t *testing.T) {
	env := newEnv(t, nil)
	valid := map[string]any{"place_osm": "node/8624123539", "place_name": "Bhiyari", "phone": "+919812345678"}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range valid {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	tests := []struct {
		name string
		body any
		code int
	}{
		{"valid", valid, 201},
		{"no plus", with("phone", "919812345678"), 400},
		{"spaces", with("phone", "+91 98123 45678"), 400},
		{"too short", with("phone", "+9112"), 400},
		{"letters", with("phone", "+91981234567x"), 400},
		{"missing phone", with("phone", ""), 400},
		{"bad osm", with("place_osm", "8624123539"), 400},
		{"bad lang", with("lang", "fr"), 400},
		{"bad channel", with("channel", "pigeon"), 400},
		{"english voice", with("lang", "en"), 201},
		{"not json", "{", 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := env.do(t, "POST", "/subscriptions", tc.body)
			if tc.code != 201 {
				expectError(t, r, tc.code)
				return
			}
			expectStatus(t, r, 201)
			sub := decode[alert.Subscription](t, r)
			if !strings.HasPrefix(sub.ID, "sub_") || sub.Channel != "sms" || sub.CreatedAt.IsZero() {
				t.Errorf("got %+v", sub)
			}
		})
	}

	subs := decode[[]alert.Subscription](t, env.do(t, "GET", "/subscriptions?place_osm=node/8624123539", nil))
	if len(subs) != 2 || subs[0].Lang != "hi" || subs[1].Lang != "en" {
		t.Fatalf("subscriptions = %+v", subs)
	}
	if got := decode[[]alert.Subscription](t, env.do(t, "GET", "/subscriptions?place_osm=node/1", nil)); len(got) != 0 {
		t.Errorf("other place has %d subscriptions", len(got))
	}
	expectStatus(t, env.do(t, "DELETE", "/subscriptions/"+subs[0].ID, nil), 204)
	expectError(t, env.do(t, "DELETE", "/subscriptions/"+subs[0].ID, nil), 404)
}

// subscribe creates one subscription per OSM id, in the given order.
func subscribe(t *testing.T, env *testEnv, lang string, osms ...string) {
	t.Helper()
	for i, osm := range osms {
		expectStatus(t, env.do(t, "POST", "/subscriptions", map[string]any{
			"place_osm": osm, "phone": "+91981234560" + string(rune('0'+i)), "lang": lang,
		}), 201)
	}
}

const (
	thirot  = "node/10928951156" // gepang-gath: at_risk, expected outside / severe margin, 107.1 min
	bhiyari = "node/8624123539"  // gepang-gath: in_flood_path, flooded / flooded, 117.1 min
	lobar   = "node/8624122591"  // gepang-gath: at_risk, outside / margin, 131.2 min
	school  = "node/809962485"   // gepang-gath: at_risk, outside / margin, 131.7 min
	kurched = "node/809944575"   // gepang-gath: at_risk, margin / margin, 155.4 min
	yamling = "node/11028063847" // outside under both
)

func TestTriggerPlan(t *testing.T) {
	tests := []struct {
		scenario       string
		wantRecipients []string
		wantPlaces     []string
	}{
		{"severe", []string{thirot, bhiyari, school, kurched}, []string{thirot, bhiyari, lobar, school, kurched}},
		// Thirot, Lobar and the school are only reached by the severe flood.
		{"expected", []string{bhiyari, kurched}, []string{bhiyari, kurched}},
		{"", []string{thirot, bhiyari, school, kurched}, []string{thirot, bhiyari, lobar, school, kurched}}, // defaults to severe
	}
	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			env := newEnv(t, nil)
			// Subscribe out of arrival order to check the plan sorts by arrival.
			subscribe(t, env, "hi", kurched, yamling, school, bhiyari, thirot)

			r := env.do(t, "POST", "/trigger", map[string]any{"lake_id": "gepang-gath", "scenario": tc.scenario, "note": "drill"})
			expectStatus(t, r, 201)
			ev := decode[alert.Event](t, r)

			if want := tc.scenario; want != "" && ev.Scenario != want || ev.Source != "simulation" {
				t.Errorf("scenario/source = %q/%q", ev.Scenario, ev.Source)
			}
			if got := recipientPlaces(ev); strings.Join(got, " ") != strings.Join(tc.wantRecipients, " ") {
				t.Errorf("recipients = %v, want %v", got, tc.wantRecipients)
			}
			var places []string
			for _, p := range ev.AffectedPlaces {
				places = append(places, p.OSM)
			}
			if strings.Join(places, " ") != strings.Join(tc.wantPlaces, " ") {
				t.Errorf("affected places = %v, want %v", places, tc.wantPlaces)
			}
			for _, rc := range ev.Recipients {
				if rc.Delivery.Status != alert.DeliverySent || rc.Delivery.At == nil {
					t.Errorf("delivery = %+v", rc.Delivery)
				}
			}
			if ev.Summary.Sent != len(tc.wantRecipients) {
				t.Errorf("summary = %+v", ev.Summary)
			}

			// The notifier saw the same messages, nearest first.
			sent := env.notifier.Sent()
			if len(sent) != len(ev.Recipients) {
				t.Fatalf("notifier got %d messages, plan has %d", len(sent), len(ev.Recipients))
			}
			for i, m := range sent {
				if m.SubscriptionID != ev.Recipients[i].SubscriptionID || m.EventID != ev.ID {
					t.Errorf("message %d out of order: %+v", i, m)
				}
			}
			// The Bhiyari message carries its data-file arrival time.
			for _, rc := range ev.Recipients {
				if rc.PlaceOSM == bhiyari && (*rc.ArrivalMinFast != 117.1 || !strings.Contains(rc.Message, "117 मिनट") || !strings.Contains(rc.Message, ev.LakeNameHi)) {
					t.Errorf("bhiyari recipient = %+v", rc)
				}
			}

			// The event is in the log with its delivery results.
			logged := decode[alert.Event](t, env.do(t, "GET", "/events/"+ev.ID, nil))
			if len(logged.Recipients) != len(ev.Recipients) || logged.Summary != ev.Summary {
				t.Errorf("logged event = %+v", logged)
			}
			if evs := decode[[]alert.Event](t, env.do(t, "GET", "/events?lake_id=gepang-gath", nil)); len(evs) != 1 {
				t.Errorf("events for lake = %d", len(evs))
			}
			if evs := decode[[]alert.Event](t, env.do(t, "GET", "/events?lake_id=samudra-tapu", nil)); len(evs) != 0 {
				t.Errorf("events for other lake = %d", len(evs))
			}
		})
	}
}

func TestTriggerErrors(t *testing.T) {
	env := newEnv(t, nil)
	expectError(t, env.do(t, "POST", "/trigger", map[string]any{"lake_id": "nope"}), 404)
	expectError(t, env.do(t, "POST", "/trigger", map[string]any{"lake_id": "gepang-gath", "scenario": "worst"}), 400)
	expectError(t, env.do(t, "POST", "/trigger", map[string]any{"lake_id": "gepang-gath", "source": "rumour"}), 400)
	expectError(t, env.do(t, "POST", "/trigger", map[string]any{}), 400)
	expectError(t, env.do(t, "GET", "/events/evt_missing", nil), 404)
}

func TestEnglishMessage(t *testing.T) {
	env := newEnv(t, nil)
	subscribe(t, env, "en", bhiyari)
	ev := decode[alert.Event](t, env.do(t, "POST", "/trigger", map[string]any{"lake_id": "gepang-gath"}))
	want := "WARNING: Flood danger from Gepang Gath lake. Water may reach your village Bhiyari in about 117 minutes. Move to high ground immediately."
	if len(ev.Recipients) != 1 || ev.Recipients[0].Message != want {
		t.Errorf("got %+v", ev.Recipients)
	}
}

func recipientPlaces(ev alert.Event) []string {
	var out []string
	for _, r := range ev.Recipients {
		out = append(out, r.PlaceOSM)
	}
	return out
}

func sign(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return hex.EncodeToString(m.Sum(nil))
}

func TestSensorWebhook(t *testing.T) {
	body := `{"lake_id":"gepang-gath","sensor_id":"gg-level-1","kind":"water_level_drop","value":-1.8,"observed_at":"2026-10-09T10:00:00Z"}`
	tests := []struct {
		name string
		body string
		sig  string
		code int
	}{
		{"valid", body, sign(testSecret, body), 201},
		{"valid with prefix", body, "sha256=" + sign(testSecret, body), 201},
		{"wrong secret", body, sign("other", body), 401},
		{"tampered body", strings.Replace(body, "gepang-gath", "samudra-tapu", 1), sign(testSecret, body), 401},
		{"missing", body, "", 401},
		{"not hex", body, "zz", 401},
		{"bad kind", `{"lake_id":"gepang-gath","sensor_id":"s","kind":"vibes"}`, sign(testSecret, `{"lake_id":"gepang-gath","sensor_id":"s","kind":"vibes"}`), 400},
		{"unknown lake", `{"lake_id":"nope","sensor_id":"s","kind":"manual"}`, sign(testSecret, `{"lake_id":"nope","sensor_id":"s","kind":"manual"}`), 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t, nil)
			subscribe(t, env, "hi", thirot)
			r := env.do(t, "POST", "/webhook/sensor", tc.body, "X-Aahat-Signature", tc.sig)
			if tc.code != 201 {
				expectError(t, r, tc.code)
				if len(env.notifier.Sent()) != 0 {
					t.Error("rejected webhook must not send anything")
				}
				return
			}
			expectStatus(t, r, 201)
			ev := decode[alert.Event](t, r)
			// Sensor triggers use the severe scenario, so Thirot (severe-only) is warned.
			if ev.Source != "sensor" || ev.Scenario != "severe" || ev.Sensor == nil || ev.Sensor.SensorID != "gg-level-1" ||
				len(ev.Recipients) != 1 || ev.Recipients[0].PlaceOSM != thirot {
				t.Errorf("event = %+v", ev)
			}
		})
	}

	t.Run("no secret configured", func(t *testing.T) {
		env := newEnv(t, nil)
		env.handler = New(Config{Store: env.store}) // no WebhookSecret
		expectError(t, env.do(t, "POST", "/webhook/sensor", body, "X-Aahat-Signature", sign("", body)), 503)
	})
}
