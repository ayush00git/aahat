import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import { COLORS, STATUS_COLOR } from "../colors";
import { km2, YEAR_STATUS_LABEL } from "../format";
import { MapController, type Basemap, type LayerGroup } from "../map";
import type { GeoJSONDoc, Impact, Lake, LakeLayers, YearRecord } from "../types";

function outlineYears(doc: GeoJSONDoc | undefined): Set<number> {
  const s = new Set<number>();
  if (!doc) return s;
  const feats = doc.type === "FeatureCollection" ? doc.features : [doc];
  for (const f of feats) {
    const y = Number(f.properties?.year);
    if (Number.isFinite(y) && f.geometry) s.add(y);
  }
  return s;
}

const PLAY_MS = 900;

export function MapView({
  lakes,
  lake,
  layers,
  impacts,
  loading,
  onSelectLake,
  onReady,
}: {
  lakes: Lake[];
  lake: Lake | null;
  layers: LakeLayers | null;
  impacts: Impact[] | null;
  loading: boolean;
  onSelectLake: (id: string) => void;
  onReady: (c: MapController) => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const ctrl = useRef<MapController | null>(null);
  const [basemap, setBasemap] = useState<Basemap>("satellite");
  const [visible, setVisible] = useState<Record<LayerGroup, boolean>>({
    outlines: true,
    glaciers: true,
    flood: true,
    impacts: true,
  });
  const [year, setYear] = useState<number | null>(null);
  const [playing, setPlaying] = useState(false);
  const [legendOpen, setLegendOpen] = useState(() => window.innerWidth > 1100);
  const autoCollapsed = useRef(false);

  // Create the map once.
  useEffect(() => {
    if (!box.current) return;
    const c = new MapController(box.current);
    ctrl.current = c;
    // Collapse the Layers box while an asset popup is open (and reopen it after,
    // if it was open), then pan the popup clear of whatever overlays remain.
    c.onPopupChange = (open) => {
      if (open) {
        setLegendOpen((was) => {
          if (was) autoCollapsed.current = true;
          return false;
        });
      } else if (autoCollapsed.current) {
        autoCollapsed.current = false;
        setLegendOpen(true);
      }
    };
    c.overlayRects = () => {
      const wrap = box.current?.parentElement;
      if (!wrap) return [];
      return [...wrap.querySelectorAll<HTMLElement>(".map-overlay, .maplibregl-ctrl-top-right .maplibregl-ctrl, .map-error")].map(
        (e) => e.getBoundingClientRect(),
      );
    };
    onReady(c);
    const ro = new ResizeObserver(() => c.resize());
    ro.observe(box.current);
    return () => {
      ro.disconnect();
      c.destroy();
      ctrl.current = null;
    };
  }, []);

  useEffect(() => {
    if (lakes.length) ctrl.current?.setLakes(lakes, onSelectLake);
  }, [lakes]);

  useEffect(() => {
    ctrl.current?.markSelected(lake?.id ?? null);
    setPlaying(false);
    if (lake) ctrl.current?.flyToLake(lake);
    else ctrl.current?.clearLakeData();
  }, [lake?.id]);

  useEffect(() => {
    if (!lake || !layers) return;
    ctrl.current?.setLakeData(layers, impacts ?? []);
  }, [layers, impacts]);

  const years: YearRecord[] = useMemo(() => (lake ? [...lake.years].sort((a, b) => a.year - b.year) : []), [lake?.id]);
  const withOutline = useMemo(() => outlineYears(layers?.outlines), [layers]);

  // Default to the latest year that has an outline.
  useEffect(() => {
    if (!years.length) {
      setYear(null);
      return;
    }
    const ys = years.map((y) => y.year).filter((y) => withOutline.has(y));
    setYear(ys.length ? ys[ys.length - 1] : years[years.length - 1].year);
  }, [years, withOutline]);

  const rec = years.find((y) => y.year === year) ?? null;

  useEffect(() => {
    if (year !== null) ctrl.current?.setYear(year, rec?.status === "partial");
  }, [year, rec?.status, layers]);

  // Time-lapse playback.
  useEffect(() => {
    if (!playing || !years.length) return;
    const t = setInterval(() => {
      setYear((cur) => {
        // The next recorded year after the current one (the slider can rest on a year without a record).
        const next = years.find((y) => cur === null || y.year > cur);
        if (!next) {
          setPlaying(false);
          return cur;
        }
        return next.year;
      });
    }, PLAY_MS);
    return () => clearInterval(t);
  }, [playing, years]);

  const step = (d: -1 | 1) => {
    setPlaying(false);
    const i = years.findIndex((y) => y.year === year);
    const next = years[Math.max(0, Math.min(years.length - 1, (i < 0 ? years.length - 1 : i) + d))];
    if (next) setYear(next.year);
  };

  const togglePlay = () => {
    if (!years.length) return;
    // From the last year (or beyond): start over from the first and play through.
    if (!playing && (year === null || year >= years[years.length - 1].year)) setYear(years[0].year);
    setPlaying(!playing);
  };

  const setGroup = (g: LayerGroup, v: boolean) => {
    setVisible({ ...visible, [g]: v });
    ctrl.current?.setGroupVisible(g, v);
  };
  const changeBasemap = (b: Basemap) => {
    setBasemap(b);
    ctrl.current?.setBasemap(b);
  };

  const sat = basemap === "satellite";
  const minYear = years.length ? years[0].year : 0;
  const maxYear = years.length ? years[years.length - 1].year : 0;

  return (
    <div class="map-wrap">
      <div ref={box} class="map" />

      <div class="map-overlay map-controls">
        <div class="seg" role="group" aria-label="Basemap">
          <button type="button" class={sat ? "on" : ""} aria-pressed={sat} onClick={() => changeBasemap("satellite")}>
            Satellite
          </button>
          <button type="button" class={!sat ? "on" : ""} aria-pressed={!sat} onClick={() => changeBasemap("map")}>
            Map
          </button>
        </div>
        <div class="seg" role="group" aria-label="Zoom to">
          <button type="button" disabled={!lake} onClick={() => ctrl.current?.fitLake()}>
            Lake
          </button>
          <button type="button" disabled={!lake} onClick={() => ctrl.current?.fitFloodPath(legendOpen ? 250 : 60)}>
            Flood path
          </button>
          <button type="button" onClick={() => ctrl.current?.fitAll(lakes)}>
            All lakes
          </button>
        </div>
        {loading && (
          <span class="map-loading" role="status">
            <span class="spinner" aria-hidden="true" /> Loading layers…
          </span>
        )}
      </div>

      {lake && (
        <div class={`map-overlay map-legend${legendOpen ? "" : " closed"}`} aria-label="Map layers">
          <button type="button" class="lg-toggle" onClick={() => {
              autoCollapsed.current = false;
              setLegendOpen(!legendOpen);
            }}
            aria-expanded={legendOpen}
          >
            <span>Layers</span>
            <svg viewBox="0 0 12 12" width="10" height="10" aria-hidden="true" class={legendOpen ? "chev up" : "chev"}>
              <path d="M2 4.5 6 8.5 10 4.5" fill="none" stroke="currentColor" stroke-width="1.6" />
            </svg>
          </button>
          {legendOpen && (
            <>
              <label class="lg-row">
                <input type="checkbox" checked={visible.outlines} onChange={(e) => setGroup("outlines", e.currentTarget.checked)} />
                <span class="sw sw-line" style={{ borderColor: sat ? COLORS.outlineSat : COLORS.outline, background: "transparent" }} />
                Lake outline {year ?? ""}
                <span class="sw sw-line thin" style={{ borderColor: sat ? COLORS.outlineStackSat : COLORS.outlineStack, opacity: 0.6 }} />
                <span class="muted">earlier</span>
              </label>
              <label class="lg-row">
                <input type="checkbox" checked={visible.glaciers} onChange={(e) => setGroup("glaciers", e.currentTarget.checked)} />
                <span class="sw" style={{ background: COLORS.glacier, borderColor: COLORS.glacierLine }} />
                Glaciers (RGI 7.0, c. 2000)
              </label>
              <label class="lg-row">
                <input type="checkbox" checked={visible.flood} onChange={(e) => setGroup("flood", e.currentTarget.checked)} />
                <span class="sw sw-line" style={{ borderColor: sat ? COLORS.floodPathSat : COLORS.floodPath }} />
                Flood path
                <span class="sw" style={{ background: COLORS.corridorExpected, opacity: 0.75 }} />
                expected
                <span class="sw" style={{ background: COLORS.corridorSevere, opacity: 0.75 }} />
                severe
              </label>
              <label class="lg-row">
                <input type="checkbox" checked={visible.impacts} onChange={(e) => setGroup("impacts", e.currentTarget.checked)} />
                <span class="sw dot" style={{ background: STATUS_COLOR.in_flood_path }} />
                In flood path
                <span class="sw dot" style={{ background: STATUS_COLOR.at_risk }} />
                At risk
              </label>
              <div class="lg-row muted small">
                <span class="sw dot outlet" /> Outlet &amp; spill path
              </div>
            </>
          )}
        </div>
      )}

      {lake && years.length > 0 && (
        <div class="map-overlay timelapse" role="group" aria-label="Lake outline time-lapse">
          <div class="tl-buttons">
            <button type="button" class="tl-step" onClick={() => step(-1)} disabled={year === minYear} aria-label="Previous year" title="Previous year">
              <svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true">
                <path d="M3 2v12M13 2 5.5 8 13 14Z" fill="currentColor" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" />
              </svg>
            </button>
            <button
              type="button"
              class="play"
              onClick={togglePlay}
              aria-label={playing ? "Pause time-lapse" : "Play time-lapse"}
              title={playing ? "Pause" : "Play"}
            >
              {playing ? (
                <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
                  <rect x="3" y="2" width="3.5" height="12" rx="1" fill="currentColor" />
                  <rect x="9.5" y="2" width="3.5" height="12" rx="1" fill="currentColor" />
                </svg>
              ) : (
                <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
                  <path d="M4.5 2.2v11.6a.8.8 0 0 0 1.2.7l9-5.8a.8.8 0 0 0 0-1.4l-9-5.8a.8.8 0 0 0-1.2.7Z" fill="currentColor" />
                </svg>
              )}
            </button>
            <button type="button" class="tl-step" onClick={() => step(1)} disabled={year === maxYear} aria-label="Next year" title="Next year">
              <svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true">
                <path d="M13 2v12M3 2l7.5 6L3 14Z" fill="currentColor" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" />
              </svg>
            </button>
          </div>
          <div class="tl-track">
            <input
              type="range"
              min={minYear}
              max={maxYear}
              step={1}
              value={year ?? maxYear}
              style={{ "--p": `${maxYear > minYear ? (((year ?? maxYear) - minYear) / (maxYear - minYear)) * 100 : 100}%` }}
              onInput={(e) => {
                setPlaying(false);
                setYear(Number(e.currentTarget.value));
              }}
              aria-label="Year"
              aria-valuetext={String(year)}
            />
            <div class="tl-ticks" aria-hidden="true">
              {years.map((y, i) => (
                <span
                  key={y.year}
                  class={`tl-tick ${y.status}${y.year === year ? " cur" : ""}${withOutline.has(y.year) ? "" : " none"}${
                    i === 0 || i === years.length - 1 ? " edge" : ""
                  }`}
                  title={`${y.year}: ${YEAR_STATUS_LABEL[y.status] ?? y.status}`}
                >
                  <span class="tl-tick-dot" />
                  <span class="tl-tick-label">{y.year}</span>
                </span>
              ))}
            </div>
          </div>
          <div class="tl-readout">
            <div class="tl-year">{year}</div>
            <div class="tl-area">
              {rec && rec.area_m2 !== null && withOutline.has(rec.year) ? km2(rec.area_m2) : "no outline"}
            </div>
            <div class={`tl-status ${rec?.status ?? ""}`}>{rec ? YEAR_STATUS_LABEL[rec.status] ?? rec.status : ""}</div>
          </div>
        </div>
      )}
    </div>
  );
}
