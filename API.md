# Aahat API

How to read Aahat's glacial-lake data and use its alert, assistant and researcher endpoints.
The implementation notes (packages, environment variables, tests) are in [`api/README.md`](api/README.md).

- **Base URL (demo server):** `https://15-206-138-242.sslip.io/api`
- **Format:** JSON in and out. Errors are `{"error": "..."}` with a fitting HTTP status.
- **CORS:** open to any origin, so a browser app can call it directly.
- **Auth:** reading data is public. Officials' routes need `Authorization: Bearer <token>`.
  Sensor webhooks are signed with HMAC.
- **Freshness:** the pipeline refreshes the data every 2 days from new Sentinel-2 scenes. Each lake
  carries the date its data runs to.

Every flood figure here (discharge, depth, arrival time) is a screening estimate from a 30 m
elevation model, not a hydraulic model. Aahat does not detect a burst as it happens and does not
predict when a lake will burst.

```bash
API=https://15-206-138-242.sslip.io/api
```

## Lakes

| Route | Returns |
| --- | --- |
| `GET /lakes` | Every monitored lake: position, yearly area series, risk score and level, downstream summary, latest drain check |
| `GET /lakes/{id}` | One lake with its latest risk assessment and downstream summary |
| `GET /lakes/{id}/risk` | Risk history, replayed year by year using only the data available at that time, with the factor breakdown |
| `GET /lakes/{id}/downstream` | Flood path and the profile at each station: peak discharge, depth, arrival time, for the expected and severe scenarios |
| `GET /lakes/{id}/impacts` | Settlements, roads, bridges and hydropower sites along the path. Filters: `?status=in_flood_path,at_risk` and `?kind=settlement,bridge` |
| `GET /lakes/{id}/layers/{name}` | GeoJSON for maps. `name` is `outlines`, `outlet`, `glaciers`, `flood_path`, `corridor_expected`, `corridor_severe`, or a year such as `2024` |

```bash
curl $API/lakes
curl $API/lakes/gepang-gath
curl "$API/lakes/gepang-gath/impacts?status=in_flood_path&kind=settlement"
curl $API/lakes/gepang-gath/layers/flood_path
```

Risk score: `100 x size x likelihood`, from lake volume, growth, outlet slope, avalanche source area
and glacier distance. Levels are low (< 20), moderate (< 40), high (< 60) and very high. The sources
for each factor are in the main [README](README.md).

## Weather trigger

| Route | Returns |
| --- | --- |
| `GET /weather` | `[{lake_id, level, reasons, fetched_at, stale}]` for every lake |
| `GET /lakes/{id}/weather` | Three past and three forecast days (rain, snow, max and min temperature) and the trigger |

`level` is `normal`, `elevated` or `high`. The rule is returned with every response:

- **high:** a forecast day with 50 mm of precipitation or more, or 100 mm over the forecast days.
- **elevated:** a forecast day with 20 mm or more, 40 mm over the forecast days, or a forecast
  maximum temperature 5 C or more above the mean of the past 3 days (fast melt).

These thresholds are Aahat's own screening choice, not a published standard. All values come from
[Open-Meteo](https://open-meteo.com) and are cached for 3 hours; `"stale": true` marks an older answer
served because a refresh failed. The level does not change the risk score.

```bash
curl $API/weather
curl $API/lakes/beas-kund/weather
```

## River blockage scan

| Route | Returns |
| --- | --- |
| `GET /barrier` | `[{lake_id, as_of, candidates}]` for every lake with a scan |
| `GET /lakes/{id}/barrier` | The latest scan of the river below one lake |

A candidate is new water on the river compared with earlier years: position, area, width, distance
along the river and the Sentinel-2 scenes it was seen in. `kind` is `possible_barrier_lake` or
`reservoir_level_change` (within 12 km of a dam, with `near_dam_km`).

## Places (villages)

| Route | Returns |
| --- | --- |
| `GET /places/search?q=` | Settlements matching an English or Hindi name, with `district`, `state` and `covered` (inside a mapped flood path) |
| `GET /places/{osm}/threats` | The lakes that threaten a place, with minutes to arrival, and nearby flood paths. URL-encode the id: `node%2F342116474` |
| `GET /places/{type}/{num}/threats` | The same with the id as two path segments: `/places/node/342116474/threats` |

```bash
curl "$API/places/search?q=sissu"
curl $API/places/node/342116474/threats
```

About 19,800 settlements in and around Himachal Pradesh are searchable. A place outside every mapped
flood path is reported as not covered, which is not the same as safe.

## Ask (assistant)

`POST /ask` answers a question in simple Hindi or English.

```bash
curl -X POST $API/ask -H 'content-type: application/json' \
  -d '{"question": "सबसे खतरनाक झील कौन सी है?", "lang": "hi"}'
```

| Field | Meaning |
| --- | --- |
| `question` | Up to 500 characters |
| `lang` | `hi` (default) or `en` |
| `place_osm` | Optional OSM id of the asker's village, for example `node/342116474` |

Response: `{"answer", "lang", "sources": [{"tool", "args"}], "audio_url", "mode"}`.

- The answer is written by Claude, which reads this API through tools. Every number in the answer is
  checked against what the tools returned; if the check fails twice the answer is a fixed template
  built from the data (`"mode": "template"`).
- `sources` lists the lookups behind the answer. `audio_url` is a spoken version, relative to the
  base URL, when voice is enabled.
- Off-topic questions are declined. Answers take roughly 7 to 15 seconds.
- Limit: 10 questions per client per minute (HTTP 429 beyond that). HTTP 503 means no model is configured.

## Alerts

| Route | Auth | Purpose |
| --- | --- | --- |
| `POST /subscriptions` | public | Subscribe a phone or browser to a place's alerts |
| `DELETE /subscriptions/{id}` | public | Unsubscribe |
| `GET /push/public-key` | public | VAPID key for web push |
| `GET /audio/{name}` | public | A spoken warning (MP3) |
| `POST /trigger` | official | Raise an alert for a lake. `"dry_run": true` plans it without sending |
| `GET /events`, `GET /events/{id}` | official | Alert log and one alert's deliveries |
| `GET /subscriptions` | official | Subscriber list |
| `POST /webhook/sensor` | HMAC | A sensor reading; signature in `X-Aahat-Signature` |

Alerts go to the villages nearest the lake first, each with its own arrival time.

`POST /trigger` body: `lake_id` (required), `scenario` (`severe` by default, or `expected`), `dry_run`,
`source` (`simulation` by default, `sensor`, or `satellite_drain_check`) and a free-text `note` of at most
300 characters. `source` and `note` are stored on the event and returned by `GET /events`. The server
raises a `satellite_drain_check` alert itself when a lake newly shows as drained; that source is only
accepted with `"dry_run": true`, so officials review it before anything is sent.

```bash
curl -X POST $API/trigger -H "Authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' -d '{"lake_id": "gepang-gath", "dry_run": true}'
```

## Researcher jobs

Officials only. Queue any lake by name or coordinates; the server maps its water from Sentinel-2,
builds the yearly series and risk assessment, and stores the files.

| Route | Purpose |
| --- | --- |
| `POST /research/lakes` | `{"name", "lat", "lon", "years"}` -> 202 `{"job_id", "status": "queued"}`. `years` is a range such as `2021-2026` (default: the last six seasons) |
| `GET /research/jobs/{id}` | Status (`queued`, `running`, `done`, `failed`), log tail, result files, and a short plain-language summary when done |
| `GET /research/jobs` | Every job, newest first |

```bash
curl -X POST $API/research/lakes -H "Authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' \
  -d '{"name": "Bhrigu Lake", "lat": 32.29333, "lon": 77.24248, "years": "2025-2026"}'
```

Without coordinates the lake is looked up by name among the monitored lakes and the places index;
if it cannot be found the request answers 422 and asks for coordinates. Jobs run one at a time and
take minutes per lake. Results are copied to the project's S3 bucket under `research/<job_id>/`.

## Where the data lives

The API serves the pipeline's output files from the server's disk. The same files are backed up to
a private, versioned S3 bucket after every refresh and once a day (`data/` for lake data, `state/`
for subscriptions and the alert log). See [`infra/ec2/README.md`](infra/ec2/README.md).
