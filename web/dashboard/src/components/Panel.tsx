import { dateOnly, km2, num } from "../format";
import type { BarrierScan, DownstreamFile, Impact, Lake, LakeLayers, RiskFile, WeatherOutlook } from "../types";
import { AlertsTab } from "./AlertsTab";
import { EvidenceTab } from "./EvidenceTab";
import { ExposureTab } from "./ExposureTab";
import { GrowthTab } from "./GrowthTab";
import { RiskTab } from "./RiskTab";
import { ErrorMsg, LevelBadge, Loading } from "./ui";

export type TabId = "risk" | "growth" | "exposure" | "evidence" | "alerts";

export const TAB_IDS: TabId[] = ["risk", "growth", "exposure", "evidence", "alerts"];

const TABS: { id: TabId; label: string }[] = [
  { id: "risk", label: "Risk" },
  { id: "growth", label: "Growth" },
  { id: "exposure", label: "Downstream exposure" },
  { id: "evidence", label: "Data & evidence" },
  { id: "alerts", label: "Alerts" },
];

export interface LakeDetail {
  risk: RiskFile | null;
  downstream: DownstreamFile | null;
  impacts: Impact[] | null;
  /** Optional extras: null when the API has no answer for the lake. */
  weather: WeatherOutlook | null;
  barrier: BarrierScan | null;
  errors: Partial<Record<"risk" | "downstream" | "impacts" | "layers", string>>;
}

export function Panel({
  lake,
  detail,
  layers,
  loading,
  tab,
  onTab,
  onClose,
  onFocusImpact,
  onFocusPoint,
}: {
  lake: Lake;
  detail: LakeDetail | null;
  layers: LakeLayers | null;
  loading: boolean;
  tab: TabId;
  onTab: (t: TabId) => void;
  onClose: () => void;
  onFocusImpact: (im: Impact) => void;
  onFocusPoint: (lon: number, lat: number) => void;
}) {
  const ds = lake.downstream;
  const drain = lake.drain;
  const settlements = ds?.exposed_counts?.settlement;
  return (
    <aside class="panel" aria-label={`${lake.name} details`}>
      <header class="panel-head">
        <div class="panel-title">
          <div class="panel-title-text">
            <p class="panel-where">
              {lake.district} · {lake.basin} · {lake.kind}
            </p>
            <h2>
              {lake.name}{" "}
              <span class="hi" lang="hi">
                {lake.name_hi}
              </span>
            </h2>
          </div>
          <button type="button" class="icon-btn close-btn" onClick={onClose} aria-label="Close lake details" title="Close">
            <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
              <path d="M3.5 3.5l9 9m0-9-9 9" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
            </svg>
          </button>
        </div>
        <dl class="panel-stats">
          <div>
            <dt>Risk score</dt>
            <dd class="panel-score">
              <span class="mono">{num(lake.risk?.score, 1)}</span>
              <LevelBadge level={lake.risk?.level} />
            </dd>
          </div>
          <div>
            <dt>Area ({lake.latest?.year ?? "—"})</dt>
            <dd>{km2(lake.latest?.area_m2)}</dd>
          </div>
          <div>
            <dt>Settlements</dt>
            <dd>
              {settlements ? (
                <>
                  {settlements.in_flood_path} in path <span class="muted">· {settlements.at_risk} at risk</span>
                </>
              ) : (
                "—"
              )}
            </dd>
          </div>
          <div>
            <dt>Data until</dt>
            <dd>{dateOnly(lake.risk?.data_until)}</dd>
          </div>
        </dl>
        {drain?.drained === true && (
          <p class="flag danger drain-flag" role="alert">
            <b>Possible sudden drainage:</b>{" "}
            {typeof drain.drop_fraction === "number"
              ? `area fell ${num(drain.drop_fraction * 100, 0)}% since the season composite`
              : "area fell sharply since the season composite"}
            {drain.as_of ? ` (satellite check of ${dateOnly(drain.as_of)})` : ""}. Verify before acting.
          </p>
        )}
      </header>
      <div class="tabs" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            id={`tab-${t.id}`}
            aria-selected={tab === t.id}
            aria-controls="tabpanel"
            class={`tab${tab === t.id ? " on" : ""}${t.id === "alerts" ? " tab-alert" : ""}`}
            onClick={() => onTab(t.id)}
          >
            {t.id === "alerts" && (
              <svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true">
                <path d="M4 11V7.5a4 4 0 0 1 8 0V11l1 1.3H3L4 11ZM6.8 14h2.4" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" stroke-linecap="round" />
              </svg>
            )}
            {t.label}
          </button>
        ))}
      </div>
      <div class="tabpanel" id="tabpanel" role="tabpanel" aria-labelledby={`tab-${tab}`}>
        {renderTab(tab, lake, detail, layers, loading, onFocusImpact, onFocusPoint)}
      </div>
    </aside>
  );
}

function renderTab(
  tab: TabId,
  lake: Lake,
  d: LakeDetail | null,
  layers: LakeLayers | null,
  loading: boolean,
  onFocus: (im: Impact) => void,
  onFocusPoint: (lon: number, lat: number) => void,
) {
  switch (tab) {
    case "risk":
      if (d?.risk) return <RiskTab risk={d.risk} weather={d.weather} />;
      if (d?.errors.risk) return <ErrorMsg msg={`Risk assessment unavailable: ${d.errors.risk}`} />;
      return loading ? <Loading what="risk assessment" /> : null;
    case "growth":
      return <GrowthTab lake={lake} risk={d?.risk ?? null} />;
    case "exposure":
      if (d?.impacts) return (
          <ExposureTab
            lake={lake}
            downstream={d.downstream}
            impacts={d.impacts}
            barrier={d.barrier}
            onFocus={onFocus}
            onFocusPoint={onFocusPoint}
          />
        );
      if (d?.errors.impacts) return <ErrorMsg msg={`Exposure unavailable: ${d.errors.impacts}`} />;
      return loading ? <Loading what="downstream exposure" /> : null;
    case "evidence":
      if (!layers && loading) return <Loading what="season records" />;
      return <EvidenceTab lake={lake} risk={d?.risk ?? null} outlines={layers?.outlines} layersError={d?.errors.layers} />;
    case "alerts":
      return <AlertsTab lake={lake} downstream={d?.downstream ?? null} />;
  }
}
