// Shapes of the Aahat API responses (api/internal/httpapi, api/internal/data,
// api/internal/alert). The pipeline writes null where it has no value, so
// optional numbers are typed `number | null` and never defaulted to zero.

export type RiskLevel = "low" | "moderate" | "high" | "very_high";
export type YearStatus = "ok" | "partial" | "not_found" | "no_data";
export type ImpactStatus = "in_flood_path" | "at_risk" | "outside";
export type ScenarioName = "expected" | "severe";
export type ImpactKind = "settlement" | "bridge" | "road" | "hydro" | "school" | "health";

export interface YearRecord {
  year: number;
  status: YearStatus;
  area_m2: number | null;
  uncertainty_m2: number | null;
  coverage: number | null;
  scenes_clear: number | null;
  /** Sentinel-2 items the year's outline was built from (newer pipeline runs only). */
  scene_ids?: string[];
  scene_days?: string[];
}

/** The latest sudden-drainage check of a lake (pipeline drain.py), as the index carries it. */
export interface DrainSummary {
  as_of?: string | null;
  drained?: boolean;
  /** Share of the season composite's area lost on the latest clear scene (0.41 = fell 41%). */
  drop_fraction?: number | null;
  status?: "ok" | "no_clear_scene" | "frozen_or_snow" | "not_found" | string;
  latest_day?: string | null;
  latest?: { day?: string | null } | string | null;
}

export interface IndexRisk {
  as_of_season: number;
  data_until: string;
  score: number | null;
  level: RiskLevel;
  volume_m3: number | null;
  peak_discharge_m3s: number | null;
}

export interface ImpactBrief {
  name: string | null;
  name_hi: string | null;
  kind: ImpactKind;
  subkind: string;
  osm: string;
  km: number | null;
  arrival_min_expected: number | null;
  arrival_min_fast: number | null;
  status: ImpactStatus;
}

export interface IndexDownstream {
  path_km: number | null;
  peak_m3s: { expected: number | null; severe: number | null } | null;
  first_exposed: ImpactBrief | null;
  first_settlement: ImpactBrief | null;
  exposed_counts: Partial<Record<ImpactKind, { in_flood_path: number; at_risk: number }>> | null;
  exposure_gaps_km: [number, number][] | number[][] | null;
}

export interface Lake {
  id: string;
  name: string;
  name_hi: string;
  lat: number;
  lon: number;
  district: string;
  basin: string;
  kind: string;
  notes?: string;
  sources: string[];
  years: YearRecord[];
  first: YearRecord | null;
  latest: YearRecord | null;
  risk: IndexRisk | null;
  downstream: IndexDownstream | null;
  drain?: DrainSummary | null;
}

export type WeatherLevel = "normal" | "elevated" | "high";

/** One row of GET /weather. */
export interface WeatherBrief {
  lake_id: string;
  level: WeatherLevel;
  reasons: string[];
  fetched_at: string;
  stale: boolean;
}

export interface WeatherDay {
  date: string;
  precipitation_mm: number | null;
  snowfall_cm: number | null;
  tmax_c: number | null;
  tmin_c: number | null;
  forecast: boolean;
}

/** GET /lakes/{id}/weather. */
export interface WeatherOutlook {
  lake_id: string;
  fetched_at: string;
  stale: boolean;
  source: string;
  days: WeatherDay[];
  trigger: { level: WeatherLevel; reasons: string[]; rule: string };
}

export interface BarrierCandidate {
  lat: number;
  lon: number;
  area_m2: number | null;
  /** Distance along the river below the lake. The pipeline's own files call it km_along_reach. */
  km?: number | null;
  km_along_reach?: number | null;
  /** Absent in older scans: counts as a possible barrier lake. */
  kind?: "possible_barrier_lake" | "reservoir_level_change" | string;
  near_dam_km?: number | null;
}

/** GET /lakes/{id}/barrier: the latest scan of the river below the lake for new water. */
export interface BarrierScan {
  lake_id?: string;
  as_of: string;
  candidates: BarrierCandidate[];
}

export interface LakeIndex {
  generated_at: string;
  lakes: Lake[];
}

export interface Factor {
  key: string;
  group: "size" | "likelihood" | string;
  label: string;
  label_hi: string;
  value: number | null;
  unit: string;
  score: number | null;
  weight: number;
  rule: string;
  why: string;
  source: string;
  note: string | null;
}

export interface RiskRecord {
  lake_id: string;
  as_of_season: number;
  data_until: string;
  score: number | null;
  level: RiskLevel;
  size: number | null;
  likelihood: number | null;
  factors: Factor[];
  area_m2: number | null;
  area_year: number | null;
  volume_m3: number | null;
  peak_discharge_m3s: number | null;
  terrain: Record<string, unknown> | null;
}

export interface RiskMethod {
  formula: string;
  levels: Record<RiskLevel, number>;
  hindsight: string;
  disclaimer: string;
}

export interface RiskFile {
  lake_id: string;
  method: RiskMethod;
  latest: RiskRecord;
  replay: RiskRecord[];
}

export interface DischargeScenario {
  peak_m3s: number | null;
  relation: string;
  source: string;
  note?: string;
}

export interface Station {
  km: number;
  lon: number;
  lat: number;
  bed_m: number;
  slope: number;
  discharge_m3s: number;
  wse_m: number;
  depth_m: number;
  width_m: number;
  arrival_min_expected: number;
  arrival_min_fast: number;
}

export interface DownstreamFile {
  lake_id: string;
  as_of_season: number;
  volume_m3: number | null;
  discharge: {
    scenarios: Record<ScenarioName, DischargeScenario>;
    all: Record<string, number>;
  };
  path_km: number | null;
  exposure_gaps_km: number[][] | null;
  caveats: string[];
  scenarios?: Record<ScenarioName, Station[]>;
}

export interface Outcome {
  status: "flooded" | "margin" | "outside";
  height_above_flood_m: number | null;
  flood_depth_m: number | null;
  road_km_flooded: number | null;
}

export interface Impact {
  name: string | null;
  name_hi: string | null;
  kind: ImpactKind;
  subkind: string;
  osm: string;
  lon: number;
  lat: number;
  km: number | null;
  lateral_m: number | null;
  arrival_min_expected: number | null;
  arrival_min_fast: number | null;
  status: ImpactStatus;
  scenarios: { expected: Outcome | null; severe: Outcome | null } | null;
}

export interface Delivery {
  /** "dry_run": planned and logged, deliberately not sent. */
  status: "pending" | "sent" | "failed" | "dry_run";
  error?: string;
  at?: string;
}

export interface Recipient {
  subscription_id: string;
  phone: string;
  lang: string;
  channel: string;
  place_osm: string;
  place_name: string;
  arrival_min_fast: number | null;
  arrival_min_expected: number | null;
  status: ImpactStatus;
  message: string;
  /** SMS-length version of the message (newer API). */
  short_message?: string;
  /** Spoken message, relative to the API root, e.g. "/audio/<hash>.mp3". */
  audio_url?: string;
  delivery: Delivery;
}

export interface AffectedPlace {
  osm: string;
  name: string | null;
  name_hi: string | null;
  kind: ImpactKind;
  subkind: string;
  lon: number;
  lat: number;
  km: number | null;
  arrival_min_fast: number | null;
  arrival_min_expected: number | null;
  status: ImpactStatus;
  scenario_status: string;
  flood_depth_m: number | null;
  height_above_flood_m: number | null;
  subscribers: number;
}

export interface AlertEvent {
  event_id: string;
  lake_id: string;
  lake_name: string;
  lake_name_hi: string;
  scenario: ScenarioName;
  source: "simulation" | "sensor" | "satellite_drain_check" | string;
  /** True when the trigger asked for a dry run (newer API). */
  dry_run?: boolean;
  note?: string;
  sensor?: { sensor_id: string; kind: string; value: number | null; observed_at: string };
  created_at: string;
  recipients: Recipient[];
  affected_places: AffectedPlace[];
  summary: { recipients: number; sent: number; failed: number };
}

// GeoJSON, as much as the dashboard needs.
export interface GeoFeature {
  type: "Feature";
  geometry: { type: string; coordinates: unknown } | null;
  properties: Record<string, unknown> | null;
}
export interface GeoCollection {
  type: "FeatureCollection";
  features: GeoFeature[];
}
export type GeoJSONDoc = GeoFeature | GeoCollection;

export const LAYER_NAMES = [
  "outlines",
  "outlet",
  "glaciers",
  "flood_path",
  "corridor_expected",
  "corridor_severe",
] as const;
export type LayerName = (typeof LAYER_NAMES)[number];
export type LakeLayers = Partial<Record<LayerName, GeoJSONDoc>>;
