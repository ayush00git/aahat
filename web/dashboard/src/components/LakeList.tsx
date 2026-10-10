import { num } from "../format";
import type { Lake, WeatherBrief } from "../types";
import { LevelBadge } from "./ui";

/** Ranked by risk score, highest first; unscored lakes last. */
export function rankLakes(lakes: Lake[]): Lake[] {
  return [...lakes].sort((a, b) => (b.risk?.score ?? -1) - (a.risk?.score ?? -1) || a.name.localeCompare(b.name));
}

export function LakeList({
  lakes,
  selectedId,
  onSelect,
  weather,
}: {
  lakes: Lake[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  /** Weather trigger per lake id; only elevated and high are marked. */
  weather?: Map<string, WeatherBrief>;
}) {
  const ranked = rankLakes(lakes);
  return (
    <nav class="sidebar" aria-label="Lakes ranked by risk">
      <div class="sidebar-head">
        <h2>Lakes</h2>
        <span class="muted">{lakes.length} · by risk</span>
      </div>
      <ol class="lake-list">
        {ranked.map((l) => {
          const w = weather?.get(l.id);
          const wx = w && (w.level === "elevated" || w.level === "high") ? w : null;
          const wxTitle = wx ? `Weather ${wx.level}${wx.reasons?.[0] ? `: ${wx.reasons[0]}` : ""}` : "";
          return (
          <li key={l.id}>
            <button
              type="button"
              class={`lake-row${l.id === selectedId ? " selected" : ""}`}
              onClick={() => onSelect(l.id)}
              aria-current={l.id === selectedId ? "true" : undefined}
              title={`${l.district} · ${l.kind}${wx ? `\n${wxTitle}` : ""}`}
            >
              <span class="lake-main">
                <span class="lake-name-row">
                  <span class="lake-name">{l.name}</span>
                  {wx && <span class={`wx-dot wx-dot-${wx.level}`} role="img" aria-label={wxTitle} title={wxTitle} />}
                </span>
                <span class="lake-hi hi" lang="hi">
                  {l.name_hi}
                </span>
              </span>
              <span class="lake-risk">
                <span class="lake-score">{num(l.risk?.score ?? null, 1)}</span>
                <LevelBadge level={l.risk?.level} compact />
              </span>
            </button>
          </li>
          );
        })}
      </ol>
    </nav>
  );
}
