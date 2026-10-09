import type { ComponentChildren } from "preact";
import { dateOnly, dateTime, num } from "../format";
import type { ImpactKind, Lake, RiskLevel } from "../types";
import { AuthStatus } from "./SignIn";

/** The app mark: a peak above a glacial lake. */
function Mark() {
  return (
    <svg class="brand-mark" viewBox="0 0 32 32" width="30" height="30" aria-hidden="true">
      <rect width="32" height="32" rx="7" fill="#15263d" />
      <path d="M5 21.5 12.5 9l4 6.2 2.7-3.7 7.8 10Z" fill="#e6edf6" />
      <path d="M8 25.5q8-3.4 16 0" fill="none" stroke="var(--accent)" stroke-width="2.4" stroke-linecap="round" />
    </svg>
  );
}

/** Sum of one kind's exposed counts over every lake (from `downstream.exposed_counts`). */
function exposed(lakes: Lake[], kind: ImpactKind) {
  let path = 0;
  let risk = 0;
  for (const l of lakes) {
    const c = l.downstream?.exposed_counts?.[kind];
    if (c) {
      path += c.in_flood_path ?? 0;
      risk += c.at_risk ?? 0;
    }
  }
  return { path, risk };
}

function Kpi({ label, value, sub, title }: { label: string; value: ComponentChildren; sub?: ComponentChildren; title?: string }) {
  return (
    <div class="kpi" title={title}>
      <dt class="kpi-label">{label}</dt>
      <dd class="kpi-value">{value}</dd>
      {sub !== undefined && <dd class="kpi-sub">{sub}</dd>}
    </div>
  );
}

export function Header({ lakes, generatedAt }: { lakes: Lake[] | null; generatedAt: string | null }) {
  const ls = lakes ?? [];
  const ready = lakes !== null;
  const level = (lv: RiskLevel) => ls.filter((l) => l.risk?.level === lv).length;
  const dataUntil = ls.reduce<string | null>(
    (m, l) => (l.risk?.data_until && (!m || l.risk.data_until > m) ? l.risk.data_until : m),
    null,
  );
  const settle = exposed(ls, "settlement");
  const bridge = exposed(ls, "bridge");
  const hydro = exposed(ls, "hydro");
  const v = (n: number) => (ready ? num(n) : "—");

  return (
    <header class="app-header">
      <div class="brand">
        <Mark />
        <div class="brand-text">
          <h1>
            Aahat{" "}
            <span class="hi" lang="hi">
              आहट
            </span>
          </h1>
          <p class="brand-sub">Glacial lake watch · Himachal Pradesh</p>
        </div>
      </div>

      <dl class="kpis" aria-label="Summary across all watched lakes">
        <Kpi
          label="Very high / high risk"
          value={
            <>
              {v(level("very_high"))}
              <span class="kpi-slash"> / </span>
              {v(level("high"))}
            </>
          }
          sub={ready ? `of ${num(ls.length)} lakes watched` : undefined}
        />
        <Kpi
          label="Settlements in flood path"
          value={v(settle.path)}
          sub={ready ? `${num(settle.risk)} more at risk` : undefined}
        />
        <Kpi
          label="Bridges · hydro in flood path"
          value={
            <>
              {v(bridge.path)}
              <span class="kpi-slash"> · </span>
              {v(hydro.path)}
            </>
          }
          sub={ready ? `${num(bridge.risk)} · ${num(hydro.risk)} more at risk` : undefined}
        />
        <Kpi
          label="Latest satellite data"
          value={dataUntil ? dateOnly(dataUntil) : "—"}
          title={generatedAt ? `Index generated ${dateTime(generatedAt)}` : undefined}
        />
      </dl>

      <div class="header-meta">
        <span class="role-tag">DDMA / SDMA</span>
        <AuthStatus />
      </div>
    </header>
  );
}

export function Footer({ apiBase }: { apiBase: string }) {
  return (
    <footer class="app-footer">
      <span class="disclaimer">Screening estimates from satellite data; not a substitute for field assessment.</span>
      <span class="spacer" />
      <span class="muted">Sources</span>
      <a href="https://registry.opendata.aws/sentinel-2-l2a-cogs/" target="_blank" rel="noopener noreferrer">
        Sentinel-2
      </a>
      <a href="https://dataspace.copernicus.eu/explore-data/data-collections/copernicus-contributing-missions/collections-description/COP-DEM" target="_blank" rel="noopener noreferrer">
        Copernicus DEM GLO-30
      </a>
      <a href="https://www.glims.org/rgi_user_guide/welcome.html" target="_blank" rel="noopener noreferrer">
        RGI 7.0
      </a>
      <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener noreferrer">
        OpenStreetMap
      </a>
      <a href="https://www.nrsc.gov.in/" target="_blank" rel="noopener noreferrer">
        NRSC
      </a>
      <span class="muted api-base">API {apiBase}</span>
    </footer>
  );
}
