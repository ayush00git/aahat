import type { ComponentChildren } from "preact";
import { hostOf, LEVEL_LABEL, splitLinks, STATUS_LABEL } from "../format";
import type { ImpactStatus, RiskLevel } from "../types";

export function LevelBadge({ level, compact }: { level: RiskLevel | null | undefined; compact?: boolean }) {
  if (!level) return <span class="badge level-none">Unscored</span>;
  return (
    <span class={`badge level-${level}${compact ? " compact" : ""}`}>
      <span class="badge-dot" aria-hidden="true" />
      {LEVEL_LABEL[level]}
    </span>
  );
}

export function StatusBadge({ status }: { status: ImpactStatus }) {
  return (
    <span class={`badge status-${status}`}>
      <span class="badge-dot" aria-hidden="true" />
      {STATUS_LABEL[status]}
    </span>
  );
}

/** Horizontal 0–1 bar. */
export function ScoreBar({ value, label }: { value: number | null; label?: string }) {
  const v = value === null || value === undefined ? null : Math.max(0, Math.min(1, value));
  return (
    <div class="scorebar" role="meter" aria-valuemin={0} aria-valuemax={1} aria-valuenow={v ?? undefined} aria-label={label}>
      <div class="scorebar-track">
        {v !== null && <div class="scorebar-fill" style={{ width: `${v * 100}%` }} />}
        <div class="scorebar-tick" style={{ left: "33.333%" }} />
        <div class="scorebar-tick" style={{ left: "66.666%" }} />
      </div>
    </div>
  );
}

/** Citation text with each URL turned into a short link. */
export function SourceText({ text }: { text: string }) {
  return (
    <>
      {splitLinks(text).map((p, i) =>
        p.url ? (
          <a key={i} href={p.url} target="_blank" rel="noopener noreferrer" title={p.url}>
            {hostOf(p.url)}
          </a>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </>
  );
}

export function Hi({ children }: { children: ComponentChildren }) {
  if (!children) return null;
  return (
    <span class="hi" lang="hi">
      {children}
    </span>
  );
}

export function Loading({ what }: { what: string }) {
  return (
    <div class="state-msg">
      <span class="spinner" aria-hidden="true" /> Loading {what}…
    </div>
  );
}

export function ErrorMsg({ msg }: { msg: string }) {
  return (
    <div class="state-msg error" role="alert">
      {msg}
    </div>
  );
}

export function Section({ title, aside, children }: { title: string; aside?: ComponentChildren; children: ComponentChildren }) {
  return (
    <section class="section">
      <header class="section-head">
        <h3>{title}</h3>
        {aside && <div class="section-aside">{aside}</div>}
      </header>
      {children}
    </section>
  );
}
