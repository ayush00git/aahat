import { useEffect, useState } from 'preact/hooks';
import { getLakes, type LakeSummary, type Threats } from '../api';
import { STRINGS, useI18n } from '../i18n';
import { altName, dateTime, placeName } from '../format';
import { href } from '../router';
import { rememberedNames } from '../coords';
import { usePlace, type PlaceState } from '../usePlace';
import { NearbyCard, ThreatCard } from './ThreatCard';
import { Actions, EmergencyNumbers } from './Actions';
import { MySubscriptions } from './MySubscriptions';
import { IconBack, IconBell, IconCheck, IconMap, IconUnknown } from './icons';

/** Names for the heading: the API's, else the ones search gave (uncovered places). */
function names(data: Threats) {
  if (data.name || data.name_hi) return data;
  return { ...data, ...rememberedNames(data.osm) };
}

export function PlaceView({ osm }: { osm: string }) {
  const { t, lang } = useI18n();
  const state = usePlace(osm);

  if (state.kind !== 'ok') return <PlaceStatus state={state} />;
  const { data, savedAt } = state;
  const named = names(data);
  const name = placeName(named, lang);
  const alt = altName(named, lang);
  const nearby = data.nearby ?? [];
  const danger = data.threats.length > 0;

  return (
    <div class="stack">
      <BackLink />
      <div class="place-head">
        <h1 class="place-name">{name}</h1>
        {alt && <p class="place-alt">{alt}</p>}
      </div>

      {savedAt && (
        <p class="banner banner-offline" role="status">
          {t.savedCopy(dateTime(savedAt, lang))}
        </p>
      )}

      {!data.known && <NotCovered />}

      {data.known && data.safe && (
        <section class="status status-safe" aria-live="polite">
          <div class="status-head">
            <span class="status-badge" aria-hidden="true">
              <IconCheck size={30} />
            </span>
            <h2 class="status-title">{t.safeTitle}</h2>
          </div>
          <p class="small">{t.safeCaveat}</p>
        </section>
      )}

      {danger && (
        <section class="stack" aria-labelledby="danger-h">
          <h2 id="danger-h" class="section-title danger-title">
            {t.dangerTitle(data.threats.length)}
          </h2>
          {data.threats.map((th) => (
            <ThreatCard key={th.lake_id} threat={th} />
          ))}
        </section>
      )}

      {nearby.length > 0 && (
        <section class="stack" aria-labelledby="near-h">
          <h2 id="near-h" class="section-title nearby-title">
            {t.nearbyTitle}
          </h2>
          {nearby.map((th) => (
            <NearbyCard key={th.lake_id} threat={th} />
          ))}
        </section>
      )}

      {data.known && (
        <div class="btn-row">
          <a class={`btn ${danger ? 'btn-primary' : 'btn-secondary'}`} href={href.subscribe(osm)}>
            <IconBell size={22} />
            {t.subscribeCta}
          </a>
          {(danger || nearby.length > 0) && (
            <a class="btn btn-secondary" href={href.map(osm)}>
              <IconMap size={22} />
              {t.openMap}
            </a>
          )}
        </div>
      )}

      <MySubscriptions osm={osm} />

      {(danger || nearby.length > 0) && <Actions />}
      <EmergencyNumbers />
    </div>
  );
}

/**
 * A place no monitored lake's analysis reaches: say so plainly (in both
 * languages), list the lakes we do watch, and note that coverage is growing.
 */
function NotCovered() {
  const { t, lang } = useI18n();
  const other = STRINGS[lang === 'hi' ? 'en' : 'hi'];
  const [lakes, setLakes] = useState<LakeSummary[] | null | 'error'>(null);

  useEffect(() => {
    let live = true;
    getLakes().then(
      (ls) => live && setLakes(ls),
      () => live && setLakes('error'),
    );
    return () => {
      live = false;
    };
  }, []);

  return (
    <section class="status status-unknown" aria-live="polite" aria-labelledby="nc-h">
      <div class="status-head">
        <span class="status-badge" aria-hidden="true">
          <IconUnknown size={30} />
        </span>
        <h2 id="nc-h" class="status-title">
          {t.notCoveredTitle}
        </h2>
      </div>
      <p class="status-alt" lang={lang === 'hi' ? 'en' : 'hi'}>
        {other.notCoveredTitle}
      </p>
      <p class="small">{t.notCoveredBody}</p>
      <div class="lakes">
        <h3 class="section-label">{t.monitoredLakes}</h3>
        {lakes === null && <p class="small muted">{t.loading}</p>}
        {lakes === 'error' && <p class="small muted">{t.lakesError}</p>}
        {Array.isArray(lakes) && (
          <ul class="lake-list">
            {lakes.map((l) => (
              <li key={l.id} class="lake-item">
                <span class="lake-item-row">
                  <span class="lake-item-name">{lang === 'hi' ? l.name_hi || l.name : l.name}</span>
                  {l.risk_level && (
                    <span class={`pill pill-risk risk-${l.risk_level}`}>{t.lakeRisk(t.risk[l.risk_level])}</span>
                  )}
                </span>
                <span class="lake-item-meta">
                  {lang === 'hi' ? l.name : l.name_hi} · {l.district}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

export function BackLink({ to }: { to?: string }) {
  const { t } = useI18n();
  return (
    <a class="back" href={to ?? href.home()}>
      <IconBack size={22} /> {t.back}
    </a>
  );
}

export function PlaceStatus({ state }: { state: Exclude<PlaceState, { kind: 'ok' }> }) {
  const { t } = useI18n();
  return (
    <div class="stack">
      <BackLink />
      {state.kind === 'loading' ? (
        <div class="skeleton" role="status">
          <span class="skel skel-title" />
          <span class="skel skel-card" />
          <p class="muted">{t.loading}</p>
        </div>
      ) : (
        <div class="note note-neutral stack-sm" role="alert">
          <p>{state.notFound ? t.unknownTitle : t.loadError}</p>
          {!state.notFound && (
            <button type="button" class="btn btn-secondary" onClick={state.retry}>
              {t.retry}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
