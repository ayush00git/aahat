// Hash routes, so the app works from any static host and offline:
//   #/                       home / search
//   #/p/node/123             village result
//   #/p/node/123/subscribe   sign up for warnings
//   #/p/node/123/map         map
//   #/alert?title=…&min=…    full-screen alert (opened from a push notification)

import { useEffect, useState } from 'preact/hooks';

export type Route =
  | { name: 'home' }
  | { name: 'place'; osm: string }
  | { name: 'subscribe'; osm: string }
  | { name: 'map'; osm: string }
  | { name: 'alert'; params: URLSearchParams };

export function parse(hash: string): Route {
  const h = hash.replace(/^#/, '');
  const [path, query = ''] = h.split('?', 2);
  if (path === '/alert') return { name: 'alert', params: new URLSearchParams(query) };
  const m = /^\/p\/(node|way|relation)\/(\d+)(?:\/(subscribe|map))?\/?$/.exec(path);
  if (m) {
    const osm = `${m[1]}/${m[2]}`;
    if (m[3] === 'subscribe') return { name: 'subscribe', osm };
    if (m[3] === 'map') return { name: 'map', osm };
    return { name: 'place', osm };
  }
  return { name: 'home' };
}

export const href = {
  home: () => '#/',
  place: (osm: string) => `#/p/${osm}`,
  subscribe: (osm: string) => `#/p/${osm}/subscribe`,
  map: (osm: string) => `#/p/${osm}/map`,
};

export function navigate(to: string): void {
  location.hash = to;
}

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parse(location.hash));
  useEffect(() => {
    const on = () => {
      setRoute(parse(location.hash));
      window.scrollTo(0, 0);
    };
    window.addEventListener('hashchange', on);
    return () => window.removeEventListener('hashchange', on);
  }, []);
  return route;
}
