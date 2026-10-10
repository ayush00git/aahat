import { dateOnly, km2, num, YEAR_STATUS_LABEL } from "../format";
import type { Lake, RiskFile } from "../types";
import { GrowthChart } from "./charts";
import { MoreToggle, Section } from "./ui";
import { useState } from "preact/hooks";

export function GrowthTab({ lake, risk }: { lake: Lake; risk: RiskFile | null }) {
  const years = [...lake.years].sort((a, b) => a.year - b.year);
  const partial = years.filter((y) => y.status === "partial").map((y) => y.year);
  const missing = years.filter((y) => y.status === "no_data" || y.status === "not_found").map((y) => y.year);
  const growth = risk?.latest.factors.find((f) => f.key === "growth");
  const dataUntil = risk?.latest.data_until ?? lake.risk?.data_until;
  const [more, setMore] = useState(false);

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
        {growth &&
          (growth.value !== null ? (
            <p class="lead">
              Trend <b>{num(growth.value, 2)} {growth.unit}</b>
            </p>
          ) : (
            <p class="muted">Trend not measured{growth.note ? `: ${growth.note}` : ""}.</p>
          ))}
        {partial.length > 0 && <p class="note">Partial coverage: {partial.join(", ")} (not used for the trend).</p>}
        <details class="more">
          <summary>How to read the chart</summary>
          <p>
            Whiskers: ± measurement uncertainty. Filled points: full-coverage seasons, joined by the line. Hollow points:
            partial coverage (cloud or snow).{missing.length > 0 && " Crosses: no usable outline."}
          </p>
          {growth?.note && <p>{growth.note}</p>}
        </details>
      </Section>

      <Section title="Yearly measurements" aside={<MoreToggle on={more} onToggle={() => setMore(!more)} />}>
        <table class="table">
          <thead>
            <tr>
              <th>Year</th>
              <th class="num">Area</th>
              {more && <th class="num">±</th>}
              {more && <th class="num">Coverage</th>}
              {more && <th class="num">Clear scenes</th>}
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {[...years].reverse().map((y) => (
              <tr key={y.year}>
                <td>{y.year}</td>
                <td class="num">{km2(y.area_m2)}</td>
                {more && <td class="num">{km2(y.uncertainty_m2)}</td>}
                {more && <td class="num">{y.coverage !== null ? `${num(y.coverage * 100, 0)}%` : "—"}</td>}
                {more && <td class="num">{num(y.scenes_clear)}</td>}
                <td class={y.status !== "ok" ? "muted" : ""}>{YEAR_STATUS_LABEL[y.status] ?? y.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Section>
    </div>
  );
}
