// What the map should show: the place on screen, published by the result and
// sign-up screens and read by the map panel, which lives outside them so it
// is created once and survives moving between screens.

import { useEffect, useState } from 'preact/hooks';
import { lakesOnce, type LakeSummary } from './api';
import type { LonLat } from './nearest';

export interface MapSelection {
  osm: string;
  label: string;
  /** null when the place's position is unknown. */
  lonlat: LonLat | null;
  /** Lakes whose flood reaches or passes the place (their flood path is drawn). */
  floodLakes: string[];
  /** True when those lakes' floods reach the place (not just pass nearby). */
  threatening: boolean;
}

let current: MapSelection | null = null;
const listeners = new Set<() => void>();

const same = (a: MapSelection | null, b: MapSelection | null) =>
  a === b ||
  (!!a &&
    !!b &&
    a.osm === b.osm &&
    a.label === b.label &&
    a.lonlat?.[0] === b.lonlat?.[0] &&
    a.lonlat?.[1] === b.lonlat?.[1] &&
    a.threatening === b.threatening &&
    a.floodLakes.join() === b.floodLakes.join());

export function setSelection(next: MapSelection | null): void {
  if (same(current, next)) return;
  current = next;
  listeners.forEach((f) => f());
}

export const currentSelection = (): MapSelection | null => current;

export function useSelection(): MapSelection | null {
  const [sel, setSel] = useState(current);
  useEffect(() => {
    const on = () => setSel(current);
    listeners.add(on);
    on();
    return () => void listeners.delete(on);
  }, []);
  return sel;
}

/** All monitored lakes (GET /lakes, fetched once, saved for offline use). */
export function useLakes(): LakeSummary[] | null | 'error' {
  const [lakes, setLakes] = useState<LakeSummary[] | null | 'error'>(null);
  useEffect(() => {
    let live = true;
    lakesOnce().then(
      (ls) => live && setLakes(ls),
      () => live && setLakes('error'),
    );
    return () => {
      live = false;
    };
  }, []);
  return lakes;
}
