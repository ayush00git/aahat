import { useEffect, useState } from 'preact/hooks';
import { getWeather, type LakeWeather, type Threat } from '../api';
import { useI18n } from '../i18n';
import { lakeName, num, wholeMinutes } from '../format';
import { IconClock, IconFlood, IconNearby, IconRain, IconRiver, IconWarn } from './icons';

/**
 * The lake's weather trigger from GET /weather (fetched once, shared): one
 * line when the level is elevated or high. `quiet` also shows a plain line for
 * a normal outlook, unless that answer is stale. No numbers: only the level
 * the API returned. Renders nothing while loading or when the API has no answer.
 */
export function WeatherLine({ lakeId, quiet }: { lakeId: string; quiet?: boolean }) {
  const { t } = useI18n();
  const [w, setW] = useState<LakeWeather | null>(null);
  useEffect(() => {
    let live = true;
    setW(null);
    getWeather().then((m) => live && setW(m.get(lakeId) ?? null));
    return () => {
      live = false;
    };
  }, [lakeId]);
  if (!w) return null;
  const text =
    w.level === 'high' ? t.weatherHigh : w.level === 'elevated' ? t.weatherElevated : w.level === 'normal' && quiet && !w.stale ? t.weatherNormal : null;
  if (!text) return null;
  return (
    <p class={`weather-line weather-${w.level}`}>
      <IconRain size={20} />
      <span>
        {text} <span class="weather-src">{t.weatherSource}</span>
      </span>
    </p>
  );
}

export function ThreatCard({ threat: th }: { threat: Threat }) {
  const { t, lang } = useI18n();
  const lake = lakeName(th, lang);
  const red = th.status === 'in_flood_path';
  const fast = th.arrival_min_fast;
  const expected = th.arrival_min_expected;
  const titleId = `lake-${th.lake_id}`;

  return (
    <article class={`scard ${red ? 'scard-red' : 'scard-orange'}`} aria-labelledby={titleId}>
      <p class="scard-band">
        {red ? <IconFlood size={26} /> : <IconWarn size={26} />}
        <span>{red ? t.badge_in_flood_path : t.badge_at_risk}</span>
      </p>
      <div class="scard-body">
        <h3 id={titleId} class="lake-name">
          {t.lakeLine(lake)}
        </h3>

        {fast !== null ? (
          <div class="arrival">
            <p class="arrival-num">
              <span class="arrival-tilde">~</span>
              {num(wholeMinutes(fast), lang, 0)} <span class="arrival-unit">{t.minutes}</span>
            </p>
            <p class="arrival-label">{t.arrivalLabel}</p>
          </div>
        ) : (
          <p class="arrival-label">{t.arrivalUnknown}</p>
        )}

        {(expected !== null || th.km !== null) && (
          <ul class="facts">
            {expected !== null && (
              <li>
                <IconClock size={20} />
                {t.arrivalExpected(wholeMinutes(expected))}
              </li>
            )}
            {th.km !== null && (
              <li>
                <IconRiver size={20} />
                {t.distance(num(th.km, lang, 0))}
              </li>
            )}
          </ul>
        )}

        <WeatherLine lakeId={th.lake_id} quiet />

        <div class="flood">
          <h4 class="flood-title">{t.floodTitle}</h4>
          {(['expected', 'severe'] as const).map((sc) => (
            <Scenario key={sc} threat={th} scenario={sc} />
          ))}
        </div>

        {th.risk_level && (
          <p class="risk-row">
            <span>{t.riskLabel}</span>
            <span class={`pill pill-risk risk-${th.risk_level}`}>{t.risk[th.risk_level]}</span>
          </p>
        )}
      </div>
    </article>
  );
}

function Scenario({ threat: th, scenario }: { threat: Threat; scenario: 'expected' | 'severe' }) {
  const { t, lang } = useI18n();
  const status = th.scenario_status[scenario];
  const depth = th.flood_depth_m[scenario];
  const above = th.height_above_flood_m[scenario];
  if (!status && depth === null && above === null) return null;

  const outcome =
    status === 'flooded' ? t.outcome_flooded : status === 'margin' ? t.outcome_margin : status === 'outside' ? t.outcome_outside : null;
  let ground: string | null = null;
  if (above !== null) {
    const m = num(Math.abs(above), lang);
    ground = m === '0' ? t.villageAt : above < 0 ? t.villageBelow(m) : t.villageAbove(m);
  }

  return (
    <div class={`scenario scenario-${status || 'unknown'}`}>
      <p class="scenario-name">
        <span class="scenario-tag">{scenario === 'expected' ? t.scenario_expected : t.scenario_severe}</span>
        {outcome && <span class="scenario-outcome">{outcome}</span>}
      </p>
      {(depth !== null || ground) && (
        <ul class="scenario-facts">
          {depth !== null && <li>{t.riverDepth(num(depth, lang))}</li>}
          {ground && <li>{ground}</li>}
        </ul>
      )}
    </div>
  );
}

/**
 * A lake whose flood passes near the village without reaching its mapped
 * point. Uses the severe-scenario height (the lower, more cautious one).
 */
export function NearbyCard({ threat: th }: { threat: Threat }) {
  const { t, lang } = useI18n();
  const fast = th.arrival_min_fast;
  const h = th.height_above_flood_m.severe;
  const titleId = `near-${th.lake_id}`;
  return (
    <article class="scard scard-amber" aria-labelledby={titleId}>
      <p class="scard-band">
        <IconNearby size={26} />
        <span>{t.nearbyBadge}</span>
      </p>
      <div class="scard-body">
        <h3 id={titleId} class="lake-name">
          {t.lakeLine(lakeName(th, lang))}
        </h3>
        <p>
          {t.nearbyText(
            lakeName(th, lang),
            fast !== null ? wholeMinutes(fast) : null,
            h !== null && h > 0 ? Math.round(h) : null,
          )}
        </p>
        {(th.lateral_m != null || th.km !== null) && (
          <ul class="facts">
            {th.lateral_m != null && (
              <li>
                <IconNearby size={20} />
                {t.nearbyRiver(num(th.lateral_m, lang, 0))}
              </li>
            )}
            {th.km !== null && (
              <li>
                <IconRiver size={20} />
                {t.distance(num(th.km, lang, 0))}
              </li>
            )}
          </ul>
        )}
        {th.risk_level && (
          <p class="risk-row">
            <span>{t.riskLabel}</span>
            <span class={`pill pill-risk risk-${th.risk_level}`}>{t.risk[th.risk_level]}</span>
          </p>
        )}
      </div>
    </article>
  );
}
