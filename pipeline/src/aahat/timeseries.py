"""Yearly post-monsoon lake area for a catalogued lake."""

from __future__ import annotations

import json
import logging
from dataclasses import asdict, dataclass
from datetime import UTC, date, datetime
from pathlib import Path

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
    status: str  # ok | partial | not_found | no_data
    area_m2: float | None
    uncertainty_m2: float | None
    perimeter_m: float | None
    coverage: float | None
    scenes_found: int
    scenes_clear: int
    first_day: str | None
    last_day: str | None
    params: dict


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
    comp = composite(found, grid, terrain, p, max_scenes=max_scenes)
    used = comp.inputs
    if not used:
        return LakeYear(lake.id, year, "no_data", None, None, None, None, len(found), 0, None, None, asdict(p)), None, None
    seed = point_in_crs(lake.lon, lake.lat, grid.crs)
    ext = extract_lake(comp, seed, p)
    days = (used[0].day.isoformat(), used[-1].day.isoformat())
    if ext is None:
        return LakeYear(lake.id, year, "not_found", None, None, None, None, len(found), len(used), *days, asdict(p)), comp, None
    status = "ok" if ext.coverage >= 0.9 else "partial"
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
    """Collect every lake's series.json into lakes/index.json, the file the API and maps read."""
    lakes = []
    for path in sorted((out_dir / "lakes").glob("*/series.json")):
        s = json.loads(path.read_text())
        years = [
            {k: r[k] for k in ("year", "status", "area_m2", "uncertainty_m2", "coverage", "scenes_clear")}
            for r in s["years"]
        ]
        measured = [r for r in years if r["area_m2"] is not None]
        lakes.append(
            s["lake"]
            | {
                "years": years,
                "first": measured[0] if measured else None,
                "latest": measured[-1] if measured else None,
            }
        )
    index = {"generated_at": datetime.now(UTC).isoformat(timespec="seconds"), "lakes": lakes}
    (out_dir / "lakes" / "index.json").write_text(json.dumps(index, indent=1, ensure_ascii=False))
    return index
