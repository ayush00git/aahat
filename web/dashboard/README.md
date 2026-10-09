# Aahat officials' dashboard

The web dashboard for DDMA/SDMA staff. It ranks the watched glacial lakes in Himachal Pradesh by risk, shows each lake's outline time-lapse, glaciers, flood path and corridors on a map, and explains the risk score factor by factor. It also lists the downstream settlements and infrastructure in nearest-first order and runs simulated burst alerts.

Stack: Vite, TypeScript, Preact and MapLibre GL JS, with no UI kit. The charts are hand-written inline SVG. Every number on screen comes from the Aahat API (`api/`). The dashboard only formats values (units and rounding) and never computes risk itself.

## Run locally

```sh
# 1. API (from the repo root), serving the pipeline outputs
cd api && go run ./cmd/server -addr 127.0.0.1:8081 -data ../pipeline/out

# 2. Dashboard
cd web/dashboard
npm install
npm run dev            # http://localhost:5173
```

In dev, Vite proxies `/api/*` to `http://127.0.0.1:8081` and strips the `/api` prefix. To point at another API, set `AAHAT_API_TARGET`, for example `AAHAT_API_TARGET=http://127.0.0.1:8080 npm run dev`.

Deep links: `#/<lake-id>/<tab>`, where tab is `risk`, `growth`, `exposure`, `evidence` or `alerts`. For example, `/#/gepang-gath/exposure`.

## Build and deploy

```sh
npm run build          # tsc --noEmit && vite build → dist/
npm run preview        # serves dist/ at http://localhost:4173/officials/, /api proxied as in dev
```

Production is served under **`/officials/`**: Caddy strips the prefix and serves `dist/`, and the API is at `/api` on the same host. Build-time settings:

| Variable | Default | Meaning |
|---|---|---|
| `VITE_API_BASE` | `/api` | API root, either a path on the same host or a full URL (the API sends `Access-Control-Allow-Origin: *`). |
| `DASHBOARD_BASE` | `/officials/` | Public path the build is served from. Dev always uses `/`. |

The MapLibre worker ships as `assets/maplibre-gl-worker-*.mjs`, so the server must send `.mjs` as `text/javascript` (Caddy does). Navigation uses only the URL hash, so no server-side SPA fallback is needed.

Bundle (gzip): app JS about 26 kB, MapLibre about 288 kB (its own chunk, cached separately), CSS about 17 kB (including MapLibre's). Fonts (Inter 400/600 with tabular figures, Noto Sans Devanagari for Hindi) load from Google Fonts (`display=swap`) with a system fallback; formulas use the system monospace. The map worker is 508 kB uncompressed.

## What's on screen

- **Header**: four KPI tiles computed only from `GET /lakes`: very high / high lakes (of all watched), settlements in the flood path, bridges · hydro in the flood path (sums of `downstream.exposed_counts`, with the at-risk counts below), and the newest `risk.data_until`. The disclaimer is in the footer.
- **Sidebar**: lakes ranked by `risk.score`: name, Hindi name, score and level. The list scrolls for any number of lakes.
- **Map**: Esri World Imagery by default, or the OSM map (desaturated). The time-lapse player (previous / play / next, a year slider with one dot per year) shows the selected year's outline over faint outlines of earlier years; partial years are dashed and years without an outline are struck through on the slider. Also shown: RGI 7.0 glaciers, the outlet and spill path, the flood path, the expected and severe corridors, and exposed assets coloured by status (click one for details).
- **Risk tab**: the score written out as `score = 100 × size × likelihood`, then every factor with its value, 0–1 score bar and weight; the rule, note, why it matters and source links sit under **Details**. Below that, the replay of the score by season (each season scored only with the data available then) and the method.
- **Growth tab**: area by year with ± uncertainty whiskers, partial and no-data years flagged, the trend factor, and a table of the yearly measurements (**More columns** adds ±, coverage and clear scenes).
- **Downstream exposure tab**: the expected and severe peak discharge, with the empirical relation and source for each; exposed-asset counts; the first asset and first settlement reached; and OpenStreetMap gaps, if any. Then a nearest-first table with kind filters (click a row to fly to it on the map; **More columns** adds depth and height above flood), followed by the caveats (collapsed).
- **Data & evidence tab**: per season, from the index and the `outlines` layer's feature properties: status, area ± uncertainty, coverage, clear scenes used / found and the scene date range, with a **View imagery** link that opens Copernicus Browser at the lake (Sentinel-2 L2A true colour, limited to that season's scene dates). Sources (Sentinel-2 L2A via AWS Open Data / Earth Search) and a link to the method in the project README.
- **Map popups**: opening an asset popup collapses the Layers box (it reopens when the popup closes) and pans the map so no overlay covers the popup.
- **Alerts tab**: a scenario selector and a **Dry run — don't send to real phones** checkbox (checked by default), then **Simulate burst** and `POST /trigger` with `source: "simulation"` and `dry_run`.
  - A dry run takes one confirmation. The server builds, logs and voices the plan but sends nothing, and records each delivery as `dry_run`.
  - A live alert takes **two** confirmations. Cancel has focus, so pressing Enter doesn't send.
  - Results and log entries are tagged **DRY RUN** or **LIVE ALERT**. The tag comes from the event's `dry_run` flag, or from the delivery statuses when the flag is missing. If a dry run was requested but the server sent anyway (an older API), the result says so.
  - The plan lists affected places in arrival order with subscriber counts, then each recipient with delivery status, SMS text (`short_message`), full message and a play button for `audio_url` (fetched from `${VITE_API_BASE}${audio_url}`). The event log (`GET /events?lake_id=`) expands each event through `GET /events/{id}`.

## Officials' token

`POST /trigger`, `GET /subscriptions`, `GET /events` and `GET /events/{id}` send `Authorization: Bearer <token>` when a token is stored. Public routes (lakes, risk, downstream, impacts, layers) never send it. When the server has `AAHAT_OFFICIAL_TOKEN` set and one of these calls answers 401, the **Officials sign-in** prompt opens. The token is kept in `localStorage` (`aahat.officials.token`) until **Sign out** in the header. A wrong token reopens the prompt. A trigger that hits 401 is not retried automatically after sign-in; run it again.

To test locally:

```sh
cd api && AAHAT_OFFICIAL_TOKEN=<some-test-token> go run ./cmd/server -addr 127.0.0.1:8081 -data ../pipeline/out
```

## Layout of the code

```
src/
  api.ts            fetch wrapper (VITE_API_BASE), one function per endpoint; Bearer token on officials' routes
  auth.ts           officials' token store (localStorage) and the 401 sign-in prompt state
  types.ts          response shapes (nullable where the pipeline writes null)
  format.ts         unit and rounding helpers, labels (minutes are floored, as in the API's messages)
  colors.ts         status and map colours
  map.ts            MapController: MapLibre style, sources and layers, popups, fit/fly helpers
  App.tsx           index loading, hash routing, per-lake data cache
  components/
    LakeList.tsx    ranked sidebar
    MapView.tsx     map, layer toggles, time-lapse
    Panel.tsx       lake header and tabs
    RiskTab.tsx  GrowthTab.tsx  ExposureTab.tsx  EvidenceTab.tsx  AlertsTab.tsx
    charts.tsx      ReplayChart, GrowthChart, Sparkline (inline SVG with hover tooltips)
    Chrome.tsx      header and footer (sources)
    SignIn.tsx      officials sign-in prompt and header sign-out
    ui.tsx          badges, score bar, citation links, loading and error states
```

Data sources credited in the footer: Sentinel-2 (Copernicus, through AWS Open Data), Copernicus DEM GLO-30, RGI 7.0, OpenStreetMap, and NRSC GLOF reports (used as benchmarks).
