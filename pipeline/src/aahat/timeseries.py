"""Yearly post-monsoon lake area for a catalogued lake."""

from __future__ import annotations

import json
import logging
from dataclasses import asdict, dataclass, field
from datetime import UTC, date, datetime
from pathlib import Path

import numpy as np
from shapely.geometry import mapping

from .dem import Terrain
from .geo import grid_around, point_in_crs, to_wgs84
from .lakes import Lake
from .stac import find_scenes
from .water import Composite, LakeExtent, WaterParams, composite, extract_lake

log = logging.getLogger(__name__)

# Post-monsoon window: lakes near their yearly maximum, before freeze-up and winter snow.
SEASON = ((8, 15), (10, 31))
FIRST_YEAR = 2017  # first year with L2A coverage over the Himalaya on Earth Search


def season(year: int) -> tuple[date, date]:
    (m0, d0), (m1, d1) = SEASON
    return date(year, m0, d0), date(year, m1, d1)


@dataclass
class LakeYear:
    lake_id: str
    year: int
    status: str  # ok | partial (lake not fully seen, or < min_obs clear scenes) | not_found | no_data
    area_m2: float | None
    uncertainty_m2: float | None
    perimeter_m: float | None
    coverage: float | None
    scenes_found: int
    scenes_clear: int
    first_day: str | None
    last_day: str | None
    params: dict
    # Evidence: the Sentinel-2 items the outline was mapped from, and their dates. Empty for years
    # computed before these were recorded (old series.json files still load).
    scene_ids: list[str] = field(default_factory=list)
    scene_days: list[str] = field(default_factory=list)


def lake_focus(grid, seed_xy: tuple[float, float], radius_m: float) -> np.ndarray:
    """Disk around the lake's seed point: scene selection cares about clouds here, not on the hills."""
    r0, c0 = grid.xy_to_rowcol(*seed_xy)
    rad = max(radius_m, 500.0) / grid.res
    rr, cc = np.ogrid[: grid.height, : grid.width]
    return (rr - r0) ** 2 + (cc - c0) ** 2 <= rad**2


def lake_year(
    lake: Lake, year: int, terrain: Terrain, p: WaterParams = WaterParams(), max_scenes: int = 12
) -> tuple[LakeYear, Composite | None, LakeExtent | None]:
    grid = terrain.grid
    aoi = to_wgs84(grid.polygon(), grid.crs)
    start, end = season(year)
    found = find_scenes(aoi, start, min(end, datetime.now(UTC).date()))
    log.info("%s %d: %d candidate scenes", lake.id, year, len(found))
    if not found:
        return LakeYear(lake.id, year, "no_data", None, None, None, None, 0, 0, None, None, asdict(p)), None, None
    seed = point_in_crs(lake.lon, lake.lat, grid.crs)
    comp = composite(
        found, grid, terrain, p, max_scenes=max_scenes, focus=lake_focus(grid, seed, lake.aoi_radius_m / 2)
    )
    used = comp.inputs
    if not used:
        return (
            LakeYear(lake.id, year, "no_data", None, None, None, None, len(found), 0, None, None, asdict(p)),
            None,
            None,
        )
    seed = point_in_crs(lake.lon, lake.lat, grid.crs)
    ext = extract_lake(comp, seed, p)
    days = (used[0].day.isoformat(), used[-1].day.isoformat())
    evidence = {"scene_ids": [s.item_id for s in used], "scene_days": [s.day.isoformat() for s in used]}
    if ext is None:
        return (
            LakeYear(
                lake.id, year, "not_found", None, None, None, None, len(found), len(used), *days, asdict(p), **evidence
            ),
            comp,
            None,
        )
    # partial: the lake was not fully observed, or the season had fewer clear scenes than p.min_obs
    status = "ok" if ext.coverage >= 0.9 and len(used) >= p.min_obs else "partial"
    rec = LakeYear(
        lake.id,
        year,
        status,
        round(ext.area_m2, 1),
        round(ext.uncertainty_m2, 1),
        round(ext.perimeter_m, 1),
        round(ext.coverage, 3),
        len(found),
        len(used),
        *days,
        asdict(p),
        **evidence,
    )
    return rec, comp, ext


def run_lake(
    lake: Lake,
    years: list[int],
    out_dir: Path,
    p: WaterParams = WaterParams(),
    max_scenes: int = 12,
    quicklook: bool = False,
) -> list[LakeYear]:
    grid = grid_around(lake.lon, lake.lat, lake.aoi_radius_m)
    terrain = Terrain(grid)
    lake_dir = out_dir / "lakes" / lake.id
    lake_dir.mkdir(parents=True, exist_ok=True)
    grid_meta = {"crs": grid.crs.to_string(), "bounds": list(grid.bounds), "res": grid.res}
    series_path = lake_dir / "series.json"
    # Merge into earlier runs on the same grid, so a refresh only recomputes the years asked for.
    by_year: dict[int, dict] = {}
    if series_path.exists():
        old = json.loads(series_path.read_text())
        if old.get("grid") == grid_meta:
            by_year = {r["year"]: r for r in old["years"]}
    records: list[LakeYear] = []
    for year in years:
        rec, comp, ext = lake_year(lake, year, terrain, p, max_scenes)
        records.append(rec)
        by_year[year] = asdict(rec)
        series = {"lake": asdict(lake), "grid": grid_meta, "years": [by_year[y] for y in sorted(by_year)]}
        series_path.write_text(json.dumps(series, indent=1))
        log.info("%s %d: %s area=%s m2 +/- %s", lake.id, year, rec.status, rec.area_m2, rec.uncertainty_m2)
        if ext is not None:
            feature = {
                "type": "Feature",
                "geometry": mapping(to_wgs84(ext.polygon.simplify(grid.res / 2), grid.crs)),
                "properties": asdict(rec) | {"name": lake.name},
            }
            (lake_dir / f"{year}.geojson").write_text(json.dumps(feature))
        if quicklook and comp is not None:
            from .quicklook import save_quicklook

            save_quicklook(comp, ext, lake_dir / f"{year}.png", title=f"{lake.name} {year}")
    return records


def build_index(out_dir: Path) -> dict:
    """Collect every lake's series.json into lakes/index.json, the file the API and maps read,
    and each lake's yearly outlines into one <lake>/outlines.geojson for the growth time-lapse."""
    lakes = []
    for path in sorted((out_dir / "lakes").glob("*/series.json")):
        s = json.loads(path.read_text())
        outlines = [
            json.loads((path.parent / f"{r['year']}.geojson").read_text())
            for r in s["years"]
            if (path.parent / f"{r['year']}.geojson").exists()
        ]
        fc = {"type": "FeatureCollection", "features": outlines}
        (path.parent / "outlines.geojson").write_text(json.dumps(fc, ensure_ascii=False))
        years = [{k: r.get(k) for k in YEAR_KEYS} | _evidence(r) for r in s["years"]]
        # first/latest repeat a year row: leave the scene lists out of them to keep the index small
        measured = [{k: r[k] for k in YEAR_KEYS} for r in years if r["area_m2"] is not None]
        downstream = _downstream_summary(path.parent)
        risk_path = path.parent / "risk.json"
        latest_risk = json.loads(risk_path.read_text()).get("latest") if risk_path.exists() else None
        risk = (
            {
                k: latest_risk[k]
                for k in ("as_of_season", "data_until", "score", "level", "volume_m3", "peak_discharge_m3s")
            }
            # which relation peak_discharge_m3s is, and both scenario peaks (absent from older risk.json files)
            | {
                k: latest_risk.get(k)
                for k in ("peak_discharge_relation", "peak_expected_m3s", "peak_severe_m3s", "peak_expected_relation")
            }
            if latest_risk
            else None
        )
        lakes.append(
            s["lake"]
            | {
                "years": years,
                "first": measured[0] if measured else None,
                "latest": measured[-1] if measured else None,
                "risk": risk,
                "downstream": downstream,
                "drain": _drain_summary(path.parent),
            }
        )
    index = {"generated_at": datetime.now(UTC).isoformat(timespec="seconds"), "lakes": lakes}
    (out_dir / "lakes" / "index.json").write_text(json.dumps(index, indent=1, ensure_ascii=False))
    return index


YEAR_KEYS = ("year", "status", "area_m2", "uncertainty_m2", "coverage", "scenes_clear")


def _evidence(rec: dict) -> dict:
    """Which scenes a year's outline came from; nothing for years computed before that was recorded."""
    return {k: rec[k] for k in ("scene_ids", "scene_days") if rec.get(k)}


def _drain_summary(lake_dir: Path) -> dict | None:
    """The latest sudden-drain check (see drain.py), if one was run."""
    path = lake_dir / "drain.json"
    if not path.exists():
        return None
    d = json.loads(path.read_text())
    return {
        "as_of": d["as_of"],
        "drained": d["drained"],
        "drop_fraction": d["drop_fraction"],
        "status": d["status"],
        "latest_day": (d.get("latest") or {}).get("day"),
    }


def _downstream_summary(lake_dir: Path) -> dict | None:
    """Headline downstream numbers for the index: what is hit first and how many places are exposed."""
    d_path, i_path = lake_dir / "downstream.json", lake_dir / "impacts.json"
    if not (d_path.exists() and i_path.exists()):
        return None
    d = json.loads(d_path.read_text())
    rows = json.loads(i_path.read_text())
    exposed = [r for r in rows if r["status"] != "outside"]
    counts: dict[str, dict[str, int]] = {}
    for r in exposed:
        counts.setdefault(r["kind"], {"in_flood_path": 0, "at_risk": 0})[r["status"]] += 1
    first_settlement = next((r for r in exposed if r["kind"] == "settlement"), None)
    scenarios = d["discharge"]["scenarios"]
    peaks = {k: v["peak_m3s"] for k, v in scenarios.items()}
    return {
        "path_km": d["path_km"],
        # breach peaks at the lake, one per scenario; the relation each comes from is in peak_relations
        "peak_expected_m3s": peaks.get("expected"),
        "peak_severe_m3s": peaks.get("severe"),
        "peak_relations": {k: v.get("relation") for k, v in scenarios.items()},
        # True when both scenarios are the same flood: for small lakes Evans (expected) exceeds Huggel,
        # and severe is the larger of the two. UIs should then show one scenario, not two equal ones.
        "scenarios_identical": d["discharge"].get("scenarios_identical", peaks.get("expected") == peaks.get("severe")),
        "peak_m3s": peaks,  # deprecated alias of the two keys above, kept for one release
        "first_exposed": exposed[0] | {"scenarios": None} if exposed else None,
        "first_settlement": first_settlement | {"scenarios": None} if first_settlement else None,
        "exposed_counts": counts,
        "exposure_gaps_km": d.get("exposure_gaps_km", []),
    }
