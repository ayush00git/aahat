import type {
  AlertEvent,
  BarrierScan,
  DownstreamFile,
  GeoJSONDoc,
  Impact,
  LakeIndex,
  LayerName,
  RiskFile,
  ScenarioName,
  WeatherBrief,
  WeatherOutlook,
} from "./types";
import { getToken, requireSignIn } from "./auth";

// VITE_API_BASE: "/api" (default; proxied to the Go API by Vite in dev) or a
// full URL such as https://api.example.org. The API allows any origin.
const BASE = (import.meta.env.VITE_API_BASE as string | undefined)?.replace(/\/+$/, "") || "/api";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

/**
 * `official` marks the officials-only routes: they carry the stored token as
 * "Authorization: Bearer <token>", and a 401 asks the user to sign in.
 */
async function request<T>(path: string, init?: RequestInit, official = false): Promise<T> {
  let res: Response;
  const headers: Record<string, string> = { Accept: "application/json" };
  if (init?.body) headers["Content-Type"] = "application/json";
  const token = official ? getToken() : null;
  if (token) headers.Authorization = `Bearer ${token}`;
  try {
    res = await fetch(BASE + path, { ...init, headers });
  } catch (e) {
    throw new ApiError(0, `API unreachable at ${BASE} (${(e as Error).message})`);
  }
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body && typeof body.error === "string") msg = `${res.status}: ${body.error}`;
    } catch {
      /* not JSON */
    }
    if (official && res.status === 401) {
      requireSignIn(token ? "The stored token was not accepted." : "This server requires an officials' token.");
    }
    throw new ApiError(res.status, msg);
  }
  return (await res.json()) as T;
}

const enc = encodeURIComponent;

export const api = {
  base: BASE,
  lakes: () => request<LakeIndex>("/lakes"),
  risk: (id: string) => request<RiskFile>(`/lakes/${enc(id)}/risk`),
  downstream: (id: string) => request<DownstreamFile>(`/lakes/${enc(id)}/downstream`),
  impacts: (id: string) =>
    request<Impact[]>(`/lakes/${enc(id)}/impacts?status=in_flood_path,at_risk`),
  weather: () => request<WeatherBrief[]>("/weather"),
  lakeWeather: (id: string) => request<WeatherOutlook>(`/lakes/${enc(id)}/weather`),
  barrier: (id: string) => request<BarrierScan>(`/lakes/${enc(id)}/barrier`),
  layer: (id: string, name: LayerName) => request<GeoJSONDoc>(`/lakes/${enc(id)}/layers/${enc(name)}`),
  trigger: (lakeId: string, scenario: ScenarioName, note: string, dryRun: boolean) =>
    request<AlertEvent>(
      "/trigger",
      {
        method: "POST",
        body: JSON.stringify({ lake_id: lakeId, scenario, source: "simulation", note, dry_run: dryRun }),
      },
      true,
    ),
  events: (lakeId: string) => request<AlertEvent[]>(`/events?lake_id=${enc(lakeId)}`, undefined, true),
  event: (id: string) => request<AlertEvent>(`/events/${enc(id)}`, undefined, true),
};
