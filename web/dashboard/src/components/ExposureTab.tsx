import { useMemo, useState } from "preact/hooks";
import { arrival, discharge, DASH, KIND_LABEL, num, STATUS_LABEL, subkind, volume } from "../format";
import type { DownstreamFile, Impact, ImpactKind, Lake, ScenarioName } from "../types";
import { Section, SourceText, StatusBadge } from "./ui";

const KINDS: ImpactKind[] = ["settlement", "school", "health", "bridge", "road", "hydro"];

function pair(a: number | null | undefined, b: number | null | undefined, signed = false): string {
  const f = (v: number | null | undefined) => {
    if (v === null || v === undefined) return DASH;
    const s = num(Math.abs(v), 1);
    return signed ? (v > 0 ? "+" : v < 0 ? "−" : "") + s : num(v, 1);
  };
  if ((a === null || a === undefined) && (b === null || b === undefined)) return DASH;
  return `${f(a)} / ${f(b)}`;
}

export function ExposureTab({
  lake,
  downstream,
  impacts,
  onFocus,
}: {
  lake: Lake;
  downstream: DownstreamFile | null;
  impacts: Impact[];
  onFocus: (im: Impact) => void;
}) {
  const [kinds, setKinds] = useState<Set<ImpactKind>>(new Set());
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
      <Section title="Flood scenarios" aside={<span class="muted small">path {num(downstream?.path_km ?? ds?.path_km, 1)} km modelled</span>}>
        {scen ? (
          <div class="scenario-cards">
            {(["expected", "severe"] as ScenarioName[]).map((s) => {
              const d = scen[s];
              if (!d) return null;
              return (
                <div key={s} class={`scenario-card ${s}`}>
                  <div class="scenario-name">{s === "expected" ? "Expected" : "Severe"}</div>
                  <div class="scenario-peak">{discharge(d.peak_m3s)}</div>
                  <div class="mono small">{d.relation}</div>
                  {d.note && <div class="small muted">{d.note}</div>}
                  <div class="small source">
                    <SourceText text={d.source} />
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <p class="muted">Discharge scenarios not available.</p>
        )}
        {downstream?.volume_m3 != null && (
          <p class="small muted">
            From lake volume {volume(downstream.volume_m3)} (season {downstream.as_of_season}).
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
          <ul class="firsts small">
            {ds?.first_exposed && (
              <li>
                <span class="k">First asset</span> {ds.first_exposed.name ?? KIND_LABEL[ds.first_exposed.kind]} (
                {KIND_LABEL[ds.first_exposed.kind]}) at {num(ds.first_exposed.km, 1)} km,{" "}
                {arrival(ds.first_exposed.arrival_min_fast, ds.first_exposed.arrival_min_expected)}
              </li>
            )}
            {ds?.first_settlement && (
              <li>
                <span class="k">First settlement</span> {ds.first_settlement.name ?? "unnamed"}
                {ds.first_settlement.name_hi ? ` (${ds.first_settlement.name_hi})` : ""} at{" "}
                {num(ds.first_settlement.km, 1)} km,{" "}
                {arrival(ds.first_settlement.arrival_min_fast, ds.first_settlement.arrival_min_expected)}
              </li>
            )}
          </ul>
        )}
        {gaps.length > 0 && (
          <p class="flag">
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

      <Section title="Downstream exposure, nearest first" aside={<span class="muted small">{shown.length} of {rows.length}</span>}>
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
          <table class="table exposure">
            <thead>
              <tr>
                <th class="num">km</th>
                <th>Name</th>
                <th>Kind</th>
                <th>Status</th>
                <th class="num" title="Fast to expected flood-front arrival, minutes after the burst">
                  Arrival (min)
                </th>
                <th class="num" title="Flood depth at the river, expected / severe">
                  Depth m<br />
                  <span class="th-sub">exp / sev</span>
                </th>
                <th class="num" title="Height of the asset above the flood level (negative: under water), expected / severe">
                  Above flood m<br />
                  <span class="th-sub">exp / sev</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {shown.map((im, i) => {
                const e = im.scenarios?.expected;
                const s = im.scenarios?.severe;
                return (
                  <tr key={`${im.osm}-${i}`} class="clickable" onClick={() => onFocus(im)} title="Show on map">
                    <td class="num">{num(im.km, 1)}</td>
                    <td>
                      <span class="cell-name">{im.name ?? <span class="muted">unnamed</span>}</span>
                      {im.name_hi && (
                        <span class="hi block" lang="hi">
                          {im.name_hi}
                        </span>
                      )}
                    </td>
                    <td>
                      {KIND_LABEL[im.kind] ?? im.kind}
                      {im.subkind && <span class="muted block small">{subkind(im.subkind)}</span>}
                    </td>
                    <td>
                      <StatusBadge status={im.status} />
                    </td>
                    <td class="num nowrap">
                      {im.arrival_min_fast === null && im.arrival_min_expected === null
                        ? DASH
                        : arrival(im.arrival_min_fast, im.arrival_min_expected).replace(" min", "").replace("~", "")}
                    </td>
                    <td class="num nowrap">{pair(e?.flood_depth_m, s?.flood_depth_m)}</td>
                    <td class="num nowrap">{pair(e?.height_above_flood_m, s?.height_above_flood_m, true)}</td>
                  </tr>
                );
              })}
              {shown.length === 0 && (
                <tr>
                  <td colSpan={7} class="muted">
                    Nothing {kinds.size ? "of this kind " : ""}in the flood path or at risk.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        <p class="caption">
          {STATUS_LABEL.in_flood_path}: flooded in the expected scenario. {STATUS_LABEL.at_risk}: flooded only in the
          severe scenario, or only just above the flood level (safety margin). Click a row to show it on the map.
        </p>
      </Section>

      {downstream?.caveats?.length ? (
        <Section title="Caveats">
          <ul class="caveats">
            {downstream.caveats.map((c, i) => (
              <li key={i}>{c}</li>
            ))}
          </ul>
        </Section>
      ) : null}
    </div>
  );
}
