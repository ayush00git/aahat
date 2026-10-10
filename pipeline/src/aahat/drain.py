"""Sudden drainage: a lake that partly empties between two satellite passes may have burst.

The season's composite outline (series.json) is the "before"; the "after" is water mapped on the
single most recent clear scene up to `as_of`. A lake counts as drained only when

- the latest scene clearly saw the lake: at least `min_coverage` of the SEASON outline's pixels were
  observed in that one scene. Cloud, cloud shadow and terrain shadow are unknown, never dry, so a
  cloud over the lake can not look like a drained lake;
- the outline is not under snow or ice (SCL class 11). A lake freezing over loses open water
  without losing any water, so a frozen lake is reported as `frozen_or_snow` and never flagged;
- the area dropped by more than `min_drop`, and by more than `sigmas` times the combined shoreline
  uncertainty of the two outlines.

Only scenes up to `as_of` are read, so replaying a past date shows what could have been seen then.
"""

from __future__ import annotations

import json
import logging
from dataclasses import asdict, dataclass
from datetime import UTC, date, datetime, timedelta
from pathlib import Path

import numpy as np
from rasterio import features
from shapely.geometry import shape

from .dem import Terrain
from .geo import Grid, from_wgs84, grid_around, point_in_crs, to_wgs84
from .lakes import Lake
from .stac import Scene, find_scenes
from .water import SCL_UNUSABLE, Composite, SceneObs, WaterParams, extract_lake, observe, read_scl

log = logging.getLogger(__name__)

SCL_SNOW = 11


@dataclass(frozen=True)
class DrainParams:
    lookback_days: int = 12  # two Sentinel-2 revisits; older looks say little about "now"
    min_drop: float = 0.30  # seasonal shrinkage and single-scene noise stay well below this
    sigmas: float = 3.0  # ... and the drop must also beat this many combined uncertainties
    min_coverage: float = 0.9  # share of the season outline the latest scene must have observed
    max_snow: float = 0.2  # more of the outline than this under snow/ice: frozen, not drained


@dataclass
class Season:
    """The "before": this season's composite lake."""

    year: int
    area_m2: float
    uncertainty_m2: float
    outline: np.ndarray  # bool mask on the lake's grid


@dataclass
class Look:
    """What one scene says about the lake."""

    status: str  # ok | no_clear_scene | frozen_or_snow
    day: str
    item_id: str
    coverage: float  # share of the season outline observed in this scene
    snow_fraction: float
    area_m2: float | None = None
    drop_fraction: float | None = None
    threshold_fraction: float | None = None
    drained: bool = False
    note: str = ""


def outline_fractions(scl: np.ndarray, outline: np.ndarray) -> tuple[float, float]:
    """(snow/ice share, usable share) of the season outline, from the cheap SCL band alone."""
    return float((scl[outline] == SCL_SNOW).mean()), float((~np.isin(scl[outline], SCL_UNUSABLE)).mean())


def unclear_look(obs_day: date, item_id: str, coverage: float, snow: float, p: DrainParams) -> Look | None:
    """A non-verdict if this scene cannot tell us about the lake, else None. Snow is checked first:
    a frozen lake is a known state, a cloudy one is not."""
    if snow > p.max_snow:
        note = f"{snow:.0%} of the lake outline is snow or ice: frozen lakes are never flagged"
        return Look("frozen_or_snow", obs_day.isoformat(), item_id, round(coverage, 3), round(snow, 3), note=note)
    if coverage < p.min_coverage:
        note = f"only {coverage:.0%} of the lake outline was clearly seen (need {p.min_coverage:.0%})"
        return Look("no_clear_scene", obs_day.isoformat(), item_id, round(coverage, 3), round(snow, 3), note=note)
    return None


def assess_scene(
    season: Season,
    obs: SceneObs,
    scl: np.ndarray,
    grid: Grid,
    slope: np.ndarray,
    seed_xy: tuple[float, float],
    wp: WaterParams = WaterParams(),
    p: DrainParams = DrainParams(),
) -> Look:
    """Compare the lake seen in one scene with the season's composite lake."""
    snow, _ = outline_fractions(scl, season.outline)
    coverage = float(obs.observed[season.outline].mean())
    if (unclear := unclear_look(obs.day, obs.item_id, coverage, snow, p)) is not None:
        return unclear
    # The same lake extraction as the yearly series, on a one-scene "composite".
    water = obs.water & (slope <= wp.max_slope_deg)
    comp = Composite(grid, water, obs.observed.astype("int16"), water.astype("float32"), [obs], [], min_obs=1)
    ext = extract_lake(comp, seed_xy, wp)
    area, unc = (ext.area_m2, ext.uncertainty_m2) if ext is not None else (0.0, 0.0)  # seen, and no water there
    drop = 1 - area / season.area_m2
    threshold = max(p.min_drop, p.sigmas * float(np.hypot(season.uncertainty_m2, unc)) / season.area_m2)
    drained = drop > threshold
    note = (
        f"lake area fell {drop:.0%} below this season's outline (flagged above {threshold:.0%})"
        if drained
        else f"no sudden loss of water: change {-drop:+.0%} against this season's outline"
    )
    return Look(
        "ok",
        obs.day.isoformat(),
        obs.item_id,
        round(coverage, 3),
        round(snow, 3),
        round(area, 1),
        round(drop, 3),
        round(threshold, 3),
        drained,
        note,
    )


def season_baseline(lake_dir: Path, as_of: date, grid: Grid) -> Season | None:
    """The latest season up to `as_of` whose lake was fully seen (status ok): the current one once it
    has a full outline, else last year's. Seasons after `as_of` are never used (no hindsight)."""
    series_path = lake_dir / "series.json"
    if not series_path.exists():
        return None
    for rec in sorted(json.loads(series_path.read_text())["years"], key=lambda r: -r["year"]):
        outline_path = lake_dir / f"{rec['year']}.geojson"
        if rec["year"] > as_of.year or rec["status"] != "ok" or not rec.get("area_m2") or not outline_path.exists():
            continue
        geom = from_wgs84(shape(json.loads(outline_path.read_text())["geometry"]), grid.crs)
        outline = features.rasterize([geom], out_shape=grid.shape, transform=grid.transform).astype(bool)
        if outline.any():
            return Season(rec["year"], rec["area_m2"], rec.get("uncertainty_m2") or 0.0, outline)
    return None


def latest_look(
    scenes: list[Scene],
    season: Season,
    grid: Grid,
    terrain: Terrain,
    seed_xy: tuple[float, float],
    wp: WaterParams = WaterParams(),
    p: DrainParams = DrainParams(),
) -> Look | None:
    """The newest scene that clearly saw the lake. Cloudy scenes are skipped for the next older one;
    a snowed-in newest look ends the search, because shrinking open water in the days before
    freeze-up is usually ice forming, not water leaving."""
    fallback: Look | None = None
    for scene in sorted(scenes, key=lambda s: s.day, reverse=True):
        scl = read_scl(scene, grid)
        snow, usable = outline_fractions(scl, season.outline)
        # SCL alone already rules the scene out: do not download its bands
        look = unclear_look(scene.day, scene.item_id, usable, snow, p)
        if look is None:
            look = assess_scene(
                season, observe(scene, scl, grid, terrain, wp), scl, grid, terrain.slope, seed_xy, wp, p
            )
        log.info("drain: %s %s coverage %.2f snow %.2f", scene.item_id, look.status, look.coverage, look.snow_fraction)
        if look.status == "ok" or (look.status == "frozen_or_snow" and fallback is None):
            return look
        fallback = fallback or look
    return fallback


def run_drain(
    lake: Lake,
    out_dir: Path,
    as_of: date | None = None,
    wp: WaterParams = WaterParams(),
    p: DrainParams = DrainParams(),
) -> dict:
    """Check one lake for sudden drainage and write lakes/<id>/drain.json."""
    as_of = as_of or datetime.now(UTC).date()
    lake_dir = out_dir / "lakes" / lake.id
    grid = grid_around(lake.lon, lake.lat, lake.aoi_radius_m)  # the grid its series was mapped on
    result = {
        "lake_id": lake.id,
        "as_of": as_of.isoformat(),
        "season_year": None,
        "season_area_m2": None,
        "latest": None,
        "drop_fraction": None,
        "threshold_fraction": None,
        "drained": False,
        "status": "not_found",
        "note": "no season with a fully seen lake outline to compare against (run `aahat series` first)",
        "params": asdict(p),
    }
    season = season_baseline(lake_dir, as_of, grid)
    if season is not None:
        result |= {"season_year": season.year, "season_area_m2": season.area_m2}
        scenes = find_scenes(to_wgs84(grid.polygon(), grid.crs), as_of - timedelta(days=p.lookback_days), as_of)
        seed = point_in_crs(lake.lon, lake.lat, grid.crs)
        look = latest_look(scenes, season, grid, Terrain(grid), seed, wp, p) if scenes else None
        result |= summarise(look, p)
    lake_dir.mkdir(parents=True, exist_ok=True)
    (lake_dir / "drain.json").write_text(json.dumps(result, indent=1))
    return result


def summarise(look: Look | None, p: DrainParams) -> dict:
    """The drain.json fields that come from the latest look (or from there being none)."""
    if look is None:
        return {"status": "no_clear_scene", "note": f"no Sentinel-2 scene in the last {p.lookback_days} days"}
    return {
        "latest": {"day": look.day, "item_id": look.item_id, "area_m2": look.area_m2, "coverage": look.coverage},
        "drop_fraction": look.drop_fraction,
        "threshold_fraction": look.threshold_fraction,
        "drained": look.drained,
        "status": look.status,
        "note": look.note,
    }
