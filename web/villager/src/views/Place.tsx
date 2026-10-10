import { useEffect, useState } from 'preact/hooks';
import { getLakes, type LakeSummary, type Threats } from '../api';
import { STRINGS, useI18n, type Lang, type Strings } from '../i18n';
import { adminLine, altName, dateTime, lakeName, num, placeName, wholeMinutes } from '../format';
import { href } from '../router';
import { placeCoords, rememberedAdmin, rememberedNames } from '../coords';
import { nearestLakes, type LonLat } from '../nearest';
import { placeLink } from '../share';
import { usePlace, type PlaceState } from '../usePlace';
import { NearbyCard, ThreatCard, WeatherLine } from './ThreatCard';
import { Actions, EmergencyNumbers } from './Actions';
import { MySubscriptions } from './MySubscriptions';
import { ShareButton } from './Share';
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
  // District from the API's answer, else from the search result that led here.
  const admin = adminLine(data.district || data.state ? data : (rememberedAdmin(osm) ?? {}), t.districtLine);
  const nearby = data.nearby ?? [];
  const danger = data.threats.length > 0;
  const shareText = shareMessage(data, name, t, lang);

  return (
    <div class="stack">
      <BackLink />
      <div class="place-head">
        <h1 class="place-name">{name}</h1>
        {alt && <p class="place-alt">{alt}</p>}
        {admin && <p class="place-admin">{admin}</p>}
      </div>

      {savedAt && (
        <p class="banner banner-offline" role="status">
          {t.savedCopy(dateTime(savedAt, lang))}
        </p>
      )}

      {!data.known && <NotCovered data={named} />}

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

      {data.known && !danger && <NearbyLakesSection data={named} />}

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
          {shareText && <ShareButton text={shareText} link={placeLink(osm)} />}
        </div>
      )}

      <MySubscriptions osm={osm} />

      {(danger || nearby.length > 0) && <Actions />}
      <EmergencyNumbers />
    </div>
  );
}

/**
 * The share message for a result, built only from the API's answer: the
 * nearest threatening lake, else a flood passing nearby, else "safe".
 * Places no lake covers get no share button.
 */
function shareMessage(data: Threats, place: string, t: Strings, lang: Lang): string | null {
  const th = data.threats[0] ?? data.nearby?.[0];
  if (th) {
    const min = th.arrival_min_fast !== null ? wholeMinutes(th.arrival_min_fast) : null;
    const lake = lakeName(th, lang);
    return data.threats.length > 0 ? t.shareThreat(lake, place, min) : t.shareNearby(lake, place, min);
  }
  return data.known && data.safe ? t.shareSafe(place) : null;
}

/** All monitored lakes (GET /lakes, saved for offline use). */
function useLakes(): LakeSummary[] | null | 'error' {
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
  return lakes;
}

/**
 * The place's position: from the threats answer, else from an earlier search
 * (or a search by name). undefined while looking, null if unknown.
 */
function useHere(data: Pick<Threats, 'osm' | 'lon' | 'lat' | 'name' | 'name_hi'>): LonLat | null | undefined {
  const direct: LonLat | null =
    typeof data.lon === 'number' && typeof data.lat === 'number' ? [data.lon, data.lat] : null;
  const [found, setFound] = useState<LonLat | null | undefined>(direct ?? undefined);
  useEffect(() => {
    if (direct) return setFound(direct);
    let live = true;
    setFound(undefined);
    placeCoords(data.osm, [data.name, data.name_hi]).then(
      (c) => live && setFound(c),
      () => live && setFound(null),
    );
    return () => {
      live = false;
    };
  }, [data.osm, direct?.[0], direct?.[1]]);
  return direct ?? found;
}

type Named = Pick<Threats, 'osm' | 'lon' | 'lat' | 'name' | 'name_hi'>;

/**
 * The three monitored lakes nearest the place, by straight-line distance,
 * with a note saying what that does (and doesn't) mean. Falls back to the
 * full list when the place's or the lakes' positions are unknown (e.g. a
 * lake list saved offline by an older version).
 */
function NearestLakeList({ data, note, fallbackAll }: { data: Named; note: string; fallbackAll: boolean }) {
  const { t } = useI18n();
  const lakes = useLakes();
  const here = useHere(data);

  if (lakes === 'error') return fallbackAll ? <p class="small muted">{t.lakesError}</p> : null;
  if (lakes === null || here === undefined) return <p class="small muted">{t.loading}</p>;

  const near = here ? nearestLakes(lakes, here, 3) : [];
  if (near.length === 0) {
    if (!fallbackAll) return null;
    return (
      <>
        <h3 class="section-label">{t.monitoredLakes}</h3>
        <LakeList lakes={lakes} />
      </>
    );
  }
  return (
    <>
      <h3 class="section-label">{t.nearestLakes}</h3>
      <p class="small">{note}</p>
      <LakeList lakes={near} />
      <p class="lake-note">{t.straightNote}</p>
    </>
  );
}

function LakeList({ lakes }: { lakes: (LakeSummary & { km?: number })[] }) {
  const { t, lang } = useI18n();
  return (
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
          {l.km !== undefined && (
            <span class="lake-item-dist">
              <strong>{t.straightKm(num(Math.round(l.km), lang, 0))}</strong> {t.straightLabel}
            </span>
          )}
          {l.km !== undefined && <WeatherLine lakeId={l.id} />}
        </li>
      ))}
    </ul>
  );
}

/**
 * A place no monitored lake's analysis reaches: say so plainly (in both
 * languages), show the nearest lakes we do watch, and note that coverage is
 * growing.
 */
function NotCovered({ data }: { data: Named }) {
  const { t, lang } = useI18n();
  const other = STRINGS[lang === 'hi' ? 'en' : 'hi'];
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
        <NearestLakeList data={data} note={t.nearestUncovered} fallbackAll />
      </div>
    </section>
  );
}

/** For covered places with no threat: the monitored lakes closest by. */
function NearbyLakesSection({ data }: { data: Named }) {
  const { t } = useI18n();
  return (
    <section class="near-lakes" aria-label={t.nearestLakes}>
      <NearestLakeList data={data} note={t.nearestSafe} fallbackAll={false} />
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
