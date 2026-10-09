"""An explainable, hindsight-free GLOF hazard score for a monitored lake.

Every factor is a raw, physically meaningful number (with units), turned into a 0-1 score by a
stated linear rule, then weighted. The UI shows all of it: value, rule, score, weight,
contribution, source. Nothing is learned or hidden.

Replay: the score "as of" season Y uses only seasons <= Y (lake areas and outlines measured from
imagery acquired up to Y's last scene) plus the DEM, acquired 2011-2015, i.e. before any season
scored. A replay can therefore show how the score would have looked at the time.

This is a screening score for prioritising attention, not a probability of failure.
"""

from __future__ import annotations

import json
import math
from dataclasses import asdict, dataclass, field
from pathlib import Path

import numpy as np

from .laketerrain import LakeTerrain, TerrainParams, lake_terrain, outlet_geojson

HUGGEL_2002 = "https://doi.org/10.1139/t01-099"
MIN_GROWTH_YEARS = 3


def huggel_volume_m3(area_m2: float) -> float:
    """Huggel et al. (2002): V = 0.104 A^1.42 (A in m2, V in m3)."""
    return 0.104 * area_m2**1.42


def huggel_peak_discharge_m3s(volume_m3: float) -> float:
    """Huggel et al. (2002), moraine-dam breach: Qmax = 0.00077 V^1.017 (m3/s)."""
    return 0.00077 * volume_m3**1.017


def theil_sen(x: np.ndarray, y: np.ndarray) -> float:
    """Median of pairwise slopes: robust to one bad year."""
    slopes = [(y[j] - y[i]) / (x[j] - x[i]) for i in range(len(x)) for j in range(i + 1, len(x)) if x[j] != x[i]]
    return float(np.median(slopes))


@dataclass(frozen=True)
class Rule:
    """value -> score in [0, 1], linear between `lo` (score 0) and `hi` (score 1), optionally on log10."""

    lo: float
    hi: float
    log10: bool = False

    def score(self, value: float | None) -> float:
        if value is None or not math.isfinite(value):
            return 0.0
        v = math.log10(max(value, 1e-12)) if self.log10 else value
        return float(min(1.0, max(0.0, (v - self.lo) / (self.hi - self.lo))))

    def describe(self, unit: str) -> str:
        if self.log10:
            return f"0 at {10**self.lo:,.0f} {unit} rising (log scale) to 1 at {10**self.hi:,.0f} {unit}"
        return f"0 at {self.lo:g} {unit} rising linearly to 1 at {self.hi:g} {unit}"


@dataclass(frozen=True)
class FactorSpec:
    key: str
    label: str
    label_hi: str
    unit: str
    rule: Rule
    weight: float
    why: str
    source: str


FACTORS: tuple[FactorSpec, ...] = (
    FactorSpec(
        "growth",
        "Lake growth rate",
        "झील बढ़ने की दर",
        "%/yr",
        Rule(0.0, 3.0),
        0.25,
        "A growing lake stores more water and often means a retreating glacier front.",
        "Sentinel-2 yearly areas (this pipeline), Theil-Sen trend over full-coverage seasons",
    ),
    FactorSpec(
        "volume",
        "Water volume",
        "पानी की मात्रा",
        "m³",
        Rule(5.0, 8.0, log10=True),
        0.25,
        "More water means a bigger, longer-reaching flood.",
        f"Huggel et al. 2002, V = 0.104·A^1.42 ({HUGGEL_2002})",
    ),
    FactorSpec(
        "outlet_slope",
        "Steepness below the outlet",
        "निकास के नीचे ढलान",
        "°",
        Rule(0.0, 10.0),
        0.25,
        "A steep drop below the dam lets a breach cut down fast.",
        "Copernicus GLO-30 DEM, mean gradient over the first 1 km below the spill point",
    ),
    FactorSpec(
        "steep_slopes",
        "Steep slopes above the lake",
        "झील के ऊपर खड़ी ढलानें",
        "% of land",
        Rule(0.0, 50.0),
        0.25,
        "Ice or rock falling into the lake can send a wave over the dam.",
        "Copernicus GLO-30 DEM, share of land above lake level within 1 km that is >= 30° steep",
    ),
)

LEVELS = ((75, "very_high"), (50, "high"), (25, "moderate"), (0, "low"))


@dataclass
class Factor:
    key: str
    label: str
    label_hi: str
    value: float | None
    unit: str
    score: float
    weight: float
    contribution: float  # points out of 100
    rule: str
    why: str
    source: str
    note: str = ""


@dataclass
class RiskRecord:
    lake_id: str
    as_of_season: int
    data_until: str | None  # last satellite scene used
    score: float
    level: str
    factors: list[Factor]
    area_m2: float
    area_year: int
    volume_m3: float
    peak_discharge_m3s: float
    terrain: dict = field(default_factory=dict)


def level_for(score: float) -> str:
    return next(name for floor, name in LEVELS if score >= floor)


def growth_as_of(years: list[dict], season: int) -> tuple[float | None, str]:
    """Relative growth (%/yr) from full-coverage seasons up to `season`, with a note."""
    ok = [r for r in years if r["year"] <= season and r["status"] == "ok" and r["area_m2"]]
    if len(ok) < MIN_GROWTH_YEARS:
        return None, f"needs {MIN_GROWTH_YEARS} full-coverage seasons, have {len(ok)}"
    x = np.array([r["year"] for r in ok], float)
    y = np.array([r["area_m2"] for r in ok], float)
    slope = theil_sen(x, y)
    pct = 100 * slope / float(np.median(y))
    return pct, f"{len(ok)} seasons {int(x.min())}-{int(x.max())}, {slope:+,.0f} m²/yr"


def score_as_of(lake_id: str, years: list[dict], terrain_for, season: int) -> RiskRecord | None:
    """The score as it could have been computed right after `season`'s last scene."""
    past = [r for r in years if r["year"] <= season and r["area_m2"]]
    if not past:
        return None
    full = [r for r in past if r["status"] == "ok"]
    current = (full or past)[-1]  # latest full-coverage outline; a partial one only if nothing better
    terrain: LakeTerrain = terrain_for(current["year"])
    volume = huggel_volume_m3(current["area_m2"])
    growth, growth_note = growth_as_of(years, season)
    values = {
        "growth": (growth, growth_note),
        "volume": (volume, f"from {current['area_m2'] / 1e6:.3f} km² in {current['year']}"),
        "outlet_slope": (
            terrain.outlet_slope_deg,
            f"{terrain.outlet_drop_m:.0f} m drop over {terrain.outlet_run_m:.0f} m"
            if terrain.surface_outlet
            else "no surface outflow found below the spill point",
        ),
        "steep_slopes": (100 * terrain.steep_share, f"{terrain.steep_area_km2:.2f} km² of >= 30° slopes"),
    }
    factors = []
    for spec in FACTORS:
        value, note = values[spec.key]
        s = spec.rule.score(value)
        factors.append(
            Factor(
                spec.key,
                spec.label,
                spec.label_hi,
                None if value is None else round(value, 3 if spec.key == "growth" else 1),
                spec.unit,
                round(s, 3),
                spec.weight,
                round(100 * s * spec.weight, 1),
                spec.rule.describe(spec.unit),
                spec.why,
                spec.source,
                note,
            )
        )
    total = round(sum(f.contribution for f in factors), 1)
    data_until = max((r["last_day"] for r in past if r.get("last_day")), default=None)
    t = terrain.to_dict()
    return RiskRecord(
        lake_id,
        season,
        data_until,
        total,
        level_for(total),
        factors,
        current["area_m2"],
        current["year"],
        round(volume),
        round(huggel_peak_discharge_m3s(volume)),
        {k: v for k, v in t.items() if k != "outlet_path"},
    )


def run_risk(lake_dir: Path, tp: TerrainParams = TerrainParams()) -> dict:
    """Replay the score for every season with data, write risk.json and outlet.geojson."""
    series = json.loads((lake_dir / "series.json").read_text())
    years = series["years"]
    outlines = {
        r["year"]: json.loads((lake_dir / f"{r['year']}.geojson").read_text())
        for r in years
        if (lake_dir / f"{r['year']}.geojson").exists()
    }
    cache: dict[int, LakeTerrain] = {}

    def terrain_for(year: int) -> LakeTerrain:
        if year not in cache:
            cache[year] = lake_terrain(outlines[year]["geometry"], tp)
        return cache[year]

    usable = [r for r in years if r["year"] in outlines]
    replay = [
        rec
        for y in sorted(r["year"] for r in years)
        if (rec := score_as_of(series["lake"]["id"], usable, terrain_for, y))
    ]
    latest = replay[-1] if replay else None
    out = {
        "lake_id": series["lake"]["id"],
        "method": {
            "factors": [asdict(f) | {"rule": f.rule.describe(f.unit)} for f in FACTORS],
            "levels": {name: floor for floor, name in LEVELS},
            "hindsight": "Each replay entry uses only seasons up to as_of_season and the 2011-2015 DEM.",
            "disclaimer": "Screening score for prioritising attention, not a probability of failure.",
        },
        "latest": asdict(latest) if latest else None,
        "replay": [asdict(r) for r in replay],
    }
    (lake_dir / "risk.json").write_text(json.dumps(out, indent=1, ensure_ascii=False))
    if latest:
        (lake_dir / "outlet.geojson").write_text(json.dumps(outlet_geojson(terrain_for(latest.area_year))))
    return out
