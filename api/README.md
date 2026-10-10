# Aahat API

Go `net/http` service that serves the pipeline's published files (lakes, flood paths, impacts,
places, barrier scans), a weather trigger from Open-Meteo, and the alert workflow.

```
go run ./cmd/server -data ../pipeline/out     # local, listens on :8080
go vet ./... && go test ./...                 # tests use fixtures and fakes, never the network
```

All responses are JSON (`{"error": "..."}` on failure) and CORS is open to any origin.

## Endpoints

Lake data (public)

| Route | Returns |
| --- | --- |
| `GET /health` | `{"status":"ok"}` |
| `GET /lakes` | `lakes/index.json` as published |
| `GET /lakes/{id}` | The index entry (every field, including `drain`), the latest risk assessment and a downstream summary |
| `GET /lakes/{id}/risk` | `risk.json`: risk history and factor breakdown |
| `GET /lakes/{id}/downstream` | `downstream.json`: flood path and per-station profiles |
| `GET /lakes/{id}/impacts` | Impact rows, optionally filtered by `?status=` and `?kind=` (comma-separated) |
| `GET /lakes/{id}/layers/{name}` | A GeoJSON layer: `outlines`, `outlet`, `glaciers`, `flood_path`, `corridor_expected`, `corridor_severe` or a year |
| `GET /lakes/{id}/weather` | Open-Meteo outlook (3 past days, 3 forecast days) and the weather trigger level with its reasons and rule |
| `GET /weather` | `[{lake_id, level, reasons, fetched_at, stale}]` for every indexed lake; lakes whose outlook cannot be fetched are left out |
| `GET /lakes/{id}/barrier` | `barrier/<id>_latest.json`: the latest barrier-lake scan of the river below the lake |
| `GET /barrier` | `[{lake_id, as_of, candidates}]` for every indexed lake that has a scan |

Villager app (public)

| Route | Returns |
| --- | --- |
| `GET /places/search?q=` | Settlements matching an English or Hindi name, with `covered`, `district` and `state` |
| `GET /places/{osm}/threats` | Which lakes threaten a place and the minutes to arrival; `osm` URL-encoded (`node%2F123`) |
| `GET /places/{type}/{num}/threats` | The same, with the OSM id as two path segments |
| `POST /subscriptions` | Subscribe a phone or browser to a place's alerts |
| `DELETE /subscriptions/{id}` | Unsubscribe (the id is the token) |
| `GET /push/public-key` | VAPID public key for web push |
| `GET /audio/{name}` | A spoken warning (when Polly is enabled) |

Officials (`Authorization: Bearer $AAHAT_OFFICIAL_TOKEN` when the token is set)

| Route | Returns |
| --- | --- |
| `GET /subscriptions` | Subscriber list |
| `POST /trigger` | Raise an alert for a lake |
| `GET /events` | Alert log |
| `GET /events/{id}` | One alert with its deliveries |

Sensors

| Route | Returns |
| --- | --- |
| `POST /webhook/sensor` | Sensor reading, signed with HMAC in `X-Aahat-Signature` |

## Weather trigger

`internal/weather` calls Open-Meteo (free, no key) for the lake's coordinates and keeps the answer
in memory for 3 hours per lake. If a refresh fails the last outlook is served with `"stale": true`;
with nothing cached the route answers 503. Every number in the response is Open-Meteo's.

Rule: `high` if any forecast day has precipitation >= 50 mm or the 3-day forecast total is
>= 100 mm; `elevated` if any forecast day has >= 20 mm, the 3-day total is >= 40 mm, or a forecast
day's maximum temperature is >= 5 C above the mean maximum of the past 3 days (fast melt);
otherwise `normal`. These are screening thresholds chosen by us, not a published standard.

`AAHAT_WEATHER_URL` overrides the Open-Meteo forecast endpoint.
