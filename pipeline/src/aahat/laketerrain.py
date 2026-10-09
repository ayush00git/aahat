"""Terrain around a lake outline, from the Copernicus GLO-30 DEM.

- Lake level: the DEM's flattened water surface (most common elevation inside the outline). The
  outline is first grown over that flat surface, since a newer outline need not match it.
- Outlet: the lowest DEM cell on the ring around that surface, i.e. where the lake spills.
- Outlet steepness: mean gradient of the steepest-descent path over the first `run_m` metres
  below the outlet. A steep drop below the dam favours rapid, erosive breaching.
- Avalanche and rockfall sources (Allen et al. 2016, Himachal Pradesh,
  https://doi.org/10.1007/s11069-016-2511-x): cells at least `steep_deg` (30°) steep whose
  straight line down to the nearest lake cell is steeper than `reach_deg` (14°), i.e. ice or rock
  released there can reach the lake and send a wave over the dam. Only cells whose D8 downhill
  path ends in the lake count (Allen et al. route the mass flow; a straight line would count
  slopes in the next valley). Searched to `reach_search_m`.

The DEM was acquired 2011-2015 (TanDEM-X), before every season we score, so using it does not
leak future information into a replay.
"""

from __future__ import annotations

import math
from dataclasses import asdict, dataclass

import numpy as np
from rasterio import features
from scipy import ndimage
from shapely.geometry import Point, mapping, shape

from .dem import read_dem, slope_deg
from .geo import from_wgs84, grid_around, to_wgs84
from .hydro import drains_to, flow_path, outlet_cell, path_length_m, spill_mask, water_level


@dataclass(frozen=True)
class TerrainParams:
    res_m: float = 30.0
    steep_deg: float = 30.0
    reach_deg: float = 14.0
    reach_search_m: float = 5000.0
    run_m: float = 1000.0
    above_lake_m: float = 10.0
    spill_candidates: int = 40
    fall_m: float = 2.0  # a path that drops less than this over run_m means no surface outlet


@dataclass
class LakeTerrain:
    lake_level_m: float
    outlet_lon: float
    outlet_lat: float
    outlet_run_m: float
    outlet_drop_m: float
    outlet_slope_deg: float
    surface_outlet: bool  # False when the ground below the spill point does not fall (seepage, closed basin)
    avalanche_area_km2: float  # source cells that can reach the lake
    outlet_path: list[list[float]]  # [lon, lat] of the first run_m below the outlet
    params: dict

    def to_dict(self) -> dict:
        return asdict(self)


def lake_terrain(outline_wgs84: dict | object, p: TerrainParams = TerrainParams()) -> LakeTerrain:
    geom = shape(outline_wgs84) if isinstance(outline_wgs84, dict) else outline_wgs84
    c = geom.centroid
    minx, miny, maxx, maxy = geom.bounds
    half_extent_m = max(maxx - minx, maxy - miny) * 111_000 / 2
    grid = grid_around(c.x, c.y, half_extent_m + max(p.reach_search_m, p.run_m) + 500, res=p.res_m)
    dem = read_dem(grid)
    lake = features.rasterize(
        [from_wgs84(geom, grid.crs)], out_shape=grid.shape, transform=grid.transform, all_touched=True
    ).astype(bool)
    if not lake.any():  # tiny lake: keep at least the centroid cell
        lake[grid.xy_to_rowcol(*from_wgs84(c, grid.crs).coords[0])] = True
    level = water_level(dem, lake)
    lake = spill_mask(dem, lake, grid.res)

    # Spill point: of the lowest shore cells, the one whose downhill path has dropped the
    # most after `run_m`, i.e. where water actually drains away. (The single lowest shore cell can
    # be a DEM dip, e.g. a pit on the glacier surface beside a proglacial lake.)
    ring = ndimage.binary_dilation(lake, structure=np.ones((3, 3))) & ~lake & np.isfinite(dem)
    rr, cc = np.nonzero(ring)
    order = np.argsort(dem[rr, cc])[: p.spill_candidates]
    best_drop, path = -np.inf, [outlet_cell(dem, lake)]
    for i in order:
        cand = flow_path(dem, (int(rr[i]), int(cc[i])), grid.res, max_length_m=p.run_m, blocked=lake)
        drop = level - dem[cand[-1]]
        if drop > best_drop:
            best_drop, path = drop, cand
    surface_outlet = best_drop > p.fall_m
    r0, c0 = path[0]
    run = float(path_length_m(path, grid.res)[-1])
    drop = float(level - dem[path[-1]])
    outlet_slope = math.degrees(math.atan2(drop, run)) if surface_outlet and run > 0 else 0.0

    slope = slope_deg(dem, grid.res)
    dist = ndimage.distance_transform_edt(~lake) * grid.res
    with np.errstate(divide="ignore", invalid="ignore"):
        reach = np.degrees(np.arctan2(dem - level, dist))
    # only slopes whose downhill path actually ends in the lake (not across a ridge)
    catchment = drains_to(dem, lake)
    steep = catchment & (dist > 0) & (dist <= p.reach_search_m) & (slope >= p.steep_deg) & (reach > p.reach_deg)
    cell_km2 = grid.res**2 / 1e6

    def lonlat(rc):
        x, y = grid.transform @ (rc[1] + 0.5, rc[0] + 0.5)
        pt = to_wgs84(Point(x, y), grid.crs)
        return [round(pt.x, 6), round(pt.y, 6)]

    out = lonlat((r0, c0))
    return LakeTerrain(
        lake_level_m=round(level, 1),
        outlet_lon=out[0],
        outlet_lat=out[1],
        outlet_run_m=round(run, 1),
        outlet_drop_m=round(drop, 1),
        outlet_slope_deg=round(outlet_slope, 2),
        surface_outlet=bool(surface_outlet),
        avalanche_area_km2=round(float(steep.sum()) * cell_km2, 4),
        outlet_path=[lonlat(rc) for rc in path],
        params=asdict(p),
    )


def outlet_geojson(t: LakeTerrain) -> dict:
    """The outlet and the first stretch of its flow path, for maps."""
    return {
        "type": "FeatureCollection",
        "features": [
            {
                "type": "Feature",
                "geometry": mapping(Point(t.outlet_lon, t.outlet_lat)),
                "properties": {"kind": "outlet"},
            },
            {
                "type": "Feature",
                "geometry": {"type": "LineString", "coordinates": t.outlet_path},
                "properties": {"kind": "outlet_path", "run_m": t.outlet_run_m, "slope_deg": t.outlet_slope_deg},
            },
        ],
    }
