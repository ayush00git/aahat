import type { Threat } from '../api';
import { useI18n } from '../i18n';
import { lakeName, num, wholeMinutes } from '../format';

export function ThreatCard({ threat: th }: { threat: Threat }) {
  const { t, lang } = useI18n();
  const lake = lakeName(th, lang);
  const red = th.status === 'in_flood_path';
  const fast = th.arrival_min_fast;
  const expected = th.arrival_min_expected;
  const titleId = `lake-${th.lake_id}`;

  return (
    <article class={`card threat ${red ? 'threat-red' : 'threat-orange'}`} aria-labelledby={titleId}>
      <div class="threat-head">
        <span class={`badge ${red ? 'badge-red' : 'badge-orange'}`}>
          {red ? t.badge_in_flood_path : t.badge_at_risk}
        </span>
      </div>
      <h3 id={titleId} class="lake-name">
        {t.lakeLine(lake)}
      </h3>

      {fast !== null ? (
        <div class="arrival">
          <p class="arrival-num">
            ~{num(wholeMinutes(fast), lang, 0)} <span class="arrival-unit">{t.minutes}</span>
          </p>
          <p class="arrival-label">{t.arrivalLabel}</p>
        </div>
      ) : (
        <p class="arrival-label">{t.arrivalUnknown}</p>
      )}

      <ul class="facts">
        {expected !== null && <li>{t.arrivalExpected(wholeMinutes(expected))}</li>}
        {th.km !== null && <li>{t.distance(num(th.km, lang, 0))}</li>}
      </ul>

      <div class="flood">
        <h4 class="flood-title">{t.floodTitle}</h4>
        {(['expected', 'severe'] as const).map((sc) => (
          <Scenario key={sc} threat={th} scenario={sc} />
        ))}
      </div>

      {th.risk_level && (
        <p class="risk">
          {t.riskLabel}:{' '}
          <strong class={`risk-${th.risk_level}`}>{t.risk[th.risk_level]}</strong>
        </p>
      )}
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
        {scenario === 'expected' ? t.scenario_expected : t.scenario_severe}
        {outcome && <span class="scenario-outcome">: {outcome}</span>}
      </p>
      <ul class="scenario-facts">
        {depth !== null && <li>{t.riverDepth(num(depth, lang))}</li>}
        {ground && <li>{ground}</li>}
      </ul>
    </div>
  );
}
