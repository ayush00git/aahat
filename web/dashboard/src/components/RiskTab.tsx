import { dateOnly, discharge, factorValue, km2, LEVEL_LABEL, num, volume } from "../format";
import type { Factor, RiskFile } from "../types";
import { ReplayChart } from "./charts";
import { LevelBadge, ScoreBar, Section, SourceText } from "./ui";

const GROUP_TITLE: Record<string, string> = {
  size: "Size: how big a flood could be",
  likelihood: "Likelihood: how prone the lake is to burst",
};

function FactorRow({ f }: { f: Factor }) {
  return (
    <li class="factor">
      <div class="factor-head">
        <div class="factor-label">
          <span class="factor-name">{f.label}</span>{" "}
          <span class="hi" lang="hi">
            {f.label_hi}
          </span>
        </div>
        <div class="factor-value">{factorValue(f.value, f.unit)}</div>
      </div>
      <div class="factor-score">
        <ScoreBar value={f.score} label={`${f.label} score`} />
        <span class="factor-score-num">{num(f.score, 3)}</span>
        <span class="factor-weight" title="Weight within its group">
          × w {num(f.weight, 2)}
        </span>
      </div>
      <div class="factor-rule">
        <span class="k">Rule</span> {f.rule}
      </div>
      {f.note && (
        <div class="factor-note">
          <span class="k">Here</span> {f.note}
        </div>
      )}
      <details class="factor-more">
        <summary>Why it matters · source</summary>
        <p>{f.why}</p>
        <p class="factor-source">
          <span class="k">Source</span> <SourceText text={f.source} />
        </p>
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
          <div class="score-big">{num(r.score, 1)}</div>
          <LevelBadge level={r.level} />
        </div>
        <div class="score-formula" aria-label="Score formula">
          <span class="mono">
            {num(r.score, 1)} = 100 × {num(r.size, 3)} × {num(r.likelihood, 3)}
          </span>
          <span class="muted">score = 100 × size × likelihood</span>
        </div>
        <dl class="kv kv-3">
          <div>
            <dt>Season</dt>
            <dd>{r.as_of_season}</dd>
          </div>
          <div>
            <dt>Data until</dt>
            <dd>{dateOnly(r.data_until)}</dd>
          </div>
          <div>
            <dt>Area ({r.area_year ?? "—"})</dt>
            <dd>{km2(r.area_m2)}</dd>
          </div>
          <div>
            <dt>Volume</dt>
            <dd>{volume(r.volume_m3)}</dd>
          </div>
          <div>
            <dt>Peak discharge (severe)</dt>
            <dd>{discharge(r.peak_discharge_m3s)}</dd>
          </div>
          <div>
            <dt>Levels</dt>
            <dd class="small">
              {(["very_high", "high", "moderate"] as const)
                .map((k) => `${LEVEL_LABEL[k]} ≥ ${risk.method.levels[k]}`)
                .join(" · ")}
            </dd>
          </div>
        </dl>
      </div>

      {groups.map((g) => {
        const fs = r.factors.filter((f) => f.group === g);
        if (!fs.length) return null;
        return (
          <Section
            key={g}
            title={GROUP_TITLE[g] ?? g}
            aside={
              <span class="mono">
                {g} = {num(groupScore(g), 3)}
              </span>
            }
          >
            {fs.length > 1 && <p class="muted small group-note">Weighted mean of the factor scores below.</p>}
            <ul class="factors">
              {fs.map((f) => (
                <FactorRow key={f.key} f={f} />
              ))}
            </ul>
          </Section>
        );
      })}

      <Section title="Replay: score by season">
        <ReplayChart replay={risk.replay} levels={risk.method.levels} />
        <p class="caption">Each year scored only with data available then.</p>
        {risk.method.hindsight && <p class="muted small">{risk.method.hindsight}</p>}
      </Section>

      <Section title="Method">
        <p class="small">{risk.method.formula}</p>
        <p class="small disclaimer-inline">{risk.method.disclaimer}</p>
      </Section>
    </div>
  );
}
