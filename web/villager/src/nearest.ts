// Nearest monitored lakes to a place, by straight-line (great-circle)
// distance. This is the one number the app computes itself; it is always
// labelled as straight-line distance, never as distance along the river.

import type { LakeSummary } from './api';

export type LonLat = [number, number];

const R_KM = 6371.0088; // mean Earth radius

/** Great-circle distance in km between two [lon, lat] points (haversine). */
export function greatCircleKm([lon1, lat1]: LonLat, [lon2, lat2]: LonLat): number {
  const rad = Math.PI / 180;
  const dLat = (lat2 - lat1) * rad;
  const dLon = (lon2 - lon1) * rad;
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(lat1 * rad) * Math.cos(lat2 * rad) * Math.sin(dLon / 2) ** 2;
  return 2 * R_KM * Math.asin(Math.min(1, Math.sqrt(a)));
}

export interface NearLake extends LakeSummary {
  km: number;
}

/** The `n` lakes closest to `here`, nearest first. Lakes without coordinates are skipped. */
export function nearestLakes(lakes: LakeSummary[], here: LonLat, n = 3): NearLake[] {
  return lakes
    .filter((l) => Number.isFinite(l.lat) && Number.isFinite(l.lon))
    .map((l) => ({ ...l, km: greatCircleKm(here, [l.lon as number, l.lat as number]) }))
    .sort((a, b) => a.km - b.km)
    .slice(0, n);
}
