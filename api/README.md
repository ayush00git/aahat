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
| `POST /trigger` | Raise an alert for a lake. Body: `lake_id`, `scenario`, `dry_run`, `source` (`simulation`, `sensor` or `satellite_drain_check`, which must be a dry run) and `note` (at most 300 characters); both are kept on the event |
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

## Assistant (Claude)

`POST /ask` `{"question": "...", "lang": "hi"|"en" (default hi), "place_osm": "node/123" (optional)}`
answers a villager's question in 2-5 short sentences:
`{"answer", "lang", "sources": [{"tool", "args"}], "audio_url"?, "mode": "model"|"template"}`.

`internal/assistant` runs Claude (Anthropic API, or the Bedrock Converse API) with five tools: `search_places`,
`place_threats`, `list_lakes`, `lake_summary`, `what_to_do`. The tools call this API's own routes in
process, so the model sees what the apps show. The model supplies no numbers: after it answers, every
number in the text must appear in a tool result of that conversation (or be 112 / 1077, or a returned
number rounded down, as the alerts round minutes). A failed check gets one corrective retry; a second
failure, a timeout or more than 6 tool rounds gives a fixed template built from the data
(`"mode": "template"`). Limits: question <= 500 characters, 500 output tokens, 20 s, 10 questions per
client IP per minute (`AAHAT_ASK_PER_MIN`). With Polly on, the answer is also spoken (`audio_url`).

| Variable | Meaning |
| --- | --- |
| `AAHAT_ANTHROPIC_API_KEY` | Anthropic API key; takes precedence over Bedrock. Set on the server by `infra/ec2/anthropic-key.sh` |
| `AAHAT_ANTHROPIC_MODEL` | Model id (default `claude-sonnet-5-5`) |
| `AAHAT_BEDROCK_MODEL` | Bedrock model or inference profile id. With neither set, `/ask` answers 503 |
| `AAHAT_BEDROCK_REGION` | Region to call Bedrock in (default `AWS_REGION`) |

`infra/ec2/bedrock-access.sh` picks a model, tests it, grants the server's role and sets both variables.

## Researcher jobs

Officials only (same token as `/trigger`).

| Route | Returns |
| --- | --- |
| `POST /research/lakes` | `{"name", "lat"?, "lon"?, "years"? ("2021-2026"; default the last six seasons), "notify_email"?}` -> 202 `{"job_id", "status": "queued", "lake"}` |
| `GET /research/jobs` | Every job, newest first |
| `GET /research/jobs/{id}` | `{"status": queued\|running\|done\|failed, "lake": {name, lat, lon, source}, "log_tail", "results": {dir, s3_prefix?, files, snapped, series, risk}, "summary"?, "error"?}` |

Without coordinates the lake is looked up by name: a name that is exactly one monitored lake resolves
directly; otherwise Claude searches the monitored lakes and the places index (`find_water`) and may
only point at one of the search results, never state coordinates. If that fails the job is stored as
`failed` and the route answers 422 asking for coordinates.

The API only writes the job file `<jobs dir>/<id>.json` (`AAHAT_JOBS_DIR`, default `<state dir>/jobs`).
On the server `research-jobs.path` starts `infra/ec2/research-worker.sh`, which runs one job at a time:
`aahat discover` to snap to the nearest water body within 1.5 km, the pipeline's `run_lake` and
`run_risk` into `/srv/aahat/research/<id>/`, an optional copy to `s3://$DATA_BUCKET/research/<id>/`,
then status and results back into the job file. The first read of a finished job adds a short `summary`
written by Claude, its numbers checked against the results as in `/ask` (template after two failures).
`notify_email` is stored with the job; nothing sends mail yet.
