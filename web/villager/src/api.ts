// Thin client for the Aahat API. Shapes mirror api/internal/httpapi.

import { load, save } from './storage';

export const API_BASE = (import.meta.env.VITE_API_BASE ?? '/api').replace(/\/+$/, '');

export type RiskLevel = 'low' | 'moderate' | 'high' | 'very_high';
export type ThreatStatus = 'in_flood_path' | 'at_risk';
/** Per-scenario outcome from the pipeline: flooded | margin | outside. */
export type ScenarioOutcome = string;

export interface Place {
  osm: string;
  name: string | null;
  name_hi: string | null;
  lon: number;
  lat: number;
}

export interface Pair<T> {
  expected: T;
  severe: T;
}

export interface Threat {
  lake_id: string;
  lake_name: string;
  lake_name_hi: string;
  risk_level: RiskLevel | '';
  risk_score: number | null;
  status: ThreatStatus;
  km: number | null;
  arrival_min_fast: number | null;
  arrival_min_expected: number | null;
  scenario_status: Pair<ScenarioOutcome>;
  flood_depth_m: Pair<number | null>;
  height_above_flood_m: Pair<number | null>;
}

export interface Threats {
  osm: string;
  name: string | null;
  name_hi: string | null;
  known: boolean;
  safe: boolean;
  threats: Threat[];
}

export type Channel = 'sms' | 'voice' | 'webpush';

export interface Subscription {
  id: string;
  place_osm: string;
  place_name: string;
  phone: string;
  lang: 'hi' | 'en';
  channel: string;
  created_at: string;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

/** url resolves an API path (or an absolute URL) against API_BASE. */
export function url(path: string): string {
  if (/^https?:\/\//.test(path)) return path;
  return `${API_BASE}${path.startsWith('/') ? '' : '/'}${path}`;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url(path), init);
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export function searchPlaces(q: string, signal?: AbortSignal): Promise<Place[]> {
  return request<Place[]>(`/places/search?q=${encodeURIComponent(q)}`, { signal });
}

export interface ThreatsResult {
  data: Threats;
  /** Set when the network failed and this is the copy saved earlier. */
  savedAt?: number;
}

const threatsKey = (osm: string) => `threats:${osm}`;

/**
 * getThreats fetches the threats for a place and keeps the answer in
 * localStorage, so the village page still opens without a connection.
 */
export async function getThreats(osm: string): Promise<ThreatsResult> {
  try {
    const data = await request<Threats>(`/places/${encodeURIComponent(osm)}/threats`);
    save(threatsKey(osm), { at: Date.now(), data });
    return { data };
  } catch (err) {
    if (err instanceof ApiError) throw err; // the server answered; don't mask it
    const saved = load<{ at: number; data: Threats }>(threatsKey(osm));
    if (saved) return { data: saved.data, savedAt: saved.at };
    throw err;
  }
}

/** savedThreats returns the last saved answer for a place, if any. */
export function savedThreats(osm: string): Threats | null {
  return load<{ at: number; data: Threats }>(threatsKey(osm))?.data ?? null;
}

export interface SubscribeRequest {
  place_osm: string;
  place_name: string;
  phone?: string;
  lang: 'hi' | 'en';
  channel: Channel;
  push_subscription?: PushSubscriptionJSON;
}

export function createSubscription(req: SubscribeRequest): Promise<Subscription> {
  return request<Subscription>('/subscriptions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
}

export function deleteSubscription(id: string): Promise<void> {
  return request<void>(`/subscriptions/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

/** pushPublicKey returns the VAPID key, or null if the API has no push support. */
export async function pushPublicKey(): Promise<string | null> {
  try {
    const { key } = await request<{ key?: string }>('/push/public-key');
    return key || null;
  } catch {
    return null;
  }
}

export const layerURL = (lakeId: string, name: string) =>
  new URL(url(`/lakes/${encodeURIComponent(lakeId)}/layers/${name}`), location.href).href;
