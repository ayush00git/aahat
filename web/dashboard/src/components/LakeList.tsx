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
        <span class="muted">{lakes.length} · by risk score</span>
      </div>
      <ol class="lake-list">
        {ranked.map((l, i) => {
          const latest = l.latest;
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
                  <span class="lake-name">
                    {l.name}{" "}
                    <span class="hi" lang="hi">
                      {l.name_hi}
                    </span>
                  </span>
                  <span class="lake-meta">
                    {l.district} · {l.kind}
                  </span>
                  <span class="lake-area">
                    {latest ? km2(latest.area_m2) : "—"}
                    {latest && <span class="muted"> in {latest.year}</span>}
                  </span>
                </span>
                <span class="lake-risk">
                  <span class="lake-score">{num(l.risk?.score, 1)}</span>
                  <LevelBadge level={l.risk?.level} compact />
                  <Sparkline years={l.years} />
                </span>
              </button>
            </li>
          );
        })}
      </ol>
      <div class="sidebar-legend muted">
        Sparkline: area by year. Hollow points are partial-coverage years.
      </div>
    </nav>
  );
}
