import { dateOnly, km2, num } from "../format";
import type { DownstreamFile, Impact, Lake, RiskFile } from "../types";
import { AlertsTab } from "./AlertsTab";
import { ExposureTab } from "./ExposureTab";
import { GrowthTab } from "./GrowthTab";
import { RiskTab } from "./RiskTab";
import { ErrorMsg, LevelBadge, Loading } from "./ui";

export type TabId = "risk" | "growth" | "exposure" | "alerts";

const TABS: { id: TabId; label: string }[] = [
  { id: "risk", label: "Risk" },
  { id: "growth", label: "Growth" },
  { id: "exposure", label: "Downstream exposure" },
  { id: "alerts", label: "Alerts" },
];

export interface LakeDetail {
  risk: RiskFile | null;
  downstream: DownstreamFile | null;
  impacts: Impact[] | null;
  errors: Partial<Record<"risk" | "downstream" | "impacts" | "layers", string>>;
}

export function Panel({
  lake,
  detail,
  loading,
  tab,
  onTab,
  onClose,
  onFocusImpact,
}: {
  lake: Lake;
  detail: LakeDetail | null;
  loading: boolean;
  tab: TabId;
  onTab: (t: TabId) => void;
  onClose: () => void;
  onFocusImpact: (im: Impact) => void;
}) {
  const ds = lake.downstream;
  const settlements = ds?.exposed_counts?.settlement;
  return (
    <aside class="panel" aria-label={`${lake.name} details`}>
      <header class="panel-head">
        <div class="panel-title">
          <h2>
            {lake.name}{" "}
            <span class="hi" lang="hi">
              {lake.name_hi}
            </span>
          </h2>
          <button type="button" class="icon-btn" onClick={onClose} aria-label="Close lake details" title="Close">
            ×
          </button>
        </div>
        <div class="panel-meta">
          <LevelBadge level={lake.risk?.level} />
          <span>
            score <b>{num(lake.risk?.score, 1)}</b>
          </span>
          <span>{lake.district}</span>
          <span>{lake.basin}</span>
          <span>{lake.kind}</span>
        </div>
        <div class="panel-meta muted small">
          <span>
            {km2(lake.latest?.area_m2)} in {lake.latest?.year ?? "—"}
          </span>
          {settlements && (
            <span>
              settlements: {settlements.in_flood_path} in flood path, {settlements.at_risk} at risk
            </span>
          )}
          <span>data until {dateOnly(lake.risk?.data_until)}</span>
        </div>
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
            {t.label}
          </button>
        ))}
      </div>
      <div class="tabpanel" id="tabpanel" role="tabpanel" aria-labelledby={`tab-${tab}`}>
        {renderTab(tab, lake, detail, loading, onFocusImpact)}
      </div>
    </aside>
  );
}

function renderTab(tab: TabId, lake: Lake, d: LakeDetail | null, loading: boolean, onFocus: (im: Impact) => void) {
  switch (tab) {
    case "risk":
      if (d?.risk) return <RiskTab risk={d.risk} />;
      if (d?.errors.risk) return <ErrorMsg msg={`Risk assessment unavailable: ${d.errors.risk}`} />;
      return loading ? <Loading what="risk assessment" /> : null;
    case "growth":
      return <GrowthTab lake={lake} risk={d?.risk ?? null} />;
    case "exposure":
      if (d?.impacts) return <ExposureTab lake={lake} downstream={d.downstream} impacts={d.impacts} onFocus={onFocus} />;
      if (d?.errors.impacts) return <ErrorMsg msg={`Exposure unavailable: ${d.errors.impacts}`} />;
      return loading ? <Loading what="downstream exposure" /> : null;
    case "alerts":
      return <AlertsTab lake={lake} downstream={d?.downstream ?? null} />;
  }
}
