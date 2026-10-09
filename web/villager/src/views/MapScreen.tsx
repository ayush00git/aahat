import { useEffect, useRef, useState } from 'preact/hooks';
import { useI18n } from '../i18n';
import { lakeName, placeName } from '../format';
import { href } from '../router';
import { usePlace } from '../usePlace';
import { placeCoords } from '../coords';
import { BackLink, PlaceStatus } from './Place';

export function MapScreen({ osm }: { osm: string }) {
  const { t, lang } = useI18n();
  const state = usePlace(osm);
  const el = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading');

  const data = state.kind === 'ok' ? state.data : null;
  const lakes = data ? [...data.threats, ...(data.nearby ?? [])] : [];
  const lakesKey = lakes.map((th) => `${th.lake_id}:${th.status}`).join(',');

  useEffect(() => {
    if (!data || !el.current) return;
    let cleanup: (() => void) | undefined;
    let live = true;
    setStatus('loading');
    // MapLibre (~250 KB gzipped) is only downloaded when this screen opens.
    const coords =
      data.lon != null && data.lat != null
        ? ([data.lon, data.lat] as [number, number])
        : placeCoords(data.osm, [data.name, data.name_hi]); // answers saved before the API had lon/lat
    Promise.all([import('../map/initMap'), coords])
      .then(([{ initMap }, lonlat]) => {
        if (!live || !el.current) return;
        cleanup = initMap(el.current, {
          village: lonlat && { lon: lonlat[0], lat: lonlat[1], label: placeName(data, lang) },
          lakes: lakes.map((th) => ({ id: th.lake_id, name: lakeName(th, lang), status: th.status })),
          onReady: () => live && setStatus('ready'),
          onError: () => live && setStatus('error'),
        });
      })
      .catch(() => live && setStatus('error'));
    return () => {
      live = false;
      cleanup?.();
    };
  }, [data?.osm, lakesKey, lang]);

  if (state.kind !== 'ok') return <PlaceStatus state={state} />;
  const name = placeName(state.data, lang);

  return (
    <div class="stack">
      <BackLink to={href.place(osm)} />
      <h1 class="page-title">
        {t.mapTitle}: {name}
      </h1>
      <p class="small muted">{t.mapData}</p>
      <div class="map-wrap">
        <div ref={el} class="map" role="region" aria-label={t.mapLabel(name)} />
        {status !== 'ready' && (
          <p class="map-status" role="status">
            {status === 'error' ? t.mapError : t.mapLoading}
          </p>
        )}
      </div>
      <Legend />
    </div>
  );
}

function Legend() {
  const { t } = useI18n();
  const items: [string, string][] = [
    ['sw-village', t.legendVillage],
    ['sw-lake', t.legendLake],
    ['sw-path', t.legendPath],
    ['sw-expected', t.legendExpected],
    ['sw-severe', t.legendSevere],
  ];
  return (
    <section aria-labelledby="legend-h">
      <h2 id="legend-h" class="section-label">
        {t.legend}
      </h2>
      <ul class="legend">
        {items.map(([cls, label]) => (
          <li key={cls}>
            <span class={`swatch ${cls}`} aria-hidden="true" />
            {label}
          </li>
        ))}
      </ul>
    </section>
  );
}
