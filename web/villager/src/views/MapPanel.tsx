import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import type { RiskLevel } from '../api';
import { useI18n } from '../i18n';
import { num } from '../format';
import { load, save } from '../storage';
import { nearestLakes } from '../nearest';
import { useLakes, useSelection } from '../mapStore';
import type { Basemap, MapHandle, MapLake, MapView } from '../map/initMap';
import { IconMap } from './icons';

type Status = 'held' | 'loading' | 'ready' | 'error';

const LEVELS: RiskLevel[] = ['low', 'moderate', 'high', 'very_high'];

/** True on a data-saver or 2G connection: the map then waits for a tap. */
function slowConnection(): boolean {
  const c = (navigator as Navigator & { connection?: { saveData?: boolean; effectiveType?: string } }).connection;
  return !!c && (c.saveData === true || /(^|-)2g$/.test(c.effectiveType ?? ''));
}

const whenIdle = (f: () => void): (() => void) => {
  if (typeof requestIdleCallback === 'function') {
    const id = requestIdleCallback(f, { timeout: 1500 });
    return () => cancelIdleCallback(id);
  }
  const id = setTimeout(f, 300);
  return () => clearTimeout(id);
};

/**
 * The map beside (laptop) or under the header of (phone) every screen: all
 * monitored lakes coloured by risk, and for the place on screen its marker,
 * the lakes that matter to it and their flood paths. MapLibre is downloaded
 * after the first paint; if it can't load, a quiet placeholder stays and the
 * rest of the app works as before.
 */
export function MapPanel() {
  const { t, lang } = useI18n();
  const sel = useSelection();
  const lakes = useLakes();
  const box = useRef<HTMLDivElement>(null);
  const handle = useRef<MapHandle | null>(null);
  const [status, setStatus] = useState<Status>(() => (slowConnection() ? 'held' : 'loading'));
  const [attempt, setAttempt] = useState(0);
  const [basemap, setBasemap] = useState<Basemap>(() => (load<Basemap>('basemap') === 'map' ? 'map' : 'satellite'));

  const wanted = status !== 'held';
  useEffect(() => {
    if (!wanted) return;
    let live = true;
    let made: MapHandle | undefined;
    const cancel = whenIdle(() => {
      import('../map/initMap')
        .then(({ createMap }) => {
          if (!live || !box.current) return;
          made = createMap(box.current, {
            basemap,
            cooperative: matchMedia('(pointer: coarse)').matches,
            gestureHelp: t.mapTwoFingers,
            onReady: () => {
              if (!live) return;
              handle.current = made ?? null;
              setStatus('ready');
            },
            onError: () => live && setStatus('error'),
          });
        })
        .catch(() => live && setStatus('error'));
    });
    return () => {
      live = false;
      cancel();
      handle.current = null;
      made?.destroy();
    };
    // The language only changes the two-finger hint, so the map is not rebuilt for it.
  }, [wanted, attempt]);

  // Back online after a failed load: try once more.
  useEffect(() => {
    if (status !== 'error') return;
    const on = () => {
      setStatus('loading');
      setAttempt((n) => n + 1);
    };
    window.addEventListener('online', on);
    return () => window.removeEventListener('online', on);
  }, [status]);

  const mapLakes = useMemo<MapLake[]>(
    () =>
      Array.isArray(lakes)
        ? lakes
            .filter((l) => typeof l.lon === 'number' && typeof l.lat === 'number')
            .map((l) => ({
              id: l.id,
              name: lang === 'hi' ? l.name_hi || l.name : l.name,
              lon: l.lon as number,
              lat: l.lat as number,
              level: l.risk_level,
              score: l.risk_score ?? null,
            }))
        : [],
    [lakes, lang],
  );

  const view = useMemo<MapView>(() => {
    if (!sel) return { place: null, keyLakes: [], fitLakes: [], floodLakes: [] };
    const floodLakes = sel.floodLakes.slice(0, 3);
    const near = sel.lonlat && Array.isArray(lakes) ? nearestLakes(lakes, sel.lonlat, 3).map((l) => l.id) : [];
    const keyLakes = floodLakes.length > 0 ? floodLakes : near;
    return {
      place: sel.lonlat ? { label: sel.label, lon: sel.lonlat[0], lat: sel.lonlat[1] } : null,
      keyLakes,
      // Lakes that threaten the place are all framed; of floods that only pass
      // nearby, the first (nearest along the river) is, so the village stays readable.
      fitLakes: sel.threatening || floodLakes.length === 0 ? keyLakes : keyLakes.slice(0, 1),
      floodLakes,
    };
  }, [sel, lakes]);

  useEffect(() => {
    if (status !== 'ready') return;
    handle.current?.setLakes(mapLakes, {
      risk: (level) => t.lakeRisk(t.risk[level]),
      score: (s) => t.mapScore(num(s, lang)),
      distance: (km) => `${t.straightKm(num(Math.round(km), lang, 0))} ${t.straightLabel}`,
    });
  }, [status, mapLakes, lang]);

  useEffect(() => {
    if (status !== 'ready') return;
    handle.current?.setView(view);
  }, [status, view]);

  useEffect(() => {
    if (status !== 'ready') return;
    handle.current?.setBasemap(basemap);
  }, [status, basemap]);

  const pick = (b: Basemap) => {
    save('basemap', b);
    setBasemap(b);
  };

  const flood = view.floodLakes.length > 0;
  return (
    <section class="map-area" aria-label={sel ? t.mapLabel(sel.label) : t.mapLakesLabel}>
      <div class="map-box">
        <div ref={box} class="map" />

        {status !== 'ready' && (
          <div class="map-status" role="status">
            <IconMap size={28} />
            {status === 'held' ? (
              <>
                <p>{t.mapData}</p>
                <button type="button" class="btn btn-secondary" onClick={() => setStatus('loading')}>
                  {t.mapShow}
                </button>
              </>
            ) : (
              <p>{status === 'error' ? t.mapUnavailable : t.mapLoading}</p>
            )}
          </div>
        )}

        {status === 'ready' && (
          <div class="basemap" role="group" aria-label={t.basemapLabel}>
            <button type="button" aria-pressed={basemap === 'satellite'} onClick={() => pick('satellite')}>
              {t.basemapSatellite}
            </button>
            <button type="button" aria-pressed={basemap === 'map'} onClick={() => pick('map')}>
              {t.basemapMap}
            </button>
          </div>
        )}
      </div>

      {status === 'ready' && (
          <div class="map-legend">
            <p class="sr-only">{t.legend}</p>
            <ul aria-label={t.riskLabel}>
              <li class="map-legend-head">{t.riskLabel}</li>
              {LEVELS.map((lv) => (
                <li key={lv}>
                  <span class={`sw sw-dot sw-${lv}`} aria-hidden="true" />
                  {t.risk[lv]}
                </li>
              ))}
            </ul>
            {(view.place || flood) && (
              <ul>
                {view.place && (
                  <li>
                    <span class="sw sw-place" aria-hidden="true" />
                    {t.legendVillage}
                  </li>
                )}
                {flood && (
                  <>
                    <li>
                      <span class={`sw sw-path${basemap === 'map' ? ' sw-path-map' : ''}`} aria-hidden="true" />
                      {t.legendPath}
                    </li>
                    <li>
                      <span class="sw sw-expected" aria-hidden="true" />
                      {t.legendExpected}
                    </li>
                    <li>
                      <span class="sw sw-severe" aria-hidden="true" />
                      {t.legendSevere}
                    </li>
                  </>
                )}
              </ul>
            )}
          </div>
      )}
    </section>
  );
}
