// The map, loaded lazily (this is the only module that pulls in MapLibre GL JS).
// Basemap sources and flood-layer colours are the ones the officials'
// dashboard uses (web/dashboard/src/map.ts, colors.ts), so both read alike.

import {
  AttributionControl,
  LngLatBounds,
  Map as MLMap,
  Marker,
  NavigationControl,
  Popup,
  setWorkerUrl,
} from 'maplibre-gl';
import type { GeoJSONSourceSpecification, StyleSpecification } from 'maplibre-gl';
import type { Feature, FeatureCollection } from 'geojson';
import 'maplibre-gl/dist/maplibre-gl.css';
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url';
import { layerURL, type RiskLevel } from '../api';
import { greatCircleKm } from '../nearest';

setWorkerUrl(workerUrl);

export type Basemap = 'satellite' | 'map';

export interface MapLake {
  id: string;
  name: string;
  lon: number;
  lat: number;
  level: RiskLevel | null;
  score: number | null;
}

export interface MapPlace {
  label: string;
  lon: number;
  lat: number;
}

/** What the map should show for the current screen. */
export interface MapView {
  place: MapPlace | null;
  /** Lakes whose names are shown. */
  keyLakes: string[];
  /** Lakes framed together with the place. */
  fitLakes: string[];
  /** Lakes whose flood path and corridors are drawn. */
  floodLakes: string[];
}

/** Text for the lake popup, in the app's language. */
export interface MapLabels {
  risk: (level: RiskLevel) => string;
  score: (score: number) => string;
  distance: (km: number) => string;
}

export interface MapOptions {
  basemap: Basemap;
  /** Two-finger panning, so a thumb scrolling the page is not caught by the map. */
  cooperative: boolean;
  gestureHelp: string;
  onReady: () => void;
  onError: () => void;
}

export interface MapHandle {
  setLakes(lakes: MapLake[], labels: MapLabels): void;
  setView(view: MapView): void;
  setBasemap(b: Basemap): void;
  destroy(): void;
}

export const LEVEL_COLOR: Record<RiskLevel, string> = {
  low: '#3fb950',
  moderate: '#e3b341',
  high: '#f0883e',
  very_high: '#f85149',
};
const UNSCORED = '#8193a9';

const COLORS = {
  corridorSevere: '#f2a0a0',
  corridorExpected: '#c0392b',
  floodPath: '#203a5c',
  floodPathSat: '#7fd3ff',
};

const STYLE: StyleSpecification = {
  version: 8,
  sources: {
    osm: {
      type: 'raster',
      tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
      tileSize: 256,
      maxzoom: 19,
      attribution:
        '© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> contributors',
    },
    esri: {
      type: 'raster',
      tiles: ['https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}'],
      tileSize: 256,
      maxzoom: 19,
      attribution: 'Imagery: Esri, Maxar, Earthstar Geographics',
    },
  },
  layers: [
    { id: 'bg', type: 'background', paint: { 'background-color': '#0a1320' } },
    {
      id: 'basemap-osm',
      type: 'raster',
      source: 'osm',
      layout: { visibility: 'none' },
      paint: { 'raster-saturation': -0.55, 'raster-contrast': -0.05 },
    },
    {
      id: 'basemap-esri',
      type: 'raster',
      source: 'esri',
      layout: { visibility: 'none' },
      paint: { 'raster-saturation': -0.2, 'raster-brightness-max': 0.9 },
    },
  ],
};

type FC = FeatureCollection | Feature;

/** Himachal Pradesh, roughly: the first view before the lakes arrive. */
const HOME_BOUNDS: [[number, number], [number, number]] = [
  [75.9, 30.9],
  [79.0, 33.0],
];

async function getJSON(lakeId: string, layer: string): Promise<FC | null> {
  try {
    const res = await fetch(layerURL(lakeId, layer));
    return res.ok ? ((await res.json()) as FC) : null;
  } catch {
    return null;
  }
}

function el<K extends keyof HTMLElementTagNameMap>(tag: K, cls?: string, text?: string): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

export function createMap(container: HTMLElement, opts: MapOptions): MapHandle {
  let map: MLMap;
  try {
    map = new MLMap({
      container,
      style: STYLE,
      bounds: HOME_BOUNDS,
      fitBoundsOptions: { padding: 24 },
      maxZoom: 16,
      attributionControl: false,
      maxPitch: 0,
      dragRotate: false,
      cooperativeGestures: opts.cooperative,
      locale: {
        'CooperativeGesturesHandler.MobileHelpText': opts.gestureHelp,
      },
      // Cheaper on low-end phones.
      fadeDuration: 0,
      pixelRatio: Math.min(window.devicePixelRatio || 1, 2),
    });
  } catch {
    opts.onError(); // no WebGL
    return { setLakes() {}, setView() {}, setBasemap() {}, destroy() {} };
  }
  map.touchZoomRotate.disableRotation();
  map.addControl(new NavigationControl({ showCompass: false }), 'top-right');
  map.addControl(new AttributionControl({ compact: true }), 'bottom-right');

  let dead = false;
  let basemap = opts.basemap;
  let lakes: MapLake[] = [];
  let labels: MapLabels | null = null;
  let view: MapView = { place: null, keyLakes: [], fitLakes: [], floodLakes: [] };
  let viewSeq = 0;
  let popup: Popup | null = null;
  let placeMarker: Marker | null = null;
  const lakeMarkers = new Map<string, Marker>();
  const drawn = new Set<string>(); // lake ids with flood layers on the map

  map.on('error', (e) => {
    // Tile errors are not fatal (offline, slow network); only note a broken style.
    if (!('sourceId' in e)) console.warn('map error', e.error);
  });

  const loaded = new Promise<void>((resolve) => map.once('load', () => resolve()));
  loaded.then(() => {
    if (dead) return;
    applyBasemap();
    opts.onReady();
  });

  // The map sits in a responsive layout (and is hidden on some phone screens),
  // so follow the container, and re-frame when it first gets a real size.
  let hadSize = container.clientWidth > 0 && container.clientHeight > 0;
  const ro =
    typeof ResizeObserver === 'function'
      ? new ResizeObserver(() => {
          if (dead) return;
          const has = container.clientWidth > 0 && container.clientHeight > 0;
          if (!has) {
            hadSize = false;
            return;
          }
          map.resize();
          if (!hadSize) fit();
          hadSize = true;
        })
      : null;
  ro?.observe(container);

  function applyBasemap() {
    const sat = basemap === 'satellite';
    map.setLayoutProperty('basemap-esri', 'visibility', sat ? 'visible' : 'none');
    map.setLayoutProperty('basemap-osm', 'visibility', sat ? 'none' : 'visible');
    for (const id of drawn) {
      if (map.getLayer(`${id}-path`)) {
        map.setPaintProperty(`${id}-path`, 'line-color', sat ? COLORS.floodPathSat : COLORS.floodPath);
      }
    }
  }

  function padding() {
    const w = container.clientWidth;
    const h = container.clientHeight;
    // Labels sit to the right of their dot; the legend sits along the bottom.
    const side = Math.min(48, Math.round(w * 0.08));
    return {
      top: Math.min(56, Math.round(h * 0.14)),
      bottom: Math.min(72, Math.round(h * 0.2)),
      left: side,
      right: Math.min(130, Math.round(w * 0.26)),
    };
  }

  function fit() {
    if (container.clientWidth === 0 || container.clientHeight === 0) return;
    const b = new LngLatBounds();
    const { place, fitLakes } = view;
    if (place) b.extend([place.lon, place.lat]);
    const key = lakes.filter((l) => fitLakes.includes(l.id));
    for (const l of key) b.extend([l.lon, l.lat]);
    if (!place && key.length === 0) for (const l of lakes) b.extend([l.lon, l.lat]);
    if (b.isEmpty()) return;
    const sw = b.getSouthWest();
    const ne = b.getNorthEast();
    if (sw.lng === ne.lng && sw.lat === ne.lat) {
      map.jumpTo({ center: sw, zoom: 10.5 });
      return;
    }
    map.fitBounds(b, { padding: padding(), maxZoom: 12.5, duration: 0 });
  }

  function lakePopup(l: MapLake): HTMLElement {
    const root = el('div', 'pop');
    root.append(el('p', 'pop-title', l.name));
    if (l.level && labels) {
      const badge = el('span', `pill pill-risk risk-${l.level}`, labels.risk(l.level));
      const row = el('p', 'pop-row');
      row.append(badge);
      root.append(row);
    }
    if (labels && l.score !== null) root.append(el('p', 'pop-line', labels.score(l.score)));
    if (labels && view.place) {
      const km = greatCircleKm([view.place.lon, view.place.lat], [l.lon, l.lat]);
      root.append(el('p', 'pop-line', labels.distance(km)));
    }
    return root;
  }

  function openPopup(l: MapLake) {
    popup?.remove();
    popup = new Popup({ offset: 14, maxWidth: '240px', closeButton: true, focusAfterOpen: false })
      .setLngLat([l.lon, l.lat])
      .setDOMContent(lakePopup(l))
      .addTo(map);
  }

  function markKeyLakes() {
    for (const [id, mk] of lakeMarkers) mk.getElement().classList.toggle('is-key', view.keyLakes.includes(id));
  }

  function drawLakes() {
    popup?.remove();
    popup = null;
    for (const mk of lakeMarkers.values()) mk.remove();
    lakeMarkers.clear();
    // Lowest score first, so the riskiest lakes are drawn on top.
    const order = [...lakes].sort((a, b) => (a.score ?? -1) - (b.score ?? -1));
    for (const l of order) {
      const node = el('button', 'lake-pin');
      node.type = 'button';
      node.title = l.name;
      node.setAttribute('aria-label', l.level && labels ? `${l.name} — ${labels.risk(l.level)}` : l.name);
      const dot = el('span', 'lake-pin-dot');
      dot.style.background = l.level ? LEVEL_COLOR[l.level] : UNSCORED;
      node.append(dot, el('span', 'lake-pin-label', l.name));
      node.addEventListener('click', (ev) => {
        ev.stopPropagation();
        openPopup(l);
      });
      lakeMarkers.set(l.id, new Marker({ element: node, anchor: 'center' }).setLngLat([l.lon, l.lat]).addTo(map));
    }
    markKeyLakes();
  }

  function drawPlace() {
    placeMarker?.remove();
    placeMarker = null;
    const p = view.place;
    if (!p) return;
    const node = el('div', 'place-pin');
    node.append(el('span', 'place-pin-dot'), el('span', 'place-pin-label', p.label));
    placeMarker = new Marker({ element: node, anchor: 'center' }).setLngLat([p.lon, p.lat]).addTo(map);
  }

  function removeFlood(id: string) {
    for (const part of ['path', 'expected', 'severe']) {
      const key = `${id}-${part}`;
      if (map.getLayer(key)) map.removeLayer(key);
      if (map.getSource(key)) map.removeSource(key);
    }
    drawn.delete(id);
  }

  async function drawFlood(id: string, seq: number) {
    const stale = () => dead || seq !== viewSeq;
    const add = (part: string, data: FC, layer: Record<string, unknown>, before?: string) => {
      const key = `${id}-${part}`;
      if (stale() || map.getSource(key)) return;
      map.addSource(key, { type: 'geojson', data } as GeoJSONSourceSpecification);
      map.addLayer(
        { id: key, source: key, ...layer } as Parameters<MLMap['addLayer']>[0],
        before && map.getLayer(before) ? before : undefined,
      );
    };
    drawn.add(id);
    // The line and the expected corridor first; the wider severe corridor after.
    const [path, expected] = await Promise.all([getJSON(id, 'flood_path'), getJSON(id, 'corridor_expected')]);
    if (path) {
      add('path', path, {
        type: 'line',
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: {
          'line-color': basemap === 'satellite' ? COLORS.floodPathSat : COLORS.floodPath,
          'line-width': ['interpolate', ['linear'], ['zoom'], 8, 1.6, 13, 3],
        },
      });
    }
    if (expected) {
      add(
        'expected',
        expected,
        { type: 'fill', paint: { 'fill-color': COLORS.corridorExpected, 'fill-opacity': 0.45 } },
        `${id}-path`,
      );
    }
    const severe = await getJSON(id, 'corridor_severe');
    if (severe) {
      add(
        'severe',
        severe,
        { type: 'fill', paint: { 'fill-color': COLORS.corridorSevere, 'fill-opacity': 0.32 } },
        map.getLayer(`${id}-expected`) ? `${id}-expected` : `${id}-path`,
      );
    }
  }

  return {
    setLakes(next, nextLabels) {
      lakes = next;
      labels = nextLabels;
      loaded.then(() => {
        if (dead) return;
        drawLakes();
        fit();
      });
    },
    setView(next) {
      view = next;
      const seq = ++viewSeq;
      loaded.then(() => {
        if (dead || seq !== viewSeq) return;
        popup?.remove();
        popup = null;
        drawPlace();
        markKeyLakes();
        for (const id of [...drawn]) if (!next.floodLakes.includes(id)) removeFlood(id);
        for (const id of next.floodLakes) if (!drawn.has(id)) void drawFlood(id, seq);
        fit();
      });
    },
    setBasemap(b) {
      basemap = b;
      loaded.then(() => !dead && applyBasemap());
    },
    destroy() {
      dead = true;
      ro?.disconnect();
      popup?.remove();
      placeMarker?.remove();
      for (const mk of lakeMarkers.values()) mk.remove();
      map.remove();
    },
  };
}
