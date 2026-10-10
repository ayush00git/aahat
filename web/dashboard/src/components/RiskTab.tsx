import { dateOnly, dateTime, discharge, factorValue, km2, LEVEL_LABEL, num, relationName, volume } from "../format";
import type { Factor, RiskFile, WeatherOutlook } from "../types";
import { ReplayChart } from "./charts";
import { LevelBadge, ScoreBar, Section, SourceText, WeatherBadge } from "./ui";

/** "08 Oct" from an ISO day. */
function shortDay(iso: string): string {
  const t = new Date(iso + "T00:00:00Z");
  if (Number.isNaN(t.getTime())) return iso;
  return t.toLocaleDateString("en-IN", { day: "2-digit", month: "short", timeZone: "UTC" });
}

/** Short-term context next to the seasonal score: the Open-Meteo outlook and the trigger it sets. */
function WeatherOutlookSection({ w }: { w: WeatherOutlook }) {
  const days = Array.isArray(w.days) ? w.days : [];
  const reasons = w.trigger?.reasons ?? [];
  return (
    <Section
      title="Weather outlook (next 3 days)"
      aside={
        <span class="aside-row">
          {w.stale && (
            <span class="wx-stale" title="The latest fetch failed: this is the last outlook that was received">
              stale
            </span>
          )}
          {w.trigger?.level && <WeatherBadge level={w.trigger.level} />}
        </span>
      }
    >
      {reasons.length > 0 ? (
        <ul class="wx-reasons">
          {reasons.map((r, i) => (
            <li key={i}>{r}</li>
          ))}
        </ul>
      ) : (
        <p class="muted">No weather trigger in the forecast.</p>
      )}
      {days.length > 0 && (
        <table class="table compact weather">
          <thead>
            <tr>
              <th>Date</th>
              <th class="num">Rain mm</th>
              <th class="num">Snow cm</th>
              <th class="num" title="Daily maximum / minimum air temperature">
                Tmax / Tmin °C
              </th>
            </tr>
          </thead>
          <tbody>
            {days.map((d) => (
              <tr key={d.date} class={d.forecast ? undefined : "row-dim"}>
                <td>
                  {shortDay(d.date)}
                  {!d.forecast && <span class="small"> · past</span>}
                </td>
                <td class="num">{num(d.precipitation_mm, 1)}</td>
                <td class="num">{num(d.snowfall_cm, 1)}</td>
                <td class="num">
                  {num(d.tmax_c, 1)} / {num(d.tmin_c, 1)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p class="caption">
        Source: <SourceText text={w.source || "Open-Meteo"} />
        {w.fetched_at ? ` · fetched ${dateTime(w.fetched_at)}` : ""}
        {w.stale ? " · stale" : ""}
      </p>
      {w.trigger?.rule && (
        <details class="more">
          <summary>Rule</summary>
          <p>{w.trigger.rule}</p>
        </details>
      )}
    </Section>
  );
}

const GROUP_TITLE: Record<string, string> = {
  size: "Size · how big a flood could be",
  likelihood: "Likelihood · how prone the lake is to burst",
};

function FactorRow({ f }: { f: Factor }) {
  return (
    <li class="factor">
      <div class="factor-head">
        <span class="factor-name">
          {f.label}{" "}
          <span class="hi" lang="hi">
            {f.label_hi}
          </span>
        </span>
        <span class="factor-value" title={f.value === null && f.note ? f.note : undefined}>
          {factorValue(f.value, f.unit)}
        </span>
      </div>
      <div class="factor-score">
        <ScoreBar value={f.score} label={`${f.label} score`} />
        <span class="factor-score-num" title="Factor score, 0 to 1">
          {num(f.score, 2)}
        </span>
        <span class="factor-weight" title="Weight within its group">
          w {num(f.weight, 2)}
        </span>
      </div>
      <details class="more">
        <summary>Details</summary>
        <dl class="more-dl">
          <dt>Rule</dt>
          <dd>{f.rule}</dd>
          {f.note && (
            <>
              <dt>Here</dt>
              <dd>{f.note}</dd>
            </>
          )}
          <dt>Why</dt>
          <dd>{f.why}</dd>
          <dt>Source</dt>
          <dd>
            <SourceText text={f.source} />
          </dd>
        </dl>
      </details>
    </li>
  );
}

export function RiskTab({ risk, weather }: { risk: RiskFile; weather?: WeatherOutlook | null }) {
  const r = risk.latest;
  const groups = ["size", "likelihood"].concat(
    [...new Set(r.factors.map((f) => f.group))].filter((g) => g !== "size" && g !== "likelihood"),
  );
  const groupScore = (g: string) => (g === "size" ? r.size : g === "likelihood" ? r.likelihood : null);
  // Null on records written before the pipeline named the relation.
  const peakRelation = relationName(r.peak_discharge_relation);
  // Small lakes: one relation gives both peaks, so the severe peak is also the expected one.
  const samePeak =
    r.peak_expected_m3s != null &&
    r.peak_expected_m3s === r.peak_discharge_m3s &&
    !!r.peak_expected_relation &&
    r.peak_expected_relation === r.peak_discharge_relation;

  return (
    <div class="tab-body">
      <div class="score-hero">
        <div class="score-hero-main">
          <span class="score-big">{num(r.score, 1)}</span>
          <LevelBadge level={r.level} />
        </div>
        <div class="score-formula" aria-label="Score formula">
          <code>
            {num(r.score, 1)} = 100 × {num(r.size, 3)} × {num(r.likelihood, 3)}
          </code>
          <span class="muted">score = 100 × size × likelihood</span>
        </div>
        <dl class="kv kv-3">
          <div>
            <dt>Volume</dt>
            <dd>{volume(r.volume_m3)}</dd>
          </div>
          <div>
            <dt>Peak discharge (severe)</dt>
            <dd>
              {discharge(r.peak_discharge_m3s)}
              {peakRelation && (
                <span
                  class="muted small block dd-sub"
                  title={`${r.peak_discharge_relation}${samePeak ? ". The expected scenario has the same peak for a lake this small." : ""}`}
                >
                  {peakRelation}
                </span>
              )}
            </dd>
          </div>
          <div>
            <dt>Data until</dt>
            <dd>{dateOnly(r.data_until)}</dd>
          </div>
        </dl>
        <details class="more">
          <summary>Details</summary>
          <dl class="more-dl">
            <dt>Season</dt>
            <dd>{r.as_of_season}</dd>
            <dt>Area ({r.area_year ?? "—"})</dt>
            <dd>{km2(r.area_m2)}</dd>
            <dt>Levels</dt>
            <dd>
              {(["very_high", "high", "moderate"] as const)
                .map((k) => `${LEVEL_LABEL[k]} ≥ ${risk.method.levels[k]}`)
                .join(" · ")}
            </dd>
          </dl>
        </details>
      </div>

      {weather && <WeatherOutlookSection w={weather} />}

      {groups.map((g) => {
        const fs = r.factors.filter((f) => f.group === g);
        if (!fs.length) return null;
        return (
          <Section
            key={g}
            title={GROUP_TITLE[g] ?? g}
            aside={
              <span class="group-score" title={fs.length > 1 ? "Weighted mean of the factor scores" : undefined}>
                {g} = {num(groupScore(g), 3)}
              </span>
            }
          >
            <ul class="factors">
              {fs.map((f) => (
                <FactorRow key={f.key} f={f} />
              ))}
            </ul>
          </Section>
        );
      })}

      <Section title="Score by season">
        <ReplayChart replay={risk.replay} levels={risk.method.levels} />
        <p class="caption">Each season scored only with the data available then.</p>
        {risk.method.hindsight && (
          <details class="more">
            <summary>About hindsight</summary>
            <p>{risk.method.hindsight}</p>
          </details>
        )}
      </Section>

      <Section title="Method">
        <p class="note">{risk.method.disclaimer}</p>
        <details class="more">
          <summary>Formula</summary>
          <p>{risk.method.formula}</p>
        </details>
      </Section>
    </div>
  );
}
