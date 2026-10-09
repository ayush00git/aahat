/* Aahat service worker.
 *
 * - Precaches the app shell so the app opens offline (the last village
 *   result is kept by the app itself, in localStorage).
 * - Caches the hashed build assets (including the lazy map chunk) the first
 *   time they are fetched.
 * - Shows flood-warning push notifications and opens the full-screen alert.
 *
 * vite.config.ts fills in the placeholders below at build time.
 */
const PRECACHE = __PRECACHE__;
const VERSION = __VERSION__;
const DEV = __DEV__;

const SHELL_CACHE = `aahat-shell-${VERSION}`;
const ASSET_CACHE = `aahat-assets-${VERSION}`;
const SCOPE = new URL(self.registration.scope);

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      if (!DEV) {
        const cache = await caches.open(SHELL_CACHE);
        await cache.addAll(PRECACHE.map((p) => new Request(new URL(p, SCOPE), { cache: 'reload' })));
      }
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keep = new Set([SHELL_CACHE, ASSET_CACHE]);
      for (const key of await caches.keys()) {
        if (key.startsWith('aahat-') && !keep.has(key)) await caches.delete(key);
      }
      await self.clients.claim();
    })(),
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (DEV || req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== SCOPE.origin || !url.pathname.startsWith(SCOPE.pathname)) return;
  const rel = url.pathname.slice(SCOPE.pathname.length);
  // API calls (same-origin "/api" proxy) are never cached here.
  if (rel.startsWith('api/')) return;

  if (req.mode === 'navigate') {
    event.respondWith(networkFirstShell(req));
  } else if (rel.startsWith('assets/')) {
    event.respondWith(cacheFirst(req, ASSET_CACHE));
  } else if (PRECACHE.includes(rel)) {
    event.respondWith(cacheFirst(req, SHELL_CACHE));
  }
});

async function networkFirstShell(req) {
  try {
    const res = await fetch(req);
    if (res.ok) {
      const cache = await caches.open(SHELL_CACHE);
      await cache.put(new URL('index.html', SCOPE), res.clone());
    }
    return res;
  } catch (err) {
    const cached = await caches.match(new URL('index.html', SCOPE));
    if (cached) return cached;
    throw err;
  }
}

async function cacheFirst(req, cacheName) {
  const cached = await caches.match(req);
  if (cached) return cached;
  const res = await fetch(req);
  if (res.ok) {
    const cache = await caches.open(cacheName);
    await cache.put(req, res.clone());
  }
  return res;
}

// ---- Push notifications -------------------------------------------------
// Payload: {title, body, lang, audio_url, lake_id, place_osm, arrival_min_fast,
// event_id}. audio_url is relative to the API root; the app resolves it.

self.addEventListener('push', (event) => {
  let p = {};
  if (event.data) {
    try {
      p = event.data.json();
    } catch {
      p = { body: event.data.text() };
    }
  }
  const data = { ...p, received_at: Date.now() };
  const title = p.title || (p.lang === 'en' ? 'Aahat: flood warning' : 'आहट: बाढ़ की चेतावनी');
  event.waitUntil(
    (async () => {
      await self.registration.showNotification(title, {
        body: p.body || '',
        lang: p.lang || 'hi',
        icon: new URL('icons/icon-192.png', SCOPE).href,
        badge: new URL('icons/badge-96.png', SCOPE).href,
        tag: `aahat-${p.lake_id || ''}-${p.place_osm || ''}`,
        renotify: true,
        requireInteraction: true,
        vibrate: [600, 200, 600, 200, 600, 200, 600],
        data,
      });
      // If the app is open, switch it to the alert screen right away.
      const wins = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
      for (const w of wins) w.postMessage({ type: 'aahat-alert', payload: data });
    })(),
  );
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const data = event.notification.data || {};
  const url = alertURL(data);
  event.waitUntil(
    (async () => {
      const wins = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
      for (const w of wins) {
        if (new URL(w.url).pathname.startsWith(SCOPE.pathname)) {
          w.postMessage({ type: 'aahat-alert', payload: data });
          try {
            await w.focus();
          } catch {}
          return;
        }
      }
      await self.clients.openWindow(url);
    })(),
  );
});

function alertURL(d) {
  const q = new URLSearchParams();
  const set = (k, v) => {
    if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
  };
  set('title', d.title);
  set('body', d.body);
  set('lang', d.lang);
  set('audio', d.audio_url);
  set('lake', d.lake_id);
  set('place', d.place_osm);
  set('min', d.arrival_min_fast);
  set('event', d.event_id);
  set('at', d.received_at);
  return new URL(`./#/alert?${q}`, SCOPE).href;
}
