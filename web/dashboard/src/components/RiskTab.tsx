import { dateOnly, discharge, factorValue, km2, LEVEL_LABEL, num, volume } from "../format";
import type { Factor, RiskFile } from "../types";
import { ReplayChart } from "./charts";
import { LevelBadge, ScoreBar, Section, SourceText } from "./ui";

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
        <span class="factor-value">{factorValue(f.value, f.unit)}</span>
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

export function RiskTab({ risk }: { risk: RiskFile }) {
  const r = risk.latest;
  const groups = ["size", "likelihood"].concat(
    [...new Set(r.factors.map((f) => f.group))].filter((g) => g !== "size" && g !== "likelihood"),
  );
  const groupScore = (g: string) => (g === "size" ? r.size : g === "likelihood" ? r.likelihood : null);

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
            <dd>{discharge(r.peak_discharge_m3s)}</dd>
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
