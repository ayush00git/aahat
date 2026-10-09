import { useI18n } from '../i18n';
import { altName, dateTime, placeName } from '../format';
import { href } from '../router';
import { usePlace, type PlaceState } from '../usePlace';
import { ThreatCard } from './ThreatCard';
import { Actions, EmergencyNumbers } from './Actions';
import { MySubscriptions } from './MySubscriptions';

export function PlaceView({ osm }: { osm: string }) {
  const { t, lang } = useI18n();
  const state = usePlace(osm);

  if (state.kind !== 'ok') return <PlaceStatus state={state} />;
  const { data, savedAt } = state;
  const name = placeName(data, lang);
  const alt = altName(data, lang);

  return (
    <div class="stack">
      <BackLink />
      <div>
        <h1 class="place-name">{name}</h1>
        {alt && <p class="place-alt">{alt}</p>}
      </div>

      {savedAt && (
        <p class="banner banner-offline" role="status">
          {t.savedCopy(dateTime(savedAt, lang))}
        </p>
      )}

      {!data.known && (
        <section class="card card-neutral" aria-live="polite">
          <h2 class="card-title">{t.unknownTitle}</h2>
          <p>{t.unknownBody}</p>
        </section>
      )}

      {data.known && data.safe && (
        <section class="card card-safe" aria-live="polite">
          <h2 class="card-title">
            <span class="status-icon" aria-hidden="true">
              ✓
            </span>
            {t.safeTitle}
          </h2>
          <p class="small">{t.safeCaveat}</p>
        </section>
      )}

      {data.threats.length > 0 && (
        <section class="stack" aria-labelledby="danger-h">
          <h2 id="danger-h" class="section-title danger-title">
            {t.dangerTitle(data.threats.length)}
          </h2>
          {data.threats.map((th) => (
            <ThreatCard key={th.lake_id} threat={th} />
          ))}
          <div class="btn-row">
            <a class="btn btn-primary" href={href.subscribe(osm)}>
              {t.subscribeCta}
            </a>
            <a class="btn btn-secondary" href={href.map(osm)}>
              {t.openMap}
            </a>
          </div>
        </section>
      )}

      <MySubscriptions osm={osm} />

      {data.known && data.safe && (
        <a class="btn btn-secondary" href={href.subscribe(osm)}>
          {t.subscribeCta}
        </a>
      )}

      {data.threats.length > 0 && <Actions />}
      <EmergencyNumbers />
    </div>
  );
}

export function BackLink({ to }: { to?: string }) {
  const { t } = useI18n();
  return (
    <a class="back" href={to ?? href.home()}>
      <span aria-hidden="true">‹</span> {t.back}
    </a>
  );
}

export function PlaceStatus({ state }: { state: Exclude<PlaceState, { kind: 'ok' }> }) {
  const { t } = useI18n();
  return (
    <div class="stack">
      <BackLink />
      {state.kind === 'loading' ? (
        <p class="muted" role="status">
          {t.loading}
        </p>
      ) : (
        <div class="card card-neutral" role="alert">
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
