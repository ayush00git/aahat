// Fallback coordinates for threats answers without lon/lat (saved by older
// versions): remember them from search results, else search by name.

import { searchPlaces, type Place } from './api';
import { load, save } from './storage';

type LonLat = [number, number];

type Names = [string | null, string | null];

/** Keeps a store to its most recent `max` entries. */
function trim<T>(all: Record<string, T>, max = 200): Record<string, T> {
  const keys = Object.keys(all);
  for (const k of keys.slice(0, Math.max(0, keys.length - max))) delete all[k];
  return all;
}

export function rememberCoords(places: Place[]): void {
  const all = load<Record<string, LonLat>>('coords') ?? {};
  const names = load<Record<string, Names>>('names') ?? {};
  for (const p of places) {
    all[p.osm] = [p.lon, p.lat];
    names[p.osm] = [p.name, p.name_hi];
  }
  save('coords', trim(all));
  save('names', trim(names));
}

/**
 * The names a search result gave for a place. Used when the threats answer
 * has none (places no monitored lake covers).
 */
export function rememberedNames(osm: string): { name: string | null; name_hi: string | null } | null {
  const n = load<Record<string, Names>>('names')?.[osm];
  return n ? { name: n[0], name_hi: n[1] } : null;
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
