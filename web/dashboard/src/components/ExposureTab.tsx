import { useMemo, useState } from "preact/hooks";
import { arrival, dateOnly, discharge, DASH, KIND_LABEL, kmRange, num, STATUS_LABEL, subkind, unnamedLabel, volume } from "../format";
import type { BarrierCandidate, BarrierScan, DownstreamFile, Impact, ImpactKind, Lake, ScenarioName } from "../types";
import { MoreToggle, Section, SourceText, StatusBadge } from "./ui";

const KINDS: ImpactKind[] = ["settlement", "school", "health", "bridge", "road", "hydro"];

function pair(a: number | null | undefined, b: number | null | undefined, signed = false): string {
  const f = (v: number | null | undefined) => {
    if (v === null || v === undefined) return DASH;
    const s = num(Math.abs(v), 1);
    return signed ? (v > 0 ? "+" : v < 0 ? "−" : "") + s : num(v, 1);
  };
  if ((a === null || a === undefined) && (b === null || b === undefined)) return DASH;
  // The zero-width space after the slash lets the pair wrap onto two lines in a narrow panel.
  return `${f(a)} /\u200b${f(b)}`;
}

/** One table row: an asset, or a run of neighbouring assets that read the same. */
interface Row {
  first: Impact;
  n: number;
  kmMax: number | null;
  fast: number | null;
  expected: number | null;
}

/** Rows within this distance of the first one of a run are folded into it. */
const MERGE_KM = 1;

/**
 * Folds consecutive assets with the same name, kind, sub-kind and status that
 * lie within MERGE_KM of the run's first one (OpenStreetMap splits one road or
 * bridge into several pieces) into one row. No value is changed: the row shows
 * the km range, the earliest fast and the latest expected arrival, and a count.
 */
function mergeRows(items: Impact[]): Row[] {
  const out: Row[] = [];
  for (const im of items) {
    const g = out[out.length - 1];
    const a = g?.first;
    if (
      a &&
      a.kind === im.kind &&
      a.status === im.status &&
      (a.name ?? null) === (im.name ?? null) &&
      (a.name_hi ?? null) === (im.name_hi ?? null) &&
      (a.subkind ?? "") === (im.subkind ?? "") &&
      a.km !== null &&
      im.km !== null &&
      Math.abs(im.km - a.km) <= MERGE_KM
    ) {
      g.n++;
      g.kmMax = Math.max(g.kmMax ?? im.km, im.km);
      if (im.arrival_min_fast !== null) g.fast = g.fast === null ? im.arrival_min_fast : Math.min(g.fast, im.arrival_min_fast);
      if (im.arrival_min_expected !== null)
        g.expected = g.expected === null ? im.arrival_min_expected : Math.max(g.expected, im.arrival_min_expected);
    } else {
      out.push({ first: im, n: 1, kmMax: im.km, fast: im.arrival_min_fast, expected: im.arrival_min_expected });
    }
  }
  return out;
}

/** New water on the river below the lake: a landslide dam would show up here first. */
function BarrierSection({ scan, onFocusPoint }: { scan: BarrierScan; onFocusPoint: (lon: number, lat: number) => void }) {
  const all = Array.isArray(scan.candidates) ? scan.candidates : [];
  const kmOf = (c: BarrierCandidate) => c.km ?? c.km_along_reach ?? null;
  const sorted = [...all].sort((a, b) => (kmOf(a) ?? Infinity) - (kmOf(b) ?? Infinity));
  const lakes = sorted.filter((c) => c.kind !== "reservoir_level_change");
  const reservoirs = sorted.filter((c) => c.kind === "reservoir_level_change");
  const where = (c: BarrierCandidate) =>
    Number.isFinite(c.lat) && Number.isFinite(c.lon) ? (
      <button type="button" class="link-btn" onClick={() => onFocusPoint(c.lon, c.lat)} title="Show on map">
        {c.lat.toFixed(4)}, {c.lon.toFixed(4)}
      </button>
    ) : null;
  const facts = (c: BarrierCandidate) => (
    <>
      {num(c.area_m2)} m² · {num(kmOf(c), 1)} km downstream
    </>
  );
  return (
    <Section title="River blockage scan" aside={<span class="muted">scan of {dateOnly(scan.as_of)}</span>}>
      {lakes.length === 0 && (
        <p class="muted">
          {all.length === 0 ? "No new water on the river" : "No possible barrier lake on the river"} as of{" "}
          {dateOnly(scan.as_of)}.
        </p>
      )}
      {(lakes.length > 0 || reservoirs.length > 0) && (
        <ul class="barrier-list">
          {lakes.map((c, i) => (
            <li key={`b${i}`} class="barrier-hit">
              <span class="badge status-in_flood_path">
                <span class="badge-dot" aria-hidden="true" />
                Possible barrier lake
              </span>
              <span class="barrier-facts">{facts(c)}</span>
              {where(c)}
            </li>
          ))}
          {reservoirs.map((c, i) => (
            <li key={`r${i}`} class="barrier-res">
              <span>
                Reservoir level change
                {typeof c.near_dam_km === "number" ? ` (dam ${num(c.near_dam_km, 1)} km away)` : ""} — not a blockage
              </span>
              <span class="barrier-facts">{facts(c)}</span>
              {where(c)}
            </li>
          ))}
        </ul>
      )}
      {lakes.length > 0 && (
        <p class="caption">
          New water where the river was dry in earlier imagery. A screening result from Sentinel-2: verify on the
          ground or with fresh imagery.
        </p>
      )}
    </Section>
  );
}

export function ExposureTab({
  lake,
  downstream,
  impacts,
  barrier,
  onFocus,
  onFocusPoint,
}: {
  lake: Lake;
  downstream: DownstreamFile | null;
  impacts: Impact[];
  barrier?: BarrierScan | null;
  onFocus: (im: Impact) => void;
  onFocusPoint: (lon: number, lat: number) => void;
}) {
  const [kinds, setKinds] = useState<Set<ImpactKind>>(new Set());
  const [more, setMore] = useState(false);
  const rows = useMemo(
    () =>
      impacts
        .filter((im) => im.status !== "outside")
        .map((im, i) => ({ im, i }))
        .sort((a, b) => (a.im.km ?? Infinity) - (b.im.km ?? Infinity) || a.i - b.i)
        .map((r) => r.im),
    [impacts],
  );
  const byKind = useMemo(() => {
    const m = new Map<ImpactKind, number>();
    for (const r of rows) m.set(r.kind, (m.get(r.kind) ?? 0) + 1);
    return m;
  }, [rows]);
  const shown = kinds.size ? rows.filter((r) => kinds.has(r.kind)) : rows;
  // "More columns" lists every asset on its own row, with its own depth figures.
  const tableRows = useMemo(
    () => (more ? shown.map((im): Row => ({ first: im, n: 1, kmMax: im.km, fast: im.arrival_min_fast, expected: im.arrival_min_expected })) : mergeRows(shown)),
    [rows, kinds, more],
  );
  const merged = tableRows.some((r) => r.n > 1);
  const toggle = (k: ImpactKind) => {
    const n = new Set(kinds);
    if (n.has(k)) n.delete(k);
    else n.add(k);
    setKinds(n);
  };

  const ds = lake.downstream;
  const counts = ds?.exposed_counts ?? {};
  const countKinds = KINDS.filter((k) => counts[k]);
  const gaps = downstream?.exposure_gaps_km ?? ds?.exposure_gaps_km ?? [];
  const scen = downstream?.discharge.scenarios;

  return (
    <div class="tab-body">
      <Section title="Flood scenarios" aside={<span class="muted">{num(downstream?.path_km ?? ds?.path_km, 1)} km modelled</span>}>
        {scen ? (
          <div class="scenario-cards">
            {(["expected", "severe"] as ScenarioName[]).map((s) => {
              const d = scen[s];
              if (!d) return null;
              return (
                <div key={s} class={`scenario-card ${s}`}>
                  <div class="scenario-name">{s === "expected" ? "Expected" : "Severe"}</div>
                  <div class="scenario-peak">{discharge(d.peak_m3s)}</div>
                  <details class="more">
                    <summary>Relation · source</summary>
                    <p class="formula">{d.relation}</p>
                    {d.note && <p>{d.note}</p>}
                    <p>
                      <SourceText text={d.source} />
                    </p>
                  </details>
                </div>
              );
            })}
          </div>
        ) : (
          <p class="muted">Discharge scenarios not available.</p>
        )}
        {downstream?.volume_m3 != null && (
          <p class="caption">
            Peak flow at the lake, from volume {volume(downstream.volume_m3)} (season {downstream.as_of_season}).
          </p>
        )}
      </Section>

      <Section title="Exposed assets">
        {countKinds.length > 0 ? (
          <table class="table compact counts">
            <thead>
              <tr>
                <th />
                {countKinds.map((k) => (
                  <th key={k} class="num">
                    {KIND_LABEL[k]}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {(["in_flood_path", "at_risk"] as const).map((st) => (
                <tr key={st}>
                  <td>
                    <StatusBadge status={st} />
                  </td>
                  {countKinds.map((k) => (
                    <td key={k} class="num">
                      {num(counts[k]?.[st])}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p class="muted">No exposed assets found along the modelled path.</p>
        )}
        {(ds?.first_settlement || ds?.first_exposed) && (
          <ul class="firsts">
            {ds?.first_exposed && (
              <li>
                <span class="k">First asset</span> {ds.first_exposed.name ?? KIND_LABEL[ds.first_exposed.kind]} (
                {KIND_LABEL[ds.first_exposed.kind]}) at {num(ds.first_exposed.km, 1)} km,{" "}
                {arrival(ds.first_exposed.arrival_min_fast, ds.first_exposed.arrival_min_expected)}
              </li>
            )}
            {ds?.first_settlement && (
              <li>
                <span class="k">First settlement</span> {ds.first_settlement.name ?? unnamedLabel("settlement")}
                {ds.first_settlement.name_hi ? ` (${ds.first_settlement.name_hi})` : ""} at{" "}
                {num(ds.first_settlement.km, 1)} km,{" "}
                {arrival(ds.first_settlement.arrival_min_fast, ds.first_settlement.arrival_min_expected)}
              </li>
            )}
          </ul>
        )}
        {gaps.length > 0 && (
          <p class="note">
            OpenStreetMap could not be read for{" "}
            {gaps.map((g, i) => (
              <span key={i}>
                {i ? ", " : ""}
                {num(g[0], 0)}–{num(g[1], 0)} km
              </span>
            ))}
            : assets there are missing from this list.
          </p>
        )}
      </Section>

      <Section
        title="Nearest first"
        aside={
          <span class="aside-row">
            <span class="muted">
              {shown.length} of {rows.length}
            </span>
            <MoreToggle on={more} onToggle={() => setMore(!more)} />
          </span>
        }
      >
        <div class="chips" role="group" aria-label="Filter by kind">
          <button type="button" class={`chip${kinds.size === 0 ? " on" : ""}`} onClick={() => setKinds(new Set())}>
            All
          </button>
          {KINDS.filter((k) => byKind.get(k)).map((k) => (
            <button key={k} type="button" class={`chip${kinds.has(k) ? " on" : ""}`} onClick={() => toggle(k)} aria-pressed={kinds.has(k)}>
              {KIND_LABEL[k]} <span class="chip-n">{byKind.get(k)}</span>
            </button>
          ))}
        </div>
        <div class="table-scroll">
          <table class={`table exposure${more ? " more" : ""}`}>
            <colgroup>
              <col class="c-km" />
              <col class="c-name" />
              <col class="c-status" />
              <col class="c-arr" />
              {more && <col class="c-depth" />}
              {more && <col class="c-above" />}
            </colgroup>
            <thead>
              <tr>
                <th class="num" title="Distance along the flood path from the lake">
                  km
                </th>
                <th>Name · kind</th>
                <th>Status</th>
                <th class="num" title="Fast to expected flood-front arrival, minutes after the burst">
                  Arrival<br />
                  <span class="th-sub">min</span>
                </th>
                {more && (
                  <th class="num" title="Flood depth at the river, expected / severe">
                    Depth m<br />
                    <span class="th-sub">exp / sev</span>
                  </th>
                )}
                {more && (
                  <th class="num" title="Height of the asset above the flood level (negative: under water), expected / severe">
                    Above flood m<br />
                    <span class="th-sub">exp / sev</span>
                  </th>
                )}
              </tr>
            </thead>
            <tbody>
              {tableRows.map((row, i) => {
                const im = row.first;
                const e = im.scenarios?.expected;
                const s = im.scenarios?.severe;
                return (
                  <tr
                    key={`${im.osm}-${i}`}
                    class="clickable"
                    onClick={() => onFocus(im)}
                    title={row.n > 1 ? `${row.n} mapped pieces, ${kmRange(im.km, row.kmMax).replace("\u200b", "")} km. Show the nearest on the map` : "Show on map"}
                  >
                    <td class="num">{row.n > 1 ? kmRange(im.km, row.kmMax) : num(im.km, 1)}</td>
                    <td>
                      <span class="cell-name">{im.name ?? <span class="muted">{unnamedLabel(im.kind)}</span>}</span>
                      {row.n > 1 && <span class="muted small"> ×{row.n}</span>}
                      {im.name_hi && (
                        <span class="hi block" lang="hi">
                          {im.name_hi}
                        </span>
                      )}
                      <span class="muted block small cell-kind">
                        {KIND_LABEL[im.kind] ?? im.kind}
                        {im.subkind && im.subkind !== im.kind ? ` · ${subkind(im.subkind)}` : ""}
                      </span>
                    </td>
                    <td>
                      <StatusBadge status={im.status} short />
                    </td>
                    <td class="num nowrap">
                      {row.fast === null && row.expected === null
                        ? DASH
                        : arrival(row.fast, row.expected).replace(" min", "").replace("~", "")}
                    </td>
                    {more && <td class="num pair">{pair(e?.flood_depth_m, s?.flood_depth_m)}</td>}
                    {more && <td class="num pair">{pair(e?.height_above_flood_m, s?.height_above_flood_m, true)}</td>}
                  </tr>
                );
              })}
              {shown.length === 0 && (
                <tr>
                  <td colSpan={more ? 6 : 4} class="muted">
                    Nothing {kinds.size ? "of this kind " : ""}in the flood path or at risk.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        <p class="caption">
          {STATUS_LABEL.in_flood_path}: flooded in the expected scenario. {STATUS_LABEL.at_risk}: flooded only in the
          severe scenario, or just above it. Click a row to show it on the map.
          {merged && " ×N: N neighbouring map pieces with the same name, kind and status, shown as one row with their km range; More columns lists each one."}
        </p>
      </Section>

      {barrier && barrier.as_of && <BarrierSection scan={barrier} onFocusPoint={onFocusPoint} />}

      {downstream?.caveats?.length ? (
        <details class="more more-section">
          <summary>Caveats ({downstream.caveats.length})</summary>
          <ul class="caveats">
            {downstream.caveats.map((c, i) => (
              <li key={i}>{c}</li>
            ))}
          </ul>
        </details>
      ) : null}
    </div>
  );
}
