// The threats endpoint has no coordinates; search results do. Remember them
// when a village is found, and fall back to searching by name for links
// opened directly.

import { searchPlaces, type Place } from './api';
import { load, save } from './storage';

type LonLat = [number, number];

export function rememberCoords(places: Place[]): void {
  const all = load<Record<string, LonLat>>('coords') ?? {};
  for (const p of places) all[p.osm] = [p.lon, p.lat];
  // Keep the store small: the most recent 200 places.
  const keys = Object.keys(all);
  for (const k of keys.slice(0, Math.max(0, keys.length - 200))) delete all[k];
  save('coords', all);
}

export async function placeCoords(osm: string, names: (string | null)[]): Promise<LonLat | null> {
  const known = load<Record<string, LonLat>>('coords')?.[osm];
  if (known) return known;
  for (const name of names) {
    if (!name) continue;
    try {
      const hit = (await searchPlaces(name)).find((p) => p.osm === osm);
      if (hit) {
        rememberCoords([hit]);
        return [hit.lon, hit.lat];
      }
    } catch {
      /* try the next name */
    }
  }
  return null;
}
