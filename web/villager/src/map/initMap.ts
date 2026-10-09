// The map, loaded lazily (this module pulls in MapLibre GL JS).

import { LngLatBounds, Map as MLMap, Marker, NavigationControl, AttributionControl, setWorkerUrl } from 'maplibre-gl';
import type { GeoJSONSourceSpecification, StyleSpecification } from 'maplibre-gl';
import type { Feature, FeatureCollection } from 'geojson';
import 'maplibre-gl/dist/maplibre-gl.css';
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url';
import { layerURL } from '../api';

setWorkerUrl(workerUrl);

export interface MapOptions {
  village: { lon: number; lat: number; label: string } | null;
  lakes: { id: string; name: string; status: string }[];
  onReady: () => void;
  onError: () => void;
}

const COLORS = {
  severe: '#8ec5f0',
  expected: '#1f5fa8',
  path: '#0b2f6b',
  lakeFill: '#38bdf8',
  lakeLine: '#075985',
};

const style: StyleSpecification = {
  version: 8,
  sources: {
    osm: {
      type: 'raster',
      tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
      tileSize: 256,
      maxzoom: 19,
      attribution: '© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> contributors',
    },
  },
  layers: [{ id: 'osm', type: 'raster', source: 'osm' }],
};

type FC = FeatureCollection | Feature;

// Labels sit to the right of their dot, so leave room on that side.
const FIT_PADDING = { top: 40, bottom: 40, left: 30, right: 120 };

async function getJSON(lakeId: string, layer: string): Promise<FC | null> {
  try {
    const res = await fetch(layerURL(lakeId, layer));
    return res.ok ? ((await res.json()) as FC) : null;
  } catch {
    return null;
  }
}

/** Only the most recent year's outline (outlines.geojson has one per year). */
function latestOutline(fc: FC): FC {
  if (fc.type !== 'FeatureCollection' || fc.features.length < 2) return fc;
  const year = (f: Feature) => Number(f.properties?.year) || 0;
  const latest = fc.features.reduce((a, b) => (year(b) > year(a) ? b : a));
  return { type: 'FeatureCollection', features: [latest] };
}

function extend(bounds: LngLatBounds, geom: FC): void {
  const walk = (c: unknown): void => {
    if (Array.isArray(c) && typeof c[0] === 'number') bounds.extend([c[0], c[1] as number]);
    else if (Array.isArray(c)) c.forEach(walk);
  };
  const feats = geom.type === 'FeatureCollection' ? geom.features : [geom];
  for (const f of feats) if (f.geometry && 'coordinates' in f.geometry) walk(f.geometry.coordinates);
}

function pin(className: string, text: string): HTMLElement {
  const el = document.createElement('div');
  el.className = `pin ${className}`;
  const dot = document.createElement('span');
  dot.className = 'pin-dot';
  const label = document.createElement('span');
  label.className = 'pin-label';
  label.textContent = text;
  el.append(dot, label);
  return el;
}

export function initMap(container: HTMLElement, opts: MapOptions): () => void {
  const { village, lakes } = opts;
  const hasVillage = village && Number.isFinite(village.lon) && Number.isFinite(village.lat);
  let map: MLMap;
  try {
    map = new MLMap({
      container,
      style,
      center: hasVillage ? [village.lon, village.lat] : [77.2, 32.3],
      zoom: hasVillage ? 11 : 7,
      attributionControl: false,
      maxPitch: 0,
      dragRotate: false,
      // Cheaper on low-end phones.
      fadeDuration: 0,
      pixelRatio: Math.min(window.devicePixelRatio || 1, 2),
    });
  } catch {
    opts.onError(); // no WebGL
    return () => {};
  }
  map.touchZoomRotate.disableRotation();
  map.addControl(new NavigationControl({ showCompass: false }), 'top-right');
  map.addControl(new AttributionControl({ compact: true }), 'bottom-right');

  const markers: Marker[] = [];
  let cancelled = false;

  map.on('error', (e) => {
    // Tile errors are not fatal; only report a broken style.
    if (!('sourceId' in e)) console.warn('map error', e.error);
  });

  map.on('load', async () => {
    opts.onReady();
    const bounds = new LngLatBounds();
    if (hasVillage) bounds.extend([village.lon, village.lat]);

    const add = (id: string, data: FC, layer: Parameters<MLMap['addLayer']>[0], before?: string) => {
      if (cancelled) return;
      map.addSource(id, { type: 'geojson', data } as GeoJSONSourceSpecification);
      map.addLayer(layer, before && map.getLayer(before) ? before : undefined);
    };

    // Village marker first, so something useful shows at once.
    if (hasVillage) {
      markers.push(new Marker({ element: pin('pin-village', village.label), anchor: 'left', offset: [-9, 0] })
        .setLngLat([village.lon, village.lat])
        .addTo(map));
    }

    await Promise.all(
      lakes.map(async (lake) => {
        const id = lake.id;
        // Small layers first: outline and flood path, then the big corridors.
        const [outline, path] = await Promise.all([getJSON(id, 'outlines'), getJSON(id, 'flood_path')]);
        if (cancelled) return;
        if (path) {
          add(`${id}-path`, path, {
            id: `${id}-path`,
            type: 'line',
            source: `${id}-path`,
            paint: { 'line-color': COLORS.path, 'line-width': ['interpolate', ['linear'], ['zoom'], 8, 2, 14, 4] },
            layout: { 'line-cap': 'round', 'line-join': 'round' },
          });
        }
        if (outline) {
          const latest = latestOutline(outline);
          add(`${id}-lake`, latest, {
            id: `${id}-lake`,
            type: 'fill',
            source: `${id}-lake`,
            paint: { 'fill-color': COLORS.lakeFill, 'fill-opacity': 0.8, 'fill-outline-color': COLORS.lakeLine },
          });
          extend(bounds, latest);
          const lb = new LngLatBounds();
          extend(lb, latest);
          if (!lb.isEmpty()) {
            markers.push(new Marker({ element: pin('pin-lake', lake.name), anchor: 'left', offset: [-9, 0] })
              .setLngLat(lb.getCenter())
              .addTo(map));
          }
        }
        if (!bounds.isEmpty()) map.fitBounds(bounds, { padding: FIT_PADDING, maxZoom: 13, duration: 0 });

        const [severe, expected] = await Promise.all([
          getJSON(id, 'corridor_severe'),
          getJSON(id, 'corridor_expected'),
        ]);
        if (severe) {
          add(`${id}-severe`, severe, {
            id: `${id}-severe`,
            type: 'fill',
            source: `${id}-severe`,
            paint: { 'fill-color': COLORS.severe, 'fill-opacity': 0.45 },
          }, `${id}-path`);
        }
        if (expected) {
          add(`${id}-expected`, expected, {
            id: `${id}-expected`,
            type: 'fill',
            source: `${id}-expected`,
            paint: { 'fill-color': COLORS.expected, 'fill-opacity': 0.45 },
          }, `${id}-path`);
        }
      }),
    );
  });

  return () => {
    cancelled = true;
    markers.forEach((m) => m.remove());
    map.remove();
  };
}
