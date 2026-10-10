"""An explainable, hindsight-free GLOF hazard score for a monitored lake.

Score = 100 x size x likelihood.
- size: how much water could come out (volume from area, Huggel et al. 2002), 0-1.
- likelihood: equal-weight mean (as in Allen et al. 2019) of what makes a release more likely: the
  lake growing, a steep drop below its outlet, slopes that can drop ice or rock into it, and glacier
  ice at its shore, 0-1. Thresholds follow published criteria, cited per factor, including India's
  CWC 2024 risk-indexing criteria.
Multiplying keeps a small pond in steep terrain from outranking a large lake, and a large lake
with nothing pushing it from being ranked first by size alone.

Every factor is a raw, physically meaningful number (with units) turned into 0-1 by a stated
linear rule. The UI shows all of it: value, rule, score, weight, contribution, source.

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
from shapely.geometry import shape

from .glaciers import RGI_DOI, GlacierProximity, glaciers_geojson, nearest_glacier
from .laketerrain import LakeTerrain, TerrainParams, lake_terrain, outlet_geojson
from .peak import peak_discharge

HUGGEL_2002 = "https://doi.org/10.1139/t01-099"
KOUGKOULOS_2018 = "https://doi.org/10.1016/j.scitotenv.2017.10.083"
CWC_2024 = "https://cwc.gov.in/sites/default/files/final-report-risk-index-criteria1-final-book.pdf"
FUJITA_2013 = "https://doi.org/10.5194/nhess-13-1827-2013"
ALLEN_2016 = "https://doi.org/10.1007/s11069-016-2511-x"
RINZIN_2021 = "https://doi.org/10.3389/feart.2021.775195"
MIN_GROWTH_YEARS = 3


def huggel_volume_m3(area_m2: float) -> float:
    """Huggel et al. (2002): V = 0.104 A^1.42 (A in m2, V in m3)."""
    return 0.104 * area_m2**1.42


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
    group: str  # "size" or "likelihood"
    label: str
    label_hi: str
    unit: str
    rule: Rule
    weight: float  # within its group; each group's weights sum to 1
    why: str
    source: str


FACTORS: tuple[FactorSpec, ...] = (
    FactorSpec(
        "volume",
        "size",
        "Water volume",
        "पानी की मात्रा",
        "m³",
        Rule(5.0, 8.0, log10=True),
        1.0,
        "More water means a bigger, longer-reaching flood. On this scale 1 and 10 million m³ (the low/medium "
        "and medium/high class limits of Kougkoulos et al. 2018) sit at 0.33 and 0.67.",
        f"Volume from area by Huggel et al. 2002, V = 0.104·A^1.42 ({HUGGEL_2002}); classes: {KOUGKOULOS_2018}",
    ),
    FactorSpec(
        "growth",
        "likelihood",
        "Lake growth",
        "झील का बढ़ना",
        "% per 5 yr",
        Rule(0.0, 25.0),
        0.25,
        "A growing lake stores more water and usually means its glacier is retreating. CWC's top class is "
        "over 25% area growth in 5 years.",
        "Sentinel-2 yearly areas (this pipeline), Theil-Sen trend over full-coverage seasons, counted only when "
        f"the change exceeds twice the measurement uncertainty; scale from CWC 2024 ({CWC_2024})",
    ),
    FactorSpec(
        "outlet_slope",
        "likelihood",
        "Steepness below the outlet",
        "निकास के नीचे ढलान",
        "°",
        Rule(0.0, 10.0),
        0.25,
        "A steep drop right below the dam lets a breach cut down fast. Fujita et al. 2013 treat lakes with "
        "no 'steep lakefront' (over 10°) as unlikely to fail.",
        f"Copernicus GLO-30 DEM: mean gradient over the first 1 km below the spill point; 10° from {FUJITA_2013}",
    ),
    FactorSpec(
        "avalanche",
        "likelihood",
        "Ice/rock fall source area",
        "बर्फ़/चट्टान गिरने का क्षेत्र",
        "km²",
        Rule(0.0, 0.5),
        0.25,
        "Ice or rock falling into the lake can send a wave over the dam: the commonest GLOF trigger.",
        "Copernicus GLO-30 DEM: slopes over 30° in the lake's catchment whose line to the lake is steeper "
        f"than 14° (Allen et al. 2016, Himachal Pradesh, {ALLEN_2016}); 0.5 km² is the high class of "
        f"Rinzin et al. 2021 ({RINZIN_2021})",
    ),
    FactorSpec(
        "glacier",
        "likelihood",
        "Distance to glacier ice",
        "ग्लेशियर से दूरी",
        "m",
        Rule(500.0, 0.0),
        0.25,
        "A lake touching its glacier gets calving ice and ice avalanches. Rinzin et al. 2021 rate contact as "
        "high and within 500 m as medium; CWC's top class is a snout within 0.5 km.",
        "Randolph Glacier Inventory 7.0 outlines (c. 2000, so contact may be overstated where the glacier has "
        f"since retreated; {RGI_DOI}); thresholds from {RINZIN_2021} and {CWC_2024}",
    ),
)

LEVELS = ((60, "very_high"), (40, "high"), (20, "moderate"), (0, "low"))


@dataclass
class Factor:
    key: str
    group: str
    label: str
    label_hi: str
    value: float | None
    unit: str
    score: float  # 0-1 from the rule
    weight: float  # within its group
    rule: str
    why: str
    source: str
    note: str = ""


@dataclass
class RiskRecord:
    lake_id: str
    as_of_season: int
    data_until: str | None  # last satellite scene used
    score: float  # 100 x size x likelihood
    level: str
    size: float
    likelihood: float
    factors: list[Factor]
    area_m2: float
    area_year: int
    volume_m3: float
    # Breach peak of the SEVERE scenario of peak.py, the same number downstream.json routes: the larger
    # of Huggel 2002 and Evans 1986. `peak_discharge_relation` says which one it is for this volume.
    # (Until 2026-10 this was always Huggel 2002, which for lakes under ~1.26 million m3 is lower than
    # the expected scenario's Evans peak, so it disagreed with the downstream summary.)
    peak_discharge_m3s: float
    terrain: dict = field(default_factory=dict)
    peak_discharge_relation: str = ""
    peak_expected_m3s: float | None = None  # Evans 1986
    peak_severe_m3s: float | None = None  # = peak_discharge_m3s
    peak_expected_relation: str = ""


def level_for(score: float) -> str:
    return next(name for floor, name in LEVELS if score >= floor)


@dataclass
class Growth:
    pct_per_yr: float | None
    significant: bool
    note: str


def growth_as_of(years: list[dict], season: int) -> Growth:
    """Relative growth (%/yr) from full-coverage seasons up to `season`.

    It counts only if the fitted change over the period exceeds twice the typical per-season area
    uncertainty; otherwise a stable lake's year-to-year noise would read as growth.
    """
    ok = [r for r in years if r["year"] <= season and r["status"] == "ok" and r["area_m2"]]
    if len(ok) < MIN_GROWTH_YEARS:
        return Growth(None, False, f"needs {MIN_GROWTH_YEARS} full-coverage seasons, have {len(ok)}")
    x = np.array([r["year"] for r in ok], float)
    y = np.array([r["area_m2"] for r in ok], float)
    slope = theil_sen(x, y)
    pct = 100 * slope / float(np.median(y))
    change = slope * (x.max() - x.min())
    noise = 2 * float(np.median([r.get("uncertainty_m2") or 0 for r in ok]))
    significant = abs(change) > noise
    note = f"{len(ok)} seasons {int(x.min())}-{int(x.max())}: {slope:+,.0f} m²/yr, {change / 1e6:+.3f} km² in total"
    if not significant:
        note += f", within measurement uncertainty (±{noise / 1e6:.3f} km²), so not counted"
    return Growth(pct, significant, note)


def score_as_of(lake_id: str, years: list[dict], terrain_for, season: int, glacier_for=None) -> RiskRecord | None:
    """The score as it could have been computed right after `season`'s last scene."""
    past = [r for r in years if r["year"] <= season and r["area_m2"]]
    if not past:
        return None
    full = [r for r in past if r["status"] == "ok"]
    current = (full or past)[-1]  # latest full-coverage outline; a partial one only if nothing better
    terrain: LakeTerrain = terrain_for(current["year"])
    volume = huggel_volume_m3(current["area_m2"])
    growth = growth_as_of(years, season)
    no_glacier = GlacierProximity(None, None, None, None)
    glacier: GlacierProximity = glacier_for(current["year"]) if glacier_for else no_glacier
    gd = glacier.distance_m
    values = {
        "volume": (volume, f"from {current['area_m2'] / 1e6:.3f} km² measured in {current['year']}", True),
        "growth": (None if growth.pct_per_yr is None else 5 * growth.pct_per_yr, growth.note, growth.significant),
        "outlet_slope": (
            terrain.outlet_slope_deg,
            f"{terrain.outlet_drop_m:.0f} m drop over {terrain.outlet_run_m:.0f} m"
            if terrain.surface_outlet
            else "no surface outflow found below the spill point",
            True,
        ),
        "avalanche": (terrain.avalanche_area_km2, "slopes > 30° that can reach the lake", True),
        "glacier": (
            gd,
            f"{glacier.rgi_id} ({glacier.glacier_area_km2} km², outline {glacier.outline_date})"
            if gd is not None
            else "no inventoried glacier within 3 km",
            gd is not None,
        ),
    }
    factors = []
    for spec in FACTORS:
        value, note, counts = values[spec.key]
        s = spec.rule.score(value) if counts else 0.0
        shown = None if value is None else round(value, 2 if spec.key in ("growth", "avalanche") else 1)
        factors.append(
            Factor(
                spec.key,
                spec.group,
                spec.label,
                spec.label_hi,
                shown,
                spec.unit,
                round(s, 3),
                round(spec.weight, 4),
                spec.rule.describe(spec.unit),
                spec.why,
                spec.source,
                note,
            )
        )
    size = sum(f.score * f.weight for f in factors if f.group == "size")
    likelihood = sum(f.score * f.weight for f in factors if f.group == "likelihood")
    total = round(100 * size * likelihood, 1)
    data_until = max((r["last_day"] for r in past if r.get("last_day")), default=None)
    t = terrain.to_dict()
    peaks = peak_discharge(round(volume))["scenarios"]  # the rounded volume is what downstream.json routes
    return RiskRecord(
        lake_id,
        season,
        data_until,
        total,
        level_for(total),
        round(size, 3),
        round(likelihood, 3),
        factors,
        current["area_m2"],
        current["year"],
        round(volume),
        peaks["severe"]["peak_m3s"],
        {k: v for k, v in t.items() if k != "outlet_path"},
        f"{peaks['severe']['relation_key']}: {peaks['severe']['relation']}",
        peaks["expected"]["peak_m3s"],
        peaks["severe"]["peak_m3s"],
        f"{peaks['expected']['relation_key']}: {peaks['expected']['relation']}",
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
    gcache: dict[int, GlacierProximity] = {}

    def terrain_for(year: int) -> LakeTerrain:
        if year not in cache:
            cache[year] = lake_terrain(outlines[year]["geometry"], tp)
        return cache[year]

    def glacier_for(year: int) -> GlacierProximity:
        if year not in gcache:
            gcache[year] = nearest_glacier(shape(outlines[year]["geometry"]))
        return gcache[year]

    usable = [r for r in years if r["year"] in outlines]
    replay = [
        rec
        for y in sorted(r["year"] for r in years)
        if (rec := score_as_of(series["lake"]["id"], usable, terrain_for, y, glacier_for))
    ]
    latest = replay[-1] if replay else None
    out = {
        "lake_id": series["lake"]["id"],
        "method": {
            "factors": [asdict(f) | {"rule": f.rule.describe(f.unit)} for f in FACTORS],
            "formula": "score = 100 x size x likelihood; size and likelihood are weighted means of their factors' 0-1 scores",
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
        latest_outline = shape(outlines[latest.area_year]["geometry"])
        (lake_dir / "glaciers.geojson").write_text(json.dumps(glaciers_geojson(latest_outline)))
    return out
