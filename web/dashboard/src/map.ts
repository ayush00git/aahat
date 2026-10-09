// Imperative MapLibre wrapper. The Preact tree owns state; this owns the map.

import {
  Map as MLMap,
  Marker,
  NavigationControl,
  Popup,
  ScaleControl,
  AttributionControl,
  setWorkerUrl,
  type GeoJSONSource,
  type LngLatBoundsLike,
  type StyleSpecification,
} from "maplibre-gl";
import workerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?url";
import "maplibre-gl/dist/maplibre-gl.css";

import { COLORS, LEVEL_COLOR, STATUS_COLOR } from "./colors";
import { arrival, KIND_LABEL, metres, num, STATUS_LABEL, subkind, LEVEL_LABEL, signedMetres } from "./format";
import type { GeoCollection, GeoFeature, GeoJSONDoc, Impact, Lake, LakeLayers } from "./types";

setWorkerUrl(workerUrl);

export type Basemap = "map" | "satellite";
export type LayerGroup = "outlines" | "glaciers" | "flood" | "impacts";

const EMPTY: GeoCollection = { type: "FeatureCollection", features: [] };

const STYLE: StyleSpecification = {
  version: 8,
  sources: {
    osm: {
      type: "raster",
      tiles: ["https://tile.openstreetmap.org/{z}/{x}/{y}.png"],
      tileSize: 256,
      maxzoom: 19,
      attribution:
        '© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> contributors',
    },
    esri: {
      type: "raster",
      tiles: [
        "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}",
      ],
      tileSize: 256,
      maxzoom: 19,
      attribution: "Imagery: Esri, Maxar, Earthstar Geographics",
    },
  },
  layers: [
    // Satellite imagery by default; the OSM map is one click away.
    {
      id: "basemap-osm",
      type: "raster",
      source: "osm",
      layout: { visibility: "none" },
      paint: { "raster-saturation": -0.55, "raster-contrast": -0.05 },
    },
    {
      id: "basemap-esri",
      type: "raster",
      source: "esri",
      // Slightly muted so the flood layers read clearly over the imagery.
      paint: { "raster-saturation": -0.2, "raster-brightness-max": 0.9 },
    },
  ],
};

const GROUP_LAYERS: Record<LayerGroup, string[]> = {
  outlines: ["outlines-stack", "outline-current-fill", "outline-current-line"],
  glaciers: ["glaciers-fill", "glaciers-line"],
  flood: [
    "corridor-severe-fill",
    "corridor-severe-line",
    "corridor-expected-fill",
    "flood-path",
    "outlet-path",
    "outlet-point",
  ],
  impacts: ["impacts"],
};

function asCollection(doc: GeoJSONDoc | undefined): GeoCollection {
  if (!doc) return EMPTY;
  if (doc.type === "FeatureCollection") return doc;
  return { type: "FeatureCollection", features: [doc as GeoFeature] };
}

function extendBounds(coords: unknown, b: [number, number, number, number]) {
  if (!Array.isArray(coords)) return;
  if (typeof coords[0] === "number" && typeof coords[1] === "number") {
    const [x, y] = coords as number[];
    b[0] = Math.min(b[0], x);
    b[1] = Math.min(b[1], y);
    b[2] = Math.max(b[2], x);
    b[3] = Math.max(b[3], y);
    return;
  }
  for (const c of coords) extendBounds(c, b);
}

export function boundsOf(doc: GeoJSONDoc | undefined): LngLatBoundsLike | null {
  const fc = asCollection(doc);
  const b: [number, number, number, number] = [Infinity, Infinity, -Infinity, -Infinity];
  for (const f of fc.features) extendBounds(f.geometry?.coordinates, b);
  if (!Number.isFinite(b[0])) return null;
  return [
    [b[0], b[1]],
    [b[2], b[3]],
  ];
}

function el<K extends keyof HTMLElementTagNameMap>(tag: K, cls?: string, text?: string): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

/** Popup body for an exposed asset, built from DOM nodes (no HTML strings). */
function impactPopup(im: Impact): HTMLElement {
  const root = el("div", "pop");
  const title = el("div", "pop-title", im.name || `Unnamed ${KIND_LABEL[im.kind]?.toLowerCase() ?? im.kind}`);
  if (im.name_hi) title.append(el("span", "hi", ` ${im.name_hi}`));
  root.append(title);
  const badge = el("span", `badge status-${im.status}`, STATUS_LABEL[im.status]);
  const sub = el("div", "pop-sub", `${KIND_LABEL[im.kind] ?? im.kind}${im.subkind ? " · " + subkind(im.subkind) : ""} `);
  sub.append(badge);
  root.append(sub);
  const dl = el("dl", "pop-dl");
  const row = (k: string, v: string) => {
    dl.append(el("dt", undefined, k), el("dd", undefined, v));
  };
  row("Distance", `${num(im.km, 1)} km downstream`);
  row("Arrival", `${arrival(im.arrival_min_fast, im.arrival_min_expected)} (fast–expected)`);
  const e = im.scenarios?.expected;
  const s = im.scenarios?.severe;
  row("Depth", `${metres(e?.flood_depth_m)} expected · ${metres(s?.flood_depth_m)} severe`);
  row(
    "Above flood",
    `${signedMetres(e?.height_above_flood_m)} expected · ${signedMetres(s?.height_above_flood_m)} severe`,
  );
  root.append(dl);
  const osm = el("a", "pop-osm", im.osm) as HTMLAnchorElement;
  osm.href = `https://www.openstreetmap.org/${im.osm}`;
  osm.target = "_blank";
  osm.rel = "noopener";
  root.append(osm);
  return root;
}

export class MapController {
  readonly map: MLMap;
  private loaded: Promise<void>;
  private lakeMarkers = new globalThis.Map<string, Marker>();
  private impacts: Impact[] = [];
  private popup: Popup | null = null;
  /** Called when the asset popup opens or closes (the view collapses the Layers box). */
  onPopupChange: ((open: boolean) => void) | null = null;
  /** Screen rectangles of overlays drawn over the map; popups are panned clear of them. */
  overlayRects: (() => DOMRect[]) | null = null;
  private basemap: Basemap = "satellite";
  private layers: LakeLayers = {};

  constructor(container: HTMLElement) {
    this.map = new MLMap({
      container,
      style: STYLE,
      center: [77.3, 32.2],
      zoom: 7,
      maxZoom: 18,
      attributionControl: false,
      fadeDuration: 0,
    });
    this.map.addControl(new NavigationControl({ visualizePitch: false }), "top-right");
    // Bottom controls stack upwards in reverse order: attribution stays on the bottom line
    // (under the time-lapse bar), the scale sits above it at the right.
    this.map.addControl(new AttributionControl({ compact: true }), "bottom-right");
    this.map.addControl(new ScaleControl({ unit: "metric" }), "bottom-right");
    this.loaded = new Promise((resolve) => this.map.once("load", () => resolve()));
    this.loaded.then(() => {
      this.addLakeLayers();
      this.applyBasemapColors();
    });
  }

  destroy() {
    this.map.remove();
  }

  private addLakeLayers() {
    const m = this.map;
    for (const id of ["glaciers", "corridor_severe", "corridor_expected", "flood_path", "outlines", "outlet", "impacts"]) {
      m.addSource(id, { type: "geojson", data: EMPTY });
    }
    m.addLayer({
      id: "glaciers-fill",
      type: "fill",
      source: "glaciers",
      paint: { "fill-color": COLORS.glacier, "fill-opacity": 0.45 },
    });
    m.addLayer({
      id: "glaciers-line",
      type: "line",
      source: "glaciers",
      paint: { "line-color": COLORS.glacierLine, "line-width": 0.8 },
    });
    m.addLayer({
      id: "corridor-severe-fill",
      type: "fill",
      source: "corridor_severe",
      paint: { "fill-color": COLORS.corridorSevere, "fill-opacity": 0.32 },
    });
    m.addLayer({
      id: "corridor-severe-line",
      type: "line",
      source: "corridor_severe",
      paint: { "line-color": COLORS.corridorSevere, "line-width": 0.7, "line-opacity": 0.9 },
    });
    m.addLayer({
      id: "corridor-expected-fill",
      type: "fill",
      source: "corridor_expected",
      paint: { "fill-color": COLORS.corridorExpected, "fill-opacity": 0.42 },
    });
    m.addLayer({
      id: "flood-path",
      type: "line",
      source: "flood_path",
      layout: { "line-cap": "round", "line-join": "round" },
      paint: { "line-color": COLORS.floodPath, "line-width": ["interpolate", ["linear"], ["zoom"], 8, 1.2, 13, 2.5] },
    });
    m.addLayer({
      id: "outlines-stack",
      type: "line",
      source: "outlines",
      filter: ["<", ["get", "year"], 0],
      paint: { "line-color": COLORS.outlineStack, "line-width": 1, "line-opacity": 0.45 },
    });
    m.addLayer({
      id: "outline-current-fill",
      type: "fill",
      source: "outlines",
      filter: ["==", ["get", "year"], 0],
      paint: { "fill-color": COLORS.outline, "fill-opacity": 0.22 },
    });
    m.addLayer({
      id: "outline-current-line",
      type: "line",
      source: "outlines",
      filter: ["==", ["get", "year"], 0],
      paint: { "line-color": COLORS.outline, "line-width": 2.4 },
    });
    m.addLayer({
      id: "outlet-path",
      type: "line",
      source: "outlet",
      filter: ["==", ["geometry-type"], "LineString"],
      paint: { "line-color": COLORS.ink, "line-width": 2, "line-dasharray": [1.5, 1] },
    });
    m.addLayer({
      id: "outlet-point",
      type: "circle",
      source: "outlet",
      filter: ["==", ["geometry-type"], "Point"],
      paint: {
        "circle-radius": 5,
        "circle-color": "#ffffff",
        "circle-stroke-color": COLORS.ink,
        "circle-stroke-width": 2.5,
      },
    });
    m.addLayer({
      id: "impacts",
      type: "circle",
      source: "impacts",
      paint: {
        "circle-radius": [
          "interpolate",
          ["linear"],
          ["zoom"],
          8,
          ["match", ["get", "kind"], ["settlement", "school", "health"], 4.5, 3],
          14,
          ["match", ["get", "kind"], ["settlement", "school", "health"], 8, 6],
        ],
        "circle-color": [
          "match",
          ["get", "status"],
          "in_flood_path",
          STATUS_COLOR.in_flood_path,
          "at_risk",
          STATUS_COLOR.at_risk,
          COLORS.muted,
        ],
        "circle-stroke-color": "#ffffff",
        "circle-stroke-width": 1.5,
      },
    });

    m.on("click", "impacts", (e) => {
      const f = e.features?.[0];
      if (!f) return;
      const i = Number(f.properties?.i);
      const im = this.impacts[i];
      if (im) this.openImpactPopup(im);
    });
    m.on("mouseenter", "impacts", () => (m.getCanvas().style.cursor = "pointer"));
    m.on("mouseleave", "impacts", () => (m.getCanvas().style.cursor = ""));
  }

  setLakes(lakes: Lake[], onSelect: (id: string) => void) {
    for (const mk of this.lakeMarkers.values()) mk.remove();
    this.lakeMarkers.clear();
    for (const lake of lakes) {
      const node = el("button", "lake-marker");
      node.type = "button";
      node.title = `${lake.name} (${lake.risk ? LEVEL_LABEL[lake.risk.level] : "unscored"})`;
      const dot = el("span", "lake-marker-dot");
      dot.style.background = lake.risk ? LEVEL_COLOR[lake.risk.level] : COLORS.muted;
      node.append(dot, el("span", "lake-marker-label", lake.name));
      node.addEventListener("click", (ev) => {
        ev.stopPropagation();
        onSelect(lake.id);
      });
      const mk = new Marker({ element: node, anchor: "left", offset: [-7, 0] }).setLngLat([lake.lon, lake.lat]).addTo(this.map);
      this.lakeMarkers.set(lake.id, mk);
    }
    const b = this.lakesBounds(lakes);
    if (b) this.map.fitBounds(b, { padding: 80, duration: 0, maxZoom: 9 });
  }

  private lakesBounds(lakes: Lake[]): LngLatBoundsLike | null {
    if (!lakes.length) return null;
    const lons = lakes.map((l) => l.lon);
    const lats = lakes.map((l) => l.lat);
    return [
      [Math.min(...lons), Math.min(...lats)],
      [Math.max(...lons), Math.max(...lats)],
    ];
  }

  markSelected(id: string | null) {
    for (const [lid, mk] of this.lakeMarkers) mk.getElement().classList.toggle("selected", lid === id);
  }

  async setLakeData(layers: LakeLayers, impacts: Impact[]) {
    await this.loaded;
    this.layers = layers;
    this.impacts = impacts;
    this.closePopup();
    const set = (src: string, doc: GeoJSONDoc | undefined) =>
      (this.map.getSource(src) as GeoJSONSource | undefined)?.setData(asCollection(doc) as never);
    set("glaciers", layers.glaciers);
    set("corridor_severe", layers.corridor_severe);
    set("corridor_expected", layers.corridor_expected);
    set("flood_path", layers.flood_path);
    set("outlines", layers.outlines);
    set("outlet", layers.outlet);
    const fc: GeoCollection = {
      type: "FeatureCollection",
      features: impacts.map((im, i) => ({
        type: "Feature",
        geometry: { type: "Point", coordinates: [im.lon, im.lat] },
        properties: { i, status: im.status, kind: im.kind },
      })),
    };
    set("impacts", fc);
  }

  async clearLakeData() {
    await this.setLakeData({}, []);
  }

  async setYear(year: number, partial: boolean) {
    await this.loaded;
    const m = this.map;
    m.setFilter("outlines-stack", ["<", ["get", "year"], year]);
    m.setFilter("outline-current-fill", ["==", ["get", "year"], year]);
    m.setFilter("outline-current-line", ["==", ["get", "year"], year]);
    m.setPaintProperty("outline-current-line", "line-dasharray", partial ? [2, 1.2] : [1, 0]);
  }

  async setBasemap(b: Basemap) {
    await this.loaded;
    this.basemap = b;
    const m = this.map;
    m.setLayoutProperty("basemap-osm", "visibility", b === "map" ? "visible" : "none");
    m.setLayoutProperty("basemap-esri", "visibility", b === "satellite" ? "visible" : "none");
    this.applyBasemapColors();
  }

  /** Line colours that read on the current basemap (light map or dark imagery). */
  private applyBasemapColors() {
    const m = this.map;
    const sat = this.basemap === "satellite";
    m.setPaintProperty("outline-current-line", "line-color", sat ? COLORS.outlineSat : COLORS.outline);
    m.setPaintProperty("outline-current-fill", "fill-color", sat ? COLORS.outlineSat : COLORS.outline);
    m.setPaintProperty("outlines-stack", "line-color", sat ? COLORS.outlineStackSat : COLORS.outlineStack);
    m.setPaintProperty("flood-path", "line-color", sat ? COLORS.floodPathSat : COLORS.floodPath);
    m.setPaintProperty("outlet-path", "line-color", sat ? "#ffffff" : COLORS.ink);
  }

  getBasemap() {
    return this.basemap;
  }

  async setGroupVisible(g: LayerGroup, visible: boolean) {
    await this.loaded;
    for (const id of GROUP_LAYERS[g]) this.map.setLayoutProperty(id, "visibility", visible ? "visible" : "none");
    if (g === "impacts" && !visible) this.closePopup();
  }

  flyToLake(lake: Lake) {
    this.map.flyTo({ center: [lake.lon, lake.lat], zoom: 13.2, speed: 1.6, essential: true });
  }

  fitLake() {
    const b = boundsOf(this.layers.outlines);
    if (b) this.map.fitBounds(b, { padding: { top: 90, bottom: 110, left: 80, right: 80 }, maxZoom: 15, duration: 900 });
  }

  /** Fits the whole modelled flood corridor; `topPad` clears the legend when it is open. */
  fitFloodPath(topPad = 60) {
    const b = boundsOf(this.layers.corridor_severe) ?? boundsOf(this.layers.flood_path);
    if (b) this.map.fitBounds(b, { padding: { top: topPad, bottom: 110, left: 40, right: 60 }, duration: 900 });
  }

  fitAll(lakes: Lake[]) {
    const b = this.lakesBounds(lakes);
    if (b) this.map.fitBounds(b, { padding: 80, maxZoom: 9, duration: 900 });
  }

  focusImpact(im: Impact) {
    this.map.flyTo({ center: [im.lon, im.lat], zoom: Math.max(this.map.getZoom(), 13), speed: 1.8, essential: true });
    this.openImpactPopup(im);
  }

  private openImpactPopup(im: Impact) {
    this.closePopup();
    const p = new Popup({ closeButton: true, maxWidth: "300px", offset: 10 })
      .setLngLat([im.lon, im.lat])
      .setDOMContent(impactPopup(im))
      .addTo(this.map);
    this.popup = p;
    p.on("close", () => {
      if (this.popup === p) this.popup = null;
      this.onPopupChange?.(false);
    });
    this.onPopupChange?.(true);
    // Wait for any fly-to to finish and for the view to collapse its overlays, then pan clear.
    const reveal = () => requestAnimationFrame(() => requestAnimationFrame(() => this.revealPopup(p)));
    if (this.map.isMoving()) this.map.once("moveend", reveal);
    else reveal();
  }

  /**
   * Pans the map so the popup is not covered by an overlay (map controls, the
   * Layers box, the time-lapse bar, the zoom buttons) or cut off at the edge.
   */
  private revealPopup(p: Popup) {
    if (this.popup !== p || !p.isOpen()) return;
    const box = this.map.getContainer().getBoundingClientRect();
    const pr = p.getElement().getBoundingClientRect();
    const gap = 8;
    let dx = 0;
    let dy = 0;
    for (const r of this.overlayRects?.() ?? []) {
      if (!r.width || !r.height) continue;
      const hit = pr.left < r.right && pr.right > r.left && pr.top < r.bottom && pr.bottom > r.top;
      if (!hit) continue;
      // Smallest move that clears this overlay: down, up, right or left.
      const moves: [number, number][] = [
        [0, r.bottom + gap - pr.top],
        [0, r.top - gap - pr.bottom],
        [r.right + gap - pr.left, 0],
        [r.left - gap - pr.right, 0],
      ];
      const fits = ([mx, my]: [number, number]) =>
        pr.left + mx >= box.left && pr.right + mx <= box.right && pr.top + my >= box.top && pr.bottom + my <= box.bottom;
      const ok = moves.filter(fits);
      const pick = (ok.length ? ok : moves).reduce((a, b) => (Math.hypot(...a) <= Math.hypot(...b) ? a : b));
      if (Math.abs(pick[0]) > Math.abs(dx)) dx = pick[0];
      if (Math.abs(pick[1]) > Math.abs(dy)) dy = pick[1];
    }
    // Keep it inside the map as well.
    if (pr.top + dy < box.top + gap) dy = box.top + gap - pr.top;
    if (pr.left + dx < box.left + gap) dx = box.left + gap - pr.left;
    if (pr.right + dx > box.right - gap) dx = box.right - gap - pr.right;
    if (dx || dy) this.map.panBy([-dx, -dy], { duration: 350 });
  }

  private closePopup() {
    const p = this.popup;
    this.popup = null;
    p?.remove();
  }

  resize() {
    this.map.resize();
  }
}
