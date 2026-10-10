"""New barrier lakes: water appearing in a river channel where there was none before.

A landslide or debris flow that blocks a river backs up a lake behind it (Parechu 2004, the
barrier dams after the August 2026 Nepal-Tibet floods). From Sentinel-2 that looks like a new,
lake-shaped body of water on the river line. Per window along a river reach:

1. "now": water from the clearest scenes of the last `now_days` before `as_of`;
2. "before": water frequency over the baseline period (`baseline_days` back, ending
   `baseline_gap_days` before `as_of`), so a lake that formed recently is not in its own baseline;
3. candidates: water now, (almost) never water before though clearly seen, within `corridor_m`
   of the river line, at least `min_area_m2` and `min_width_m` wide (a river widening along its
   banks makes long slivers, a dammed lake a blob).

A reservoir refilling behind a dam looks exactly the same (Chamera on the Ravi, October 2026), so
each candidate near a mapped dam, weir or hydro plant is tagged `reservoir_level_change` instead of
`possible_barrier_lake`.

Only scenes up to `as_of` are used, so a replay of a past date shows what could have been seen then.
Reads at 20 m from COG overviews to keep downloads small: lakes of interest are tens of pixels.
"""

from __future__ import annotations

import logging
from dataclasses import asdict, dataclass
from datetime import date, timedelta

import numpy as np
from rasterio import features
from scipy import ndimage
from shapely.geometry import LineString, Point

from .dem import Terrain
from .exposure import fetch_assets
from .geo import Grid, from_wgs84, grid_around, to_wgs84, utm_crs
from .stac import find_scenes
from .water import WaterParams, composite

log = logging.getLogger(__name__)


@dataclass(frozen=True)
class BarrierParams:
    res_m: float = 20.0
    window_radius_m: float = 3000.0
    spacing_km: float = 5.0
    corridor_m: float = 1000.0
    now_days: int = 20
    baseline_days: int = 400
    baseline_gap_days: int = 30
    max_baseline_freq: float = 0.1
    min_baseline_obs: int = 2
    min_area_m2: float = 20_000.0
    min_width_m: float = 60.0
    max_scenes_now: int = 4
    max_scenes_baseline: int = 8
    # Chamera dam is 8.4 km in a straight line from the head of its reservoir, where the refill showed.
    dam_radius_km: float = 12.0


@dataclass
class Candidate:
    lon: float
    lat: float
    area_m2: float
    width_m: float  # of the new water: twice the largest distance from its edge to its interior
    km_along_reach: float
    baseline_water_freq: float  # mean over the blob in the baseline period
    now_scenes: list[str]
    as_of: str
    near_dam_km: float | None = None  # straight line to the nearest mapped dam/weir/hydro plant within reach
    kind: str = "possible_barrier_lake"  # | "reservoir_level_change"


def new_water_blobs(
    now_water: np.ndarray,
    base_freq: np.ndarray,
    base_obs: np.ndarray,
    corridor: np.ndarray,
    res: float,
    p: BarrierParams,
) -> list[tuple[np.ndarray, float, float]]:
    """(mask, area_m2, width_m) for each new-water blob that passes the filters."""
    was_dry = (base_obs >= p.min_baseline_obs) & (np.nan_to_num(base_freq, nan=1.0) <= p.max_baseline_freq)
    new = now_water & was_dry & corridor
    # A dammed lake straddles the river, whose channel was water before: join the halves through
    # old water right next to new water (not the whole river, which would chain everything together).
    joined = new | (now_water & ~was_dry & corridor & ndimage.binary_dilation(new, iterations=3))
    labels, n = ndimage.label(joined, structure=np.ones((3, 3)))
    out = []
    for i in range(1, n + 1):
        blob = labels == i
        if float((blob & new).sum()) * res * res < p.min_area_m2:  # the new part alone must be lake-sized
            continue
        area = float(blob.sum()) * res * res
        # width of the new water alone: a bank overtopped along the river stays a thin sliver even
        # though it touches the old channel
        width = 2 * float(ndimage.distance_transform_edt(blob & new).max()) * res
        if width < p.min_width_m:
            continue
        out.append((blob, area, width))
    return out


def _corridor_mask(grid: Grid, reach_xy: LineString, corridor_m: float) -> np.ndarray:
    return features.rasterize([reach_xy.buffer(corridor_m)], out_shape=grid.shape, transform=grid.transform).astype(
        bool
    )


def scan_window(
    center_lonlat: tuple[float, float], reach_wgs84: LineString, as_of: date, km_offset: float, p: BarrierParams
) -> list[Candidate]:
    grid = grid_around(*center_lonlat, p.window_radius_m, res=p.res_m)
    reach = from_wgs84(reach_wgs84, grid.crs)
    corridor = _corridor_mask(grid, reach, p.corridor_m)
    if not corridor.any():
        return []
    terrain = Terrain(grid)
    aoi = to_wgs84(grid.polygon(), grid.crs)
    wp = WaterParams()
    now_scenes = find_scenes(aoi, as_of - timedelta(days=p.now_days), as_of)
    base_scenes = find_scenes(aoi, as_of - timedelta(days=p.baseline_days), as_of - timedelta(days=p.baseline_gap_days))
    if not now_scenes or not base_scenes:
        return []
    now = composite(now_scenes, grid, terrain, wp, max_scenes=p.max_scenes_now, focus=corridor)
    if not now.inputs or not now.water[corridor].any():
        return []
    base = composite(base_scenes, grid, terrain, wp, max_scenes=p.max_scenes_baseline, focus=corridor)
    found = []
    for blob, area, width in new_water_blobs(now.water, base.water_freq, base.obs_count, corridor, grid.res, p):
        r, c = ndimage.center_of_mass(blob)
        x, y = grid.transform @ (c + 0.5, r + 0.5)
        pt = to_wgs84(Point(x, y), grid.crs)
        found.append(
            Candidate(
                lon=round(pt.x, 5),
                lat=round(pt.y, 5),
                area_m2=round(area),
                width_m=round(width),
                km_along_reach=round(km_offset + reach.project(Point(x, y)) / 1000, 2),
                baseline_water_freq=round(float(np.nanmean(base.water_freq[blob])), 3),
                now_scenes=[s.item_id for s in now.inputs],
                as_of=as_of.isoformat(),
            )
        )
    return found


def scan_reach(
    reach_wgs84: LineString, as_of: date, p: BarrierParams = BarrierParams(), max_km: float | None = None
) -> list[Candidate]:
    """Scan windows every p.spacing_km along a river reach (lon/lat line) for new barrier lakes."""
    start = reach_wgs84.coords[0]
    crs = utm_crs(*start)
    reach_xy = from_wgs84(reach_wgs84, crs)
    length_km = reach_xy.length / 1000
    if max_km is not None:
        length_km = min(length_km, max_km)
    candidates: list[Candidate] = []
    seen: set[tuple[float, float]] = set()
    for km in np.arange(0, length_km + 1e-9, p.spacing_km):
        center = to_wgs84(reach_xy.interpolate(km * 1000), crs)
        log.info("barrier scan: km %.0f of %.0f", km, length_km)
        for cand in scan_window((center.x, center.y), reach_wgs84, as_of, 0.0, p):
            # windows overlap: keep each blob once
            key = (round(cand.lon, 3), round(cand.lat, 3))
            if key not in seen:
                seen.add(key)
                cand.km_along_reach = round(reach_xy.project(from_wgs84(Point(cand.lon, cand.lat), crs)) / 1000, 2)
                candidates.append(cand)
    tag_reservoirs(candidates, p)
    return sorted(candidates, key=lambda c: c.km_along_reach)


def nearest_dam_km(lon: float, lat: float, radius_km: float) -> float | None:
    """Straight-line km from a point to the nearest OSM dam, weir or hydro plant within `radius_km`
    (to the nearest point of its outline, not its centre: dams are long). None if there is none.

    Every "hydro" asset counts, not only waterway=dam: Chamera Dam is mapped as a power plant.
    """
    dlat = radius_km / 111.0
    dlon = dlat / np.cos(np.radians(lat))
    crs = utm_crs(lon, lat)
    here = from_wgs84(Point(lon, lat), crs)
    dists = [
        here.distance(from_wgs84(a.geometry, crs)) / 1000
        for a in fetch_assets((lon - dlon, lat - dlat, lon + dlon, lat + dlat))
        if a.kind == "hydro"
    ]
    near = [d for d in dists if d <= radius_km]
    return round(min(near), 2) if near else None


def tag_reservoirs(cands: list[Candidate], p: BarrierParams = BarrierParams()) -> None:
    """Mark candidates near a dam as a reservoir changing level rather than a possible barrier lake."""
    for c in cands:
        try:
            c.near_dam_km = nearest_dam_km(c.lon, c.lat, p.dam_radius_km)
        except Exception as e:  # noqa: BLE001 - no OSM answer (Overpass down): leave it a possible barrier lake
            log.warning("dam lookup failed for %.4f,%.4f: %s", c.lat, c.lon, e)
            continue
        c.kind = "reservoir_level_change" if c.near_dam_km is not None else "possible_barrier_lake"


def candidates_json(cands: list[Candidate]) -> list[dict]:
    return [asdict(c) for c in cands]
