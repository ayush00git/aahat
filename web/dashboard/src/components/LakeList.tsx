import { LEVEL_COLOR } from "../colors";
import { km2, num } from "../format";
import type { Lake } from "../types";
import { Sparkline } from "./charts";
import { LevelBadge } from "./ui";

/** Ranked by risk score, highest first; unscored lakes last. */
export function rankLakes(lakes: Lake[]): Lake[] {
  return [...lakes].sort((a, b) => (b.risk?.score ?? -1) - (a.risk?.score ?? -1) || a.name.localeCompare(b.name));
}

export function LakeList({
  lakes,
  selectedId,
  onSelect,
}: {
  lakes: Lake[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}) {
  const ranked = rankLakes(lakes);
  return (
    <nav class="sidebar" aria-label="Lakes ranked by risk">
      <div class="sidebar-head">
        <h2>Watched lakes</h2>
        <span class="muted small">{lakes.length} · ranked by risk score</span>
      </div>
      <ol class="lake-list">
        {ranked.map((l, i) => {
          const latest = l.latest;
          const score = l.risk?.score ?? null;
          return (
            <li key={l.id}>
              <button
                type="button"
                class={`lake-row${l.id === selectedId ? " selected" : ""}`}
                onClick={() => onSelect(l.id)}
                aria-current={l.id === selectedId ? "true" : undefined}
              >
                <span class="lake-rank">{i + 1}</span>
                <span class="lake-main">
                  <span class="lake-name">{l.name}</span>
                  <span class="lake-hi hi" lang="hi">
                    {l.name_hi}
                  </span>
                  <span class="lake-meta">
                    {l.district} · {l.kind}
                  </span>
                </span>
                <span class="lake-risk">
                  <span class="lake-score">{num(score, 1)}</span>
                  <LevelBadge level={l.risk?.level} compact />
                </span>
                <span class="lake-foot">
                  <Sparkline years={l.years} />
                  <span class="lake-area">
                    {latest ? km2(latest.area_m2) : "—"}
                    {latest && <span class="muted"> in {latest.year}</span>}
                  </span>
                </span>
                {score !== null && l.risk && (
                  <span class="lake-meter" aria-hidden="true">
                    <span style={{ width: `${Math.max(0, Math.min(100, score))}%`, background: LEVEL_COLOR[l.risk.level] }} />
                  </span>
                )}
              </button>
            </li>
          );
        })}
      </ol>
      <div class="sidebar-legend muted">
        Sparkline: lake area by year; hollow points are partial-coverage years. Bar: risk score out of 100.
      </div>
    </nav>
  );
}
