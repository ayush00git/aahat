import { Fragment } from "preact";
import { useState } from "preact/hooks";
import { dateOnly, DASH, km2, num, YEAR_STATUS_LABEL } from "../format";
import type { DrainSummary, GeoJSONDoc, Lake, RiskFile, YearRecord, YearStatus } from "../types";
import { Section } from "./ui";

/** Where the method is written up (README section on lake mapping). */
const METHOD_URL = "https://github.com/ayush00git/aahat#how-lake-mapping-works";
const EARTH_SEARCH_URL = "https://registry.opendata.aws/sentinel-2-l2a-cogs/";
/** The STAC item of one scene (the catalogue the pipeline reads, pipeline/src/aahat/stac.py). */
const stacItemUrl = (id: string) =>
  `https://earth-search.aws.element84.com/v1/collections/sentinel-2-l2a/items/${encodeURIComponent(id)}`;

function strList(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string" && x !== "") : [];
}

const DRAIN_STATUS: Record<string, string> = {
  ok: "lake clearly seen",
  no_clear_scene: "no clear scene (cloud or no pass)",
  frozen_or_snow: "lake frozen or under snow, not assessed",
  not_found: "no full season outline to compare against",
  too_small: "lake too small for a single-scene check",
};

/** "Last satellite check" from the index's drain summary; null when no check was run. */
function drainLine(d: DrainSummary | null | undefined): string | null {
  if (!d || (!d.as_of && !d.status)) return null;
  const latest = d.latest_day ?? (typeof d.latest === "string" ? d.latest : d.latest?.day) ?? null;
  const parts = [dateOnly(d.as_of)];
  if (latest) parts.push(`latest scene ${dateOnly(latest)}`);
  if (d.status) parts.push(DRAIN_STATUS[d.status] ?? d.status);
  if (typeof d.drop_fraction === "number") {
    const pct = num(Math.abs(d.drop_fraction) * 100, 0);
    parts.push(
      d.drained
        ? `possible sudden drainage: area fell ${pct}% since the season composite`
        : `no sudden loss of water (area ${d.drop_fraction > 0 ? "−" : "+"}${pct}% against the season composite)`,
    );
  } else if (d.drained) {
    parts.push("possible sudden drainage");
  }
  return parts.join(" · ");
}

/** The per-year record as the outlines layer carries it (a superset of the index's YearRecord). */
interface YearEvidence extends YearRecord {
  scenes_found: number | null;
  first_day: string | null;
  last_day: string | null;
  params: Record<string, unknown> | null;
  /** Scenes behind the outline, as { day, id } pairs (either may be missing). */
  scenes: { day: string | null; id: string | null }[];
}

function str(v: unknown): string | null {
  return typeof v === "string" && v ? v : null;
}
function numOrNull(v: unknown): number | null {
  return typeof v === "number" && Number.isFinite(v) ? v : null;
}

/** Joins the index's yearly records with the outline features' properties (dates, scenes found). */
function evidenceRows(lake: Lake, outlines: GeoJSONDoc | undefined): YearEvidence[] {
  const props = new Map<number, Record<string, unknown>>();
  if (outlines) {
    const feats = outlines.type === "FeatureCollection" ? outlines.features : [outlines];
    for (const f of feats) {
      const y = Number(f.properties?.year);
      if (Number.isFinite(y) && f.properties) props.set(y, f.properties);
    }
  }
  const years = new Set<number>([...lake.years.map((y) => y.year), ...props.keys()]);
  return [...years]
    .sort((a, b) => b - a)
    .map((year) => {
      const rec = lake.years.find((y) => y.year === year);
      const p = props.get(year) ?? {};
      const ids = strList(rec?.scene_ids).length ? strList(rec?.scene_ids) : strList(p.scene_ids);
      const days = strList(rec?.scene_days).length ? strList(rec?.scene_days) : strList(p.scene_days);
      const scenes = Array.from({ length: Math.max(ids.length, days.length) }, (_, i) => ({
        day: days[i] ?? null,
        id: ids[i] ?? null,
      }));
      return {
        scenes,
        year,
        status: (rec?.status ?? (str(p.status) as YearStatus | null) ?? "no_data") as YearStatus,
        area_m2: rec ? rec.area_m2 : numOrNull(p.area_m2),
        uncertainty_m2: rec ? rec.uncertainty_m2 : numOrNull(p.uncertainty_m2),
        coverage: rec ? rec.coverage : numOrNull(p.coverage),
        scenes_clear: rec ? rec.scenes_clear : numOrNull(p.scenes_clear),
        scenes_found: numOrNull(p.scenes_found),
        first_day: str(p.first_day),
        last_day: str(p.last_day),
        params: p.params && typeof p.params === "object" ? (p.params as Record<string, unknown>) : null,
      };
    });
}

/** "16 Aug – 5 Oct" from two ISO days of the same season (the year is in its own column). */
function dayRange(first: string | null, last: string | null): string {
  const d = (iso: string) => {
    const t = new Date(iso + "T00:00:00Z");
    if (Number.isNaN(t.getTime())) return iso;
    return t.toLocaleDateString("en-IN", { day: "numeric", month: "short", timeZone: "UTC" });
  };
  if (!first && !last) return DASH;
  if (!first || !last || first === last) return d((first ?? last) as string);
  return `${d(first)} – ${d(last)}`;
}

/**
 * Copernicus Browser at the lake, true colour Sentinel-2 L2A, limited to the
 * season's scene window when the record has one. No account is needed to view.
 */
export function imageryUrl(lat: number, lon: number, first?: string | null, last?: string | null): string {
  const q = new URLSearchParams({ lat: String(lat), lng: String(lon), zoom: "14" });
  if (first && last) {
    q.set("themeId", "DEFAULT-THEME");
    q.set("datasetId", "S2_L2A_CDAS");
    q.set("layerId", "1_TRUE_COLOR");
    q.set("fromTime", `${first}T00:00:00.000Z`);
    q.set("toTime", `${last}T23:59:59.999Z`);
  }
  return `https://browser.dataspace.copernicus.eu/?${q.toString()}`;
}

const STATUS_PILL: Record<YearStatus, string> = {
  ok: "Full",
  partial: "Partial",
  not_found: "Not found",
  no_data: "No data",
};

function YearPill({ status }: { status: YearStatus }) {
  return (
    <span class={`badge ystat ystat-${status}`} title={YEAR_STATUS_LABEL[status] ?? status}>
      <span class="badge-dot" aria-hidden="true" />
      {STATUS_PILL[status] ?? status}
    </span>
  );
}

function paramText(v: unknown): string {
  if (v === null || v === undefined) return "off";
  if (typeof v === "number") return String(v);
  return String(v);
}

export function EvidenceTab({
  lake,
  risk,
  outlines,
  layersError,
}: {
  lake: Lake;
  risk: RiskFile | null;
  outlines: GeoJSONDoc | undefined;
  layersError?: string;
}) {
  const rows = evidenceRows(lake, outlines);
  const dataUntil = risk?.latest.data_until ?? lake.risk?.data_until;
  const asOf = risk?.latest.as_of_season ?? lake.risk?.as_of_season;
  const params = rows.find((r) => r.params)?.params ?? null;
  const haveDates = rows.some((r) => r.first_day || r.last_day);
  const [openYears, setOpenYears] = useState<Set<number>>(new Set());
  const toggleYear = (y: number) => {
    const n = new Set(openYears);
    if (n.has(y)) n.delete(y);
    else n.add(y);
    setOpenYears(n);
  };
  const drain = drainLine(lake.drain);

  return (
    <div class="tab-body">
      <dl class="kv kv-3">
        <div>
          <dt>Seasons measured</dt>
          <dd>
            {rows.length ? `${rows[rows.length - 1].year}–${rows[0].year}` : DASH}
          </dd>
        </div>
        <div>
          <dt>Risk scored as of</dt>
          <dd>{asOf ?? DASH}</dd>
        </div>
        <div>
          <dt>Data until</dt>
          <dd>{dateOnly(dataUntil)}</dd>
        </div>
      </dl>
      {drain && (
        <p class={`drain-line${lake.drain?.drained ? " drained" : ""}`}>
          <span class="k">Last satellite check</span> {drain}
        </p>
      )}

      <Section
        title="By season"
        aside={
          <a
            href={imageryUrl(lake.lat, lake.lon)}
            target="_blank"
            rel="noopener noreferrer"
            title="Open Copernicus Browser at this lake"
          >
            Open in Copernicus Browser ↗
          </a>
        }
      >
        <div class="table-scroll">
          <table class="table evidence">
            <colgroup>
              <col class="c-year" />
              <col class="c-status" />
              <col class="c-area" />
              <col class="c-cov" />
              <col class="c-scenes" />
              <col class="c-dates" />
            </colgroup>
            <thead>
              <tr>
                <th>Year</th>
                <th>Status</th>
                <th class="num" title="Lake area ± measurement uncertainty">
                  Area km²
                  <br />
                  <span class="th-sub">± uncert.</span>
                </th>
                <th class="num" title="Share of the lake's search area seen clear of cloud and snow">
                  Cover
                </th>
                <th class="num" title="Clear scenes used for the outline / scenes found in the season window">
                  Scenes
                  <br />
                  <span class="th-sub">used / found</span>
                </th>
                <th title="First and last day of the scenes used; the link opens that imagery">
                  Scene dates
                  <br />
                  <span class="th-sub">and imagery</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <Fragment key={r.year}>
                <tr class={openYears.has(r.year) ? "scenes-open" : undefined}>
                  <td>{r.year}</td>
                  <td>
                    <YearPill status={r.status} />
                  </td>
                  <td class="num">
                    {r.area_m2 !== null ? km2(r.area_m2).replace(" km²", "") : DASH}
                    {r.uncertainty_m2 !== null && (
                      <span class="muted block">± {km2(r.uncertainty_m2).replace(" km²", "")}</span>
                    )}
                  </td>
                  <td class="num">{r.coverage !== null ? `${num(r.coverage * 100, 0)}%` : DASH}</td>
                  <td class="num">
                    {num(r.scenes_clear)}
                    <span class="muted"> / {num(r.scenes_found)}</span>
                  </td>
                  <td>
                    <span class="block">{dayRange(r.first_day, r.last_day)}</span>
                    <a
                      class="img-link"
                      href={imageryUrl(lake.lat, lake.lon, r.first_day, r.last_day)}
                      target="_blank"
                      rel="noopener noreferrer"
                      title={`Sentinel-2 imagery of ${lake.name}${
                        r.first_day && r.last_day ? `, ${r.first_day} to ${r.last_day}` : ""
                      } (Copernicus Browser)`}
                    >
                      View imagery ↗
                    </a>
                    {r.scenes.length > 0 && (
                      <button
                        type="button"
                        class="link-btn block scene-toggle"
                        aria-expanded={openYears.has(r.year)}
                        onClick={() => toggleYear(r.year)}
                        title={`The Sentinel-2 scenes behind the ${r.year} outline`}
                      >
                        {openYears.has(r.year) ? "Hide scenes" : `Scenes (${r.scenes.length})`}
                      </button>
                    )}
                  </td>
                </tr>
                {openYears.has(r.year) && r.scenes.length > 0 && (
                  <tr class="scene-row">
                    <td colSpan={6}>
                      <ul class="scene-list">
                        {r.scenes.map((s, i) => (
                          <li key={i}>
                            {s.day && <span>{dateOnly(s.day)}</span>}
                            {s.id && (
                              <a href={stacItemUrl(s.id)} target="_blank" rel="noopener noreferrer" title="STAC item (Earth Search)">
                                {s.id}
                              </a>
                            )}
                          </li>
                        ))}
                      </ul>
                    </td>
                  </tr>
                )}
                </Fragment>
              ))}
              {rows.length === 0 && (
                <tr>
                  <td colSpan={6} class="muted">
                    No seasons measured yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        {!haveDates && (
          <p class="note">
            Scene dates and scenes found are not available
            {layersError ? " (the outline layer failed to load)" : ""}; the other columns come from the lake index.
          </p>
        )}
        <details class="more">
          <summary>What the columns mean</summary>
          <p>
            Status: <b>Full</b> coverage, <b>Partial</b> (cloud or snow hid part of the lake; not used for the trend),{" "}
            <b>Not found</b> (no lake outline at the seed point) or <b>No data</b> (no clear scenes). Cover: share of the
            lake and its rim seen clearly at least twice. Scenes: clear scenes used out of those found in the season
            window. <b>View imagery</b> opens Copernicus Browser at the lake, true colour, limited to that season's scene
            dates where known (free; no account needed to view).
          </p>
        </details>
      </Section>

      <Section title="Sources and method">
        <ul class="evidence-notes">
          <li>
            <b>Imagery.</b> Sentinel-2 Level-2A surface reflectance (Copernicus), read from{" "}
            <a href={EARTH_SEARCH_URL} target="_blank" rel="noopener noreferrer">
              AWS Open Data
            </a>{" "}
            through the Earth Search STAC catalogue. Post-monsoon scenes (15 Aug to 31 Oct), one outline per season.
          </li>
          <li>
            <b>Method.</b> Cloud, snow and terrain-shadow pixels dropped per scene; water where NDWI
            &gt; 0.3 in at least half of the clear looks; the water body at the lake's seed point is the outline. ± is
            a half-pixel shoreline (perimeter × 5 m). Coverage under 90% or fewer than two clear scenes marks a year
            partial.{" "}
            <a href={METHOD_URL} target="_blank" rel="noopener noreferrer">
              How lake mapping works (project README)
            </a>
            .
          </li>
        </ul>
        {params && (
          <details class="more">
            <summary>Water-mask parameters</summary>
            <p class="formula params">
              {Object.entries(params)
                .map(([k, v]) => `${k}=${paramText(v)}`)
                .join(" · ")}
            </p>
          </details>
        )}
      </Section>
    </div>
  );
}
