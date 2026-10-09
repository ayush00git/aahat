import { dateOnly, dateTime } from "../format";
import { AuthStatus } from "./SignIn";

export function Header({ dataUntil, generatedAt }: { dataUntil: string | null; generatedAt: string | null }) {
  return (
    <header class="app-header">
      <div class="brand">
        <span class="brand-mark" aria-hidden="true">
          <svg viewBox="0 0 32 32" width="22" height="22">
            <path d="M3 24 L12 9 L16 16 L19.5 11 L29 24 Z" fill="currentColor" opacity="0.9" />
            <ellipse cx="16" cy="25" rx="7.5" ry="2.4" fill="#4ea1ff" />
          </svg>
        </span>
        <h1>
          Aahat <span class="hi" lang="hi">आहट</span>
          <span class="brand-sub">— Glacial lake watch, Himachal Pradesh</span>
        </h1>
      </div>
      <div class="header-meta">
        <span class="role-tag">DDMA / SDMA</span>
        <AuthStatus />
        {dataUntil && (
          <span title={generatedAt ? `Index generated ${dateTime(generatedAt)}` : undefined}>
            Satellite data until <b>{dateOnly(dataUntil)}</b>
          </span>
        )}
        <span class="disclaimer">Screening estimates from satellite data; not a substitute for field assessment.</span>
      </div>
    </header>
  );
}

export function Footer({ apiBase }: { apiBase: string }) {
  return (
    <footer class="app-footer">
      <span class="k">Sources</span>
      <a href="https://registry.opendata.aws/sentinel-2-l2a-cogs/" target="_blank" rel="noopener noreferrer">
        Sentinel-2 (Copernicus, via AWS Open Data)
      </a>
      <span aria-hidden="true">·</span>
      <a href="https://dataspace.copernicus.eu/explore-data/data-collections/copernicus-contributing-missions/collections-description/COP-DEM" target="_blank" rel="noopener noreferrer">
        Copernicus DEM GLO-30
      </a>
      <span aria-hidden="true">·</span>
      <a href="https://www.glims.org/rgi_user_guide/welcome.html" target="_blank" rel="noopener noreferrer">
        RGI 7.0
      </a>
      <span aria-hidden="true">·</span>
      <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener noreferrer">
        OpenStreetMap
      </a>
      <span aria-hidden="true">·</span>
      <a href="https://www.nrsc.gov.in/" target="_blank" rel="noopener noreferrer">
        NRSC GLOF reports (benchmarks)
      </a>
      <span class="spacer" />
      <span class="muted">API {apiBase}</span>
    </footer>
  );
}
