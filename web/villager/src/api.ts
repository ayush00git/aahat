// Thin client for the Aahat API. Shapes mirror api/internal/httpapi.

import { load, save } from './storage';

export const API_BASE = (import.meta.env.VITE_API_BASE ?? '/api').replace(/\/+$/, '');

export type RiskLevel = 'low' | 'moderate' | 'high' | 'very_high';
/** "outside" only appears on `nearby` rows. */
export type ThreatStatus = 'in_flood_path' | 'at_risk' | 'outside';
/** Per-scenario outcome from the pipeline: flooded | margin | outside. */
export type ScenarioOutcome = string;

export interface Place {
  osm: string;
  name: string | null;
  name_hi: string | null;
  lon: number;
  lat: number;
  /**
   * True when at least one monitored lake's analysis includes the place.
   * Newer API only: a missing field means "not known", so no badge is shown.
   */
  covered?: boolean;
  /** Admin areas the place lies in (newer API); tell same-named villages apart. */
  district?: string | null;
  state?: string | null;
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
  /** Distance of the place from the river / flood line (nearby rows). */
  lateral_m?: number | null;
}

export interface Threats {
  osm: string;
  name: string | null;
  name_hi: string | null;
  known: boolean;
  safe: boolean;
  threats: Threat[];
  /**
   * Lakes whose flood passes within ~2 km of the place without reaching its
   * mapped point. Optional: answers saved by older app versions lack it.
   */
  nearby?: Threat[];
  lon?: number | null;
  lat?: number | null;
  district?: string | null;
  state?: string | null;
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

/** A watched lake, as much of GET /lakes as the "not covered" screen shows. */
export interface LakeSummary {
  id: string;
  name: string;
  name_hi: string;
  district: string;
  risk_level: RiskLevel | null;
  /** The API's risk score; missing in copies saved by older app versions. */
  risk_score?: number | null;
  /** Lake position from the API; null in copies saved by older app versions. */
  lat: number | null;
  lon: number | null;
}

interface LakeIndex {
  lakes: {
    id: string;
    name: string;
    name_hi: string;
    district: string;
    lat?: number | null;
    lon?: number | null;
    risk: { level: RiskLevel; score: number | null } | null;
  }[];
}

/**
 * getLakes lists the monitored lakes, highest risk first (the API's own
 * score order). The answer is kept in localStorage so it shows offline too.
 */
export async function getLakes(): Promise<LakeSummary[]> {
  try {
    const idx = await request<LakeIndex>('/lakes');
    const lakes = [...idx.lakes]
      .sort((a, b) => (b.risk?.score ?? -1) - (a.risk?.score ?? -1))
      .map((l) => ({
        id: l.id,
        name: l.name,
        name_hi: l.name_hi,
        district: l.district,
        risk_level: l.risk?.level ?? null,
        risk_score: typeof l.risk?.score === 'number' ? l.risk.score : null,
        lat: typeof l.lat === 'number' ? l.lat : null,
        lon: typeof l.lon === 'number' ? l.lon : null,
      }));
    save('lakes', lakes);
    return lakes;
  } catch (err) {
    const saved = load<LakeSummary[]>('lakes');
    if (saved) return saved;
    throw err;
  }
}

export type WeatherLevel = 'normal' | 'elevated' | 'high';

/** One row of GET /weather: the short-term weather trigger for a lake. */
export interface LakeWeather {
  lake_id: string;
  level: WeatherLevel;
  reasons: string[];
  fetched_at: string;
  stale: boolean;
}

let weatherMemo: Promise<Map<string, LakeWeather>> | null = null;

/**
 * getWeather fetches GET /weather once per app session and indexes it by
 * lake. It never rejects: without an answer the map is empty and the app
 * simply shows no weather line (a failed fetch is retried on the next call).
 */
export function getWeather(): Promise<Map<string, LakeWeather>> {
  if (!weatherMemo) {
    weatherMemo = request<LakeWeather[]>('/weather').then(
      (rows) => new Map((Array.isArray(rows) ? rows : []).filter((r) => r && r.lake_id).map((r) => [r.lake_id, r])),
      () => {
        weatherMemo = null;
        return new Map<string, LakeWeather>();
      },
    );
  }
  return weatherMemo;
}

/** The assistant's answer (POST /ask). */
export interface AskAnswer {
  answer: string;
  lang: string;
  sources?: { tool: string; args?: unknown }[];
  /** Spoken answer: a path under the API base, e.g. "/audio/abc.mp3". */
  audio_url?: string | null;
  mode?: string;
}

/** ask sends one question to the Aahat assistant. It can take 7-15 seconds. */
export function ask(req: { question: string; lang: 'hi' | 'en'; place_osm?: string }): Promise<AskAnswer> {
  return request<AskAnswer>('/ask', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
}

let lakesMemo: Promise<LakeSummary[]> | null = null;

/**
 * lakesOnce shares one GET /lakes between the map and the result page for the
 * app session (a failed fetch is retried on the next call).
 */
export function lakesOnce(): Promise<LakeSummary[]> {
  if (!lakesMemo) {
    lakesMemo = getLakes().catch((err) => {
      lakesMemo = null;
      throw err;
    });
  }
  return lakesMemo;
}

export const layerURL = (lakeId: string, name: string) =>
  new URL(url(`/lakes/${encodeURIComponent(lakeId)}/layers/${name}`), location.href).href;
