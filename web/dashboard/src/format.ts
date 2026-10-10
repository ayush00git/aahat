// Display formatting only: every number shown comes from the API; these
// helpers change units and rounding, never the value.

import type { ImpactKind, ImpactStatus, RiskLevel, WeatherLevel, YearStatus } from "./types";

// One digit grouping everywhere: international (1,234,567), never the Indian
// lakh grouping (12,34,567). Dates below keep the day-month-year order.
const nf = (digits: number) =>
  new Intl.NumberFormat("en-US", { minimumFractionDigits: digits, maximumFractionDigits: digits });

export const DASH = "—";

export function num(v: number | null | undefined, digits = 0): string {
  if (v === null || v === undefined || !Number.isFinite(v)) return DASH;
  return nf(digits).format(v);
}

export function km2(m2: number | null | undefined, digits = 3): string {
  if (m2 === null || m2 === undefined) return DASH;
  // A real lake under half the last digit would print as "0.000 km²": say "<0.001 km²".
  if (m2 > 0 && m2 / 1e6 < 0.5 * 10 ** -digits) return `<${nf(digits).format(10 ** -digits)} km²`;
  return `${nf(digits).format(m2 / 1e6)} km²`;
}

export function volume(m3: number | null | undefined): string {
  if (m3 === null || m3 === undefined) return DASH;
  if (m3 >= 1e6) return `${nf(2).format(m3 / 1e6)} million m³`;
  return `${nf(0).format(m3)} m³`;
}

export function discharge(m3s: number | null | undefined): string {
  if (m3s === null || m3s === undefined) return DASH;
  return `${nf(0).format(m3s)} m³/s`;
}

const RELATION_NAME: Record<string, string> = {
  evans1986: "Evans 1986",
  huggel2002: "Huggel et al. 2002",
  popov1991: "Popov 1991",
};

/**
 * Short citation for a peak-discharge relation, from its key ("evans1986") or
 * from the API's "evans1986: Qmax = 0.72 V^0.53" string. Null when there is none.
 */
export function relationName(relation: string | null | undefined): string | null {
  if (!relation) return null;
  const key = relation.split(":")[0].trim();
  if (RELATION_NAME[key]) return RELATION_NAME[key];
  const m = /^([a-z]+)(\d{4})$/i.exec(key);
  return m ? `${m[1][0].toUpperCase()}${m[1].slice(1)} ${m[2]}` : key || null;
}

/** Whole minutes, rounded down (as the API's warning messages do: never later than modelled). */
export function minutes(v: number | null | undefined): string {
  if (v === null || v === undefined) return DASH;
  return nf(0).format(Math.floor(v));
}

/** "~19–24 min" from fast and expected arrival times. */
export function arrival(fast: number | null | undefined, expected: number | null | undefined): string {
  if ((fast === null || fast === undefined) && (expected === null || expected === undefined)) return DASH;
  const f = minutes(fast);
  const e = minutes(expected);
  if (f === e) return `~${f} min`;
  return `~${f}–${e} min`;
}

export function metres(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined) return DASH;
  return `${nf(digits).format(v)} m`;
}

export function signedMetres(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined) return DASH;
  const s = nf(digits).format(Math.abs(v));
  return `${v > 0 ? "+" : v < 0 ? "−" : ""}${s} m`;
}

/** Factor value with its unit, scaled for readability where the unit allows. */
export function factorValue(value: number | null, unit: string): string {
  if (value === null || value === undefined) return DASH;
  if (unit === "m³") return volume(value);
  const digits = Number.isInteger(value) || Math.abs(value) >= 100 ? 0 : Math.abs(value) >= 10 ? 1 : 2;
  const sep = unit === "°" ? "" : " ";
  return `${nf(digits).format(value)}${sep}${unit}`;
}

export function dateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("en-IN", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

export function dateOnly(iso: string | null | undefined): string {
  if (!iso) return DASH;
  const d = new Date(iso.length === 10 ? iso + "T00:00:00Z" : iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-IN", { day: "2-digit", month: "short", year: "numeric", timeZone: "UTC" });
}

export const LEVEL_LABEL: Record<RiskLevel, string> = {
  low: "Low",
  moderate: "Moderate",
  high: "High",
  very_high: "Very high",
};

export const WEATHER_LABEL: Record<WeatherLevel, string> = {
  normal: "Normal",
  elevated: "Elevated",
  high: "High",
};

export const STATUS_LABEL: Record<ImpactStatus, string> = {
  in_flood_path: "In flood path",
  at_risk: "At risk",
  outside: "Outside",
};

/** Table-width labels for the same statuses. */
export const STATUS_SHORT: Record<ImpactStatus, string> = {
  in_flood_path: "In path",
  at_risk: "At risk",
  outside: "Outside",
};

export const OUTCOME_LABEL: Record<string, string> = {
  flooded: "flooded",
  margin: "margin",
  outside: "outside",
};

export const KIND_LABEL: Record<ImpactKind, string> = {
  settlement: "Settlement",
  bridge: "Bridge",
  road: "Road",
  hydro: "Hydro",
  school: "School",
  health: "Health",
};

const UNNAMED_LABEL: Record<ImpactKind, string> = {
  settlement: "Unnamed settlement",
  bridge: "Unnamed bridge",
  road: "Unnamed road",
  hydro: "Unnamed hydro site",
  school: "Unnamed school",
  health: "Unnamed health facility",
};

/** What to show for an asset OpenStreetMap has no name for. */
export function unnamedLabel(kind: ImpactKind): string {
  return UNNAMED_LABEL[kind] ?? "Unnamed";
}

/** "11.5" or "11.5–12.2" (no unit) for a run of assets along the flood path. */
export function kmRange(lo: number | null | undefined, hi: number | null | undefined, digits = 1): string {
  const a = num(lo, digits);
  const b = num(hi, digits);
  // The zero-width space lets the range wrap in a narrow column.
  return a === b ? a : `${a}–\u200b${b}`;
}

/**
 * The year of `dataUntil` when it lies more than `months` before today, else
 * null: marks lakes whose last clear satellite season is old.
 */
export function staleDataYear(dataUntil: string | null | undefined, months = 18, now = new Date()): number | null {
  if (!dataUntil) return null;
  const d = new Date(dataUntil.length === 10 ? dataUntil + "T00:00:00Z" : dataUntil);
  if (Number.isNaN(d.getTime())) return null;
  const cutoff = new Date(now.getTime());
  cutoff.setUTCMonth(cutoff.getUTCMonth() - months);
  return d < cutoff ? d.getUTCFullYear() : null;
}

export const YEAR_STATUS_LABEL: Record<YearStatus, string> = {
  ok: "full coverage",
  partial: "partial (cloud/snow)",
  not_found: "lake not found",
  no_data: "no clear scenes",
};

export function subkind(s: string | null | undefined): string {
  return s ? s.replace(/_/g, " ") : "";
}

/** Splits text into plain strings and URLs, for rendering source citations. */
export function splitLinks(text: string): { text: string; url?: string }[] {
  const out: { text: string; url?: string }[] = [];
  const re = /https?:\/\/[^\s);,]+/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text))) {
    if (m.index > last) out.push({ text: text.slice(last, m.index) });
    out.push({ text: m[0], url: m[0] });
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push({ text: text.slice(last) });
  return out;
}

export function hostOf(url: string): string {
  try {
    const u = new URL(url);
    if (u.hostname === "doi.org") return "doi:" + u.pathname.slice(1);
    return u.hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}
