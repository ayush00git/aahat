import { dateOnly, km2, num, YEAR_STATUS_LABEL } from "../format";
import type { Lake, RiskFile } from "../types";
import { GrowthChart } from "./charts";
import { Section } from "./ui";

export function GrowthTab({ lake, risk }: { lake: Lake; risk: RiskFile | null }) {
  const years = [...lake.years].sort((a, b) => a.year - b.year);
  const partial = years.filter((y) => y.status === "partial").map((y) => y.year);
  const missing = years.filter((y) => y.status === "no_data" || y.status === "not_found").map((y) => y.year);
  const growth = risk?.latest.factors.find((f) => f.key === "growth");
  const dataUntil = risk?.latest.data_until ?? lake.risk?.data_until;

  return (
    <div class="tab-body">
      <dl class="kv kv-3">
        <div>
          <dt>First ({lake.first?.year ?? "—"})</dt>
          <dd>{km2(lake.first?.area_m2)}</dd>
        </div>
        <div>
          <dt>Latest ({lake.latest?.year ?? "—"})</dt>
          <dd>{km2(lake.latest?.area_m2)}</dd>
        </div>
        <div>
          <dt>Data until</dt>
          <dd>{dateOnly(dataUntil)}</dd>
        </div>
      </dl>

      <Section title="Lake area by year">
        <GrowthChart years={years} />
        <p class="caption">
          Whiskers: ± measurement uncertainty. Filled points: full-coverage seasons (joined by the line). Hollow points:
          partial coverage (cloud or snow), not used for the trend.
          {missing.length > 0 && " Crosses: no usable outline."}
        </p>
        {partial.length > 0 && (
          <p class="flag">
            Partial years: <b>{partial.join(", ")}</b>
          </p>
        )}
        {growth && (
          <p class="small">
            <span class="k">Trend</span> {num(growth.value, 2)} {growth.unit}
            {growth.note ? ` · ${growth.note}` : ""}
          </p>
        )}
      </Section>

      <Section title="Yearly measurements">
        <table class="table compact">
          <thead>
            <tr>
              <th>Year</th>
              <th class="num">Area</th>
              <th class="num">±</th>
              <th class="num">Coverage</th>
              <th class="num">Clear scenes</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {years.map((y) => (
              <tr key={y.year} class={y.status !== "ok" ? "row-flag" : ""}>
                <td>{y.year}</td>
                <td class="num">{km2(y.area_m2)}</td>
                <td class="num">{km2(y.uncertainty_m2)}</td>
                <td class="num">{y.coverage !== null ? `${num(y.coverage * 100, 0)}%` : "—"}</td>
                <td class="num">{num(y.scenes_clear)}</td>
                <td>{YEAR_STATUS_LABEL[y.status] ?? y.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p class="muted small">Areas from late-summer/autumn Sentinel-2 scenes (NDWI water mask), one outline per season.</p>
      </Section>
    </div>
  );
}
