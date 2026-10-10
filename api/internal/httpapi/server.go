// Package httpapi is the Aahat HTTP API: lake data for the map, place lookups
// for the villager app, subscriptions, triggers and the alert log.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/data"
	"github.com/ayush00git/aahat/api/internal/store"
	"github.com/ayush00git/aahat/api/internal/weather"
)

type Config struct {
	Catalog *data.Catalog
	Store   store.Store
	Alerts  *alert.Service
	// WebhookSecret signs sensor webhooks. Empty disables the webhook.
	WebhookSecret []byte
	Logger        *slog.Logger
	// OfficialAuth wraps the officials-only routes (subscriber lists,
	// triggers, the alert log). Nil leaves them open.
	OfficialAuth func(http.Handler) http.Handler
	// PushPublicKey is the VAPID public key browsers subscribe with; empty disables /push/public-key.
	PushPublicKey string
	// AudioPath maps an /audio/{name} request to a file of spoken warnings; nil disables /audio.
	AudioPath func(name string) string

	// Weather serves the Open-Meteo outlook and trigger level; nil makes the weather routes answer 503.
	Weather *weather.Service

	// Ask serves POST /ask, the villagers' question-answering assistant (Amazon Bedrock); nil makes it answer 503.
	Ask http.Handler
	// Research serves the officials' researcher jobs under /research/; nil leaves those routes out.
	Research http.Handler
}

type server struct {
	catalog       *data.Catalog
	store         store.Store
	alerts        *alert.Service
	webhookSecret []byte
	log           *slog.Logger
	pushKey       string
	audioPath     func(string) string

	weather *weather.Service
}

// New returns the API's root handler.
func New(cfg Config) http.Handler {
	s := &server{
		catalog: cfg.Catalog, store: cfg.Store, alerts: cfg.Alerts,
		webhookSecret: cfg.WebhookSecret, log: cfg.Logger,
		pushKey: cfg.PushPublicKey, audioPath: cfg.AudioPath,
	}
	s.weather = cfg.Weather
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	official := cfg.OfficialAuth
	if official == nil {
		official = func(h http.Handler) http.Handler { return h }
	}
	only := func(h http.HandlerFunc) http.Handler { return official(h) }

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)

	// Lake data (public, read-only).
	mux.HandleFunc("GET /lakes", s.listLakes)
	mux.HandleFunc("GET /lakes/{id}", s.getLake)
	mux.HandleFunc("GET /lakes/{id}/risk", s.lakeFile("risk.json"))
	mux.HandleFunc("GET /lakes/{id}/downstream", s.lakeFile("downstream.json"))
	mux.HandleFunc("GET /lakes/{id}/impacts", s.lakeImpacts)
	mux.HandleFunc("GET /lakes/{id}/layers/{name}", s.lakeLayer)

	// Weather trigger (Open-Meteo) and barrier-lake scans.
	mux.HandleFunc("GET /lakes/{id}/weather", s.lakeWeather)
	mux.HandleFunc("GET /weather", s.weatherAll)
	mux.HandleFunc("GET /lakes/{id}/barrier", s.lakeBarrier)
	mux.HandleFunc("GET /barrier", s.listBarriers)

	// Villager app.
	mux.HandleFunc("GET /places/search", s.searchPlaces)
	mux.HandleFunc("GET /places/{osm}/threats", s.placeThreats)        // osm URL-encoded: node%2F123
	mux.HandleFunc("GET /places/{type}/{num}/threats", s.placeThreats) // or as two segments
	mux.HandleFunc("POST /subscriptions", s.createSubscription)
	// The random subscription id doubles as the unsubscribe token.
	mux.HandleFunc("DELETE /subscriptions/{id}", s.deleteSubscription)
	mux.HandleFunc("GET /push/public-key", s.pushPublicKey)
	mux.HandleFunc("GET /audio/{name}", s.audio)

	// Officials.
	mux.Handle("GET /subscriptions", only(s.listSubscriptions))
	mux.Handle("POST /trigger", only(s.trigger))
	mux.Handle("GET /events", only(s.listEvents))
	mux.Handle("GET /events/{id}", only(s.getEvent))

	// Assistant (public, rate-limited per client) and researcher jobs (officials).
	ask := cfg.Ask
	if ask == nil {
		ask = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusServiceUnavailable, "the assistant is not configured")
		})
	}
	mux.Handle("POST /ask", ask)
	if cfg.Research != nil {
		for _, route := range []string{"POST /research/lakes", "GET /research/jobs", "GET /research/jobs/{id}"} {
			mux.Handle(route, official(cfg.Research))
		}
	}

	// Sensors authenticate with an HMAC signature instead.
	mux.HandleFunc("POST /webhook/sensor", s.sensorWebhook)

	mux.Handle("/", fallback(mux))

	return recoverer(s.log, cors(mux))
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
