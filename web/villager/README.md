# Aahat villager app (आहट)

A small, Hindi-first web app (installable PWA) that tells a villager whether
their village is in a glacial-lake flood path, from which lake, how many
minutes the water could take, and what to do. Every number on screen comes
from the Aahat API. The app only formats them, showing whole minutes rounded
down, which is the same rule the SMS, voice and push messages use.

Stack: Vite, TypeScript and Preact. MapLibre GL is lazy-loaded only on the map
screen. The service worker is hand-written (`sw/sw.js`).

## Run (development)

```sh
# 1. API (from the repo root)
cd api && go run ./cmd/server -addr 127.0.0.1:8080 -data ../pipeline/out

# 2. App
cd web/villager
npm install
npm run dev            # http://localhost:5173
```

The dev server proxies `/api/*` to `http://127.0.0.1:8080/*` and strips the
`/api` prefix. To proxy somewhere else, set `AAHAT_API_TARGET`.

## Build

```sh
npm run build          # type-check (tsc) + vite build → dist/
npm run preview        # serve dist/ on :4173, with the same /api proxy
```

`dist/` is static. Serve it from any static host, and route `/api/` to the Go
server with the prefix stripped. For example, with nginx:

```nginx
location /api/ { proxy_pass http://127.0.0.1:8080/; }
location = /sw.js { add_header Cache-Control "no-cache"; }
```

Service workers and push only work over HTTPS (localhost is exempt). Enable
gzip/brotli for `application/json`, `application/geo+json` and `.mjs`. The
flood-corridor GeoJSON layers are about 250 KB each before compression.

## Environment

| Variable           | Where       | Default                 | Meaning |
|--------------------|-------------|-------------------------|---------|
| `VITE_API_BASE`    | build time  | `/api`                  | API base URL. Use a full URL (e.g. `https://api.example.org`) to call the API cross-origin; the API sends CORS headers. |
| `AAHAT_API_TARGET` | dev/preview | `http://127.0.0.1:8080` | Where the `/api` proxy forwards. |

See `.env.example`.

## Screens (hash routes)

| Route                  | Screen |
|------------------------|--------|
| `#/`                   | Home: village search (debounced), last-viewed village, install button |
| `#/p/node/123`         | Village result: safe, unknown, or one card per threatening lake (nearest first) + "क्या करें" |
| `#/p/node/123/subscribe` | Sign up: SMS, phone call (shown as "coming soon"), or app notification (web push, shown only if the browser supports it and `GET /push/public-key` answers) |
| `#/p/node/123/map`     | Map (lazy): village, latest lake outline, flood path, expected/severe corridors |
| `#/alert?…`            | Full-screen red alert, opened from a push notification |

## Where things are

- `src/content/safety.ts`: the "what to do" advice, emergency numbers and data credits. Edit this to change the advice.
- `src/i18n.ts`: all other UI text (Hindi and English).
- `src/api.ts`: API client and response types. Saves the last threats answer per village in localStorage so the page opens offline.
- `src/push.ts`: service worker registration, Web Push subscribe and unsubscribe.
- `sw/sw.js`: service worker template. At build time `vite.config.ts` writes `dist/sw.js` with the app-shell precache list. It handles offline fallback, `push` (shows the notification) and `notificationclick` (opens `#/alert`).
- `src/map/initMap.ts`: everything MapLibre. It is the only module that imports it, so the map stays out of the first-load bundle.

## Push payload

The service worker expects JSON:
`{title, body, lang, audio_url, lake_id, place_osm, arrival_min_fast, event_id}`.
`audio_url` is a path relative to the API root (e.g. `/audio/abc.mp3`), and
the alert screen plays it from `${VITE_API_BASE}${audio_url}`.

To test locally, start the API, open the app and subscribe to Kalsuin with
"ऐप सूचना", then run:

```sh
curl -X POST localhost:8080/trigger -H 'Content-Type: application/json' \
  -d '{"lake_id":"lam-dal","scenario":"severe"}'
```

If the app is open, it switches to the alert screen. Otherwise, tapping the
notification opens it.
