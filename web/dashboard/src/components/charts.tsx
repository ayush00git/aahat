// Small inline-SVG charts. Single series each, so no legend box: the section
// title names the series; status colours only mark risk levels.

import type { ComponentChildren } from "preact";
import { useRef, useState } from "preact/hooks";
import { LEVEL_COLOR, CHART } from "../colors";
import { dateOnly, km2, LEVEL_LABEL, num, YEAR_STATUS_LABEL } from "../format";
import type { RiskLevel, RiskRecord, YearRecord } from "../types";

// ---------- shared ----------

interface Pad {
  t: number;
  r: number;
  b: number;
  l: number;
}

function linear(d0: number, d1: number, r0: number, r1: number) {
  const k = d1 === d0 ? 0 : (r1 - r0) / (d1 - d0);
  return (v: number) => r0 + (v - d0) * k;
}

function niceTicks(lo: number, hi: number, count = 4): number[] {
  const span = hi - lo;
  if (span <= 0) return [lo];
  const raw = span / count;
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => span / s <= count + 1) ?? raw;
  const out: number[] = [];
  for (let v = Math.ceil(lo / step) * step; v <= hi + 1e-9; v += step) out.push(+v.toFixed(10));
  return out;
}

/** Tracks the hovered index by nearest x, in viewBox units. */
function useNearest(W: number, xs: number[]) {
  const ref = useRef<SVGSVGElement>(null);
  const [idx, setIdx] = useState<number | null>(null);
  const [scale, setScale] = useState(1);
  const onMove = (e: PointerEvent) => {
    const svg = ref.current;
    if (!svg || !xs.length) return;
    const rect = svg.getBoundingClientRect();
    const k = W / rect.width;
    const x = (e.clientX - rect.left) * k;
    let best = 0;
    for (let i = 1; i < xs.length; i++) if (Math.abs(xs[i] - x) < Math.abs(xs[best] - x)) best = i;
    setIdx(best);
    setScale(1 / k);
  };
  return { ref, idx, scale, onMove, onLeave: () => setIdx(null) };
}

function Tooltip({ x, y, children, flip }: { x: number; y: number; children: ComponentChildren; flip: boolean }) {
  return (
    <div class={`chart-tip${flip ? " flip" : ""}`} style={{ left: `${x}px`, top: `${y}px` }} role="status">
      {children}
    </div>
  );
}

// ---------- risk replay ----------

const LEVEL_ORDER: RiskLevel[] = ["low", "moderate", "high", "very_high"];

export function ReplayChart({ replay, levels }: { replay: RiskRecord[]; levels: Record<RiskLevel, number> }) {
  const W = 440;
  const H = 180;
  const pad: Pad = { t: 10, r: 74, b: 24, l: 32 };
  const pts = replay.filter((r) => r.score !== null).sort((a, b) => a.as_of_season - b.as_of_season);
  const x0 = pts.length ? pts[0].as_of_season : 0;
  const x1 = pts.length ? pts[pts.length - 1].as_of_season : 1;
  const sx = linear(x0, x1 === x0 ? x0 + 1 : x1, pad.l + 8, W - pad.r - 8);
  const sy = linear(0, 100, H - pad.b, pad.t);
  const xs = pts.map((p) => sx(p.as_of_season));
  const hov = useNearest(W, xs);
  if (!pts.length) return <div class="state-msg">No replay available.</div>;

  const bands = LEVEL_ORDER.map((lv, i) => {
    const lo = levels[lv] ?? 0;
    const hi = i + 1 < LEVEL_ORDER.length ? (levels[LEVEL_ORDER[i + 1]] ?? 100) : 100;
    return { lv, lo, hi };
  });
  const path = pts.map((p, i) => `${i ? "L" : "M"}${xs[i].toFixed(1)},${sy(p.score as number).toFixed(1)}`).join("");
  const h = hov.idx !== null ? pts[hov.idx] : null;

  return (
    <div class="chart">
      <svg
        ref={hov.ref}
        viewBox={`0 0 ${W} ${H}`}
        class="chart-svg"
        role="img"
        aria-label={`Risk score by season, ${x0} to ${x1}`}
        onPointerMove={hov.onMove}
        onPointerLeave={hov.onLeave}
      >
        {bands.map((b) => (
          <g key={b.lv}>
            <rect x={pad.l} y={sy(b.hi)} width={W - pad.l - pad.r} height={sy(b.lo) - sy(b.hi)} fill={LEVEL_COLOR[b.lv]} opacity={0.1} />
            {b.lo > 0 && <line x1={pad.l} x2={W - pad.r} y1={sy(b.lo)} y2={sy(b.lo)} class="grid" />}
            <text x={W - pad.r + 6} y={(sy(b.lo) + sy(b.hi)) / 2 + 3} class="axis-label">
              {LEVEL_LABEL[b.lv]}
            </text>
          </g>
        ))}
        {[0, 20, 40, 60, 80, 100].map((t) => (
          <text key={t} x={pad.l - 6} y={sy(t) + 3} class="axis-label" text-anchor="end">
            {t}
          </text>
        ))}
        {pts.map((p, i) => (
          <text key={p.as_of_season} x={xs[i]} y={H - 6} class="axis-label" text-anchor="middle">
            {pts.length > 8 && i % 2 && i !== pts.length - 1 ? "" : `'${String(p.as_of_season).slice(2)}`}
          </text>
        ))}
        <line x1={pad.l} x2={W - pad.r} y1={H - pad.b} y2={H - pad.b} class="axis" />
        {h && <line x1={xs[hov.idx!]} x2={xs[hov.idx!]} y1={pad.t} y2={H - pad.b} class="crosshair" />}
        <path d={path} fill="none" stroke={CHART.line} stroke-width={2} stroke-linejoin="round" opacity={0.9} />
        {pts.map((p, i) => (
          <circle
            key={p.as_of_season}
            cx={xs[i]}
            cy={sy(p.score as number)}
            r={hov.idx === i ? 5.5 : 4}
            fill={LEVEL_COLOR[p.level]}
            stroke="var(--surface)"
            stroke-width={2}
          />
        ))}
        <rect x={pad.l} y={pad.t} width={W - pad.l - pad.r} height={H - pad.t - pad.b} fill="transparent" />
      </svg>
      {h && (
        <Tooltip x={xs[hov.idx!] * hov.scale} y={sy(h.score as number) * hov.scale} flip={hov.idx! > pts.length / 2}>
          <div class="tip-title">Season {h.as_of_season}</div>
          <div>
            Score <b>{num(h.score, 1)}</b> · {LEVEL_LABEL[h.level]}
          </div>
          <div class="muted">
            = 100 × {num(h.size, 3)} × {num(h.likelihood, 3)}
          </div>
          <div class="muted">data until {dateOnly(h.data_until)}</div>
          {h.area_m2 !== null && (
            <div class="muted">
              area {km2(h.area_m2)} ({h.area_year})
            </div>
          )}
        </Tooltip>
      )}
    </div>
  );
}

// ---------- area by year ----------

export function GrowthChart({ years }: { years: YearRecord[] }) {
  const W = 440;
  const H = 200;
  const pad: Pad = { t: 12, r: 12, b: 24, l: 52 };
  const ys = [...years].sort((a, b) => a.year - b.year);
  const measured = ys.filter((y) => y.area_m2 !== null);
  const x0 = ys.length ? ys[0].year : 0;
  const x1 = ys.length ? ys[ys.length - 1].year : 1;
  const sx = linear(x0, x1 === x0 ? x0 + 1 : x1, pad.l + 10, W - pad.r - 10);
  let lo = Infinity;
  let hi = -Infinity;
  for (const y of measured) {
    const u = y.uncertainty_m2 ?? 0;
    lo = Math.min(lo, (y.area_m2 as number) - u);
    hi = Math.max(hi, (y.area_m2 as number) + u);
  }
  if (!Number.isFinite(lo)) {
    lo = 0;
    hi = 1;
  }
  const span = hi - lo || hi || 1;
  lo = Math.max(0, lo - span * 0.12);
  hi = hi + span * 0.08;
  const sy = linear(lo / 1e6, hi / 1e6, H - pad.b, pad.t);
  const ticks = niceTicks(lo / 1e6, hi / 1e6, 4);
  const xs = ys.map((y) => sx(y.year));
  const hov = useNearest(W, xs);
  if (!ys.length) return <div class="state-msg">No yearly areas.</div>;
  const fullPath = (() => {
    // Line through full-coverage years only; partial years are shown but not joined.
    const ok = ys.map((y, i) => ({ y, i })).filter(({ y }) => y.status === "ok" && y.area_m2 !== null);
    return ok.map(({ y, i }, k) => `${k ? "L" : "M"}${xs[i].toFixed(1)},${sy((y.area_m2 as number) / 1e6).toFixed(1)}`).join("");
  })();
  const h = hov.idx !== null ? ys[hov.idx] : null;
  const decimals = hi / 1e6 < 0.5 ? 3 : 2;

  return (
    <div class="chart">
      <svg
        ref={hov.ref}
        viewBox={`0 0 ${W} ${H}`}
        class="chart-svg"
        role="img"
        aria-label={`Lake area by year, ${x0} to ${x1}, with measurement uncertainty`}
        onPointerMove={hov.onMove}
        onPointerLeave={hov.onLeave}
      >
        {ticks.map((t) => (
          <g key={t}>
            <line x1={pad.l} x2={W - pad.r} y1={sy(t)} y2={sy(t)} class="grid" />
            <text x={pad.l - 6} y={sy(t) + 3} class="axis-label" text-anchor="end">
              {t.toFixed(decimals)}
            </text>
          </g>
        ))}
        <text x={10} y={pad.t + (H - pad.t - pad.b) / 2} class="axis-label" transform={`rotate(-90 10 ${pad.t + (H - pad.t - pad.b) / 2})`} text-anchor="middle">
          km²
        </text>
        {ys.map((y, i) => (
          <text key={y.year} x={xs[i]} y={H - 6} class="axis-label" text-anchor="middle">
            {`'${String(y.year).slice(2)}`}
          </text>
        ))}
        {h && <line x1={xs[hov.idx!]} x2={xs[hov.idx!]} y1={pad.t} y2={H - pad.b} class="crosshair" />}
        <path d={fullPath} fill="none" stroke={CHART.accent} stroke-width={2} stroke-linejoin="round" opacity={0.85} />
        {ys.map((y, i) => {
          if (y.area_m2 === null) {
            return (
              <g key={y.year}>
                <line x1={xs[i] - 3} x2={xs[i] + 3} y1={H - pad.b - 9} y2={H - pad.b - 3} stroke={CHART.muted} stroke-width={1.5} />
                <line x1={xs[i] + 3} x2={xs[i] - 3} y1={H - pad.b - 9} y2={H - pad.b - 3} stroke={CHART.muted} stroke-width={1.5} />
              </g>
            );
          }
          const a = (y.area_m2 as number) / 1e6;
          const u = (y.uncertainty_m2 ?? 0) / 1e6;
          const partial = y.status !== "ok";
          return (
            <g key={y.year}>
              <line x1={xs[i]} x2={xs[i]} y1={sy(a - u)} y2={sy(a + u)} stroke={CHART.whisker} stroke-width={1.2} opacity={0.7} />
              <line x1={xs[i] - 4} x2={xs[i] + 4} y1={sy(a - u)} y2={sy(a - u)} stroke={CHART.whisker} stroke-width={1.2} opacity={0.7} />
              <line x1={xs[i] - 4} x2={xs[i] + 4} y1={sy(a + u)} y2={sy(a + u)} stroke={CHART.whisker} stroke-width={1.2} opacity={0.7} />
              <circle
                cx={xs[i]}
                cy={sy(a)}
                r={hov.idx === i ? 5.5 : 4.5}
                fill={partial ? "var(--surface)" : CHART.accent}
                stroke={CHART.accent}
                stroke-width={2}
              />
            </g>
          );
        })}
        <rect x={pad.l} y={pad.t} width={W - pad.l - pad.r} height={H - pad.t - pad.b} fill="transparent" />
      </svg>
      {h && (
        <Tooltip
          x={xs[hov.idx!] * hov.scale}
          y={(h.area_m2 !== null ? sy(h.area_m2 / 1e6) : H - pad.b) * hov.scale}
          flip={hov.idx! > ys.length / 2}
        >
          <div class="tip-title">{h.year}</div>
          <div>
            {h.area_m2 !== null ? (
              <>
                <b>{km2(h.area_m2)}</b> ± {km2(h.uncertainty_m2)}
              </>
            ) : (
              "no area"
            )}
          </div>
          <div class="muted">{YEAR_STATUS_LABEL[h.status] ?? h.status}</div>
          <div class="muted">
            coverage {h.coverage !== null ? `${num(h.coverage * 100, 0)}%` : "—"} · {num(h.scenes_clear)} clear scenes
          </div>
        </Tooltip>
      )}
    </div>
  );
}

// ---------- sparkline ----------

export function Sparkline({ years }: { years: YearRecord[] }) {
  const W = 72;
  const H = 22;
  const ys = [...years].sort((a, b) => a.year - b.year);
  const m = ys.filter((y) => y.area_m2 !== null);
  if (m.length < 2) return <svg width={W} height={H} aria-hidden="true" />;
  const areas = m.map((y) => y.area_m2 as number);
  const lo = Math.min(...areas);
  const hi = Math.max(...areas);
  const sx = linear(ys[0].year, ys[ys.length - 1].year, 3, W - 3);
  const sy = linear(lo, hi === lo ? lo + 1 : hi, H - 3, 3);
  const ok = m.filter((y) => y.status === "ok");
  const d = ok.map((y, i) => `${i ? "L" : "M"}${sx(y.year).toFixed(1)},${sy(y.area_m2 as number).toFixed(1)}`).join("");
  return (
    <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} class="spark" aria-label="Area by year" role="img">
      <path d={d} fill="none" stroke="currentColor" stroke-width={1.4} stroke-linejoin="round" />
      {m.map((y) =>
        y.status === "ok" ? (
          <circle key={y.year} cx={sx(y.year)} cy={sy(y.area_m2 as number)} r={1.4} fill="currentColor" />
        ) : (
          <circle key={y.year} cx={sx(y.year)} cy={sy(y.area_m2 as number)} r={2} fill="var(--surface)" stroke="currentColor" stroke-width={1} />
        ),
      )}
    </svg>
  );
}
