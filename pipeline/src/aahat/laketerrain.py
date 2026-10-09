"""Terrain around a lake outline, from the Copernicus GLO-30 DEM.

- Lake level: the DEM's flattened water surface (most common elevation inside the outline). The
  outline is first grown over that flat surface, since a newer outline need not match it.
- Outlet: the lowest DEM cell on the ring around that surface, i.e. where the lake spills.
- Outlet steepness: mean gradient of the steepest-descent path over the first `run_m` metres
  below the outlet. A steep drop below the dam favours rapid, erosive breaching.
- Steep slopes above the lake: area of cells within `buffer_m` of the shore that sit above lake
  level and are at least `steep_deg` steep, the source zones for ice or rock avalanches and
  rockfall that can hit the lake and send a wave over the dam.

The DEM was acquired 2011-2015 (TanDEM-X), before every season we score, so using it does not
leak future information into a replay.
"""

from __future__ import annotations

import math
from dataclasses import asdict, dataclass

from rasterio import features
from scipy import ndimage
from shapely.geometry import Point, mapping, shape

from .dem import read_dem, slope_deg
from .geo import from_wgs84, grid_around, to_wgs84
from .hydro import flow_path, outlet_cell, path_length_m, spill_mask, water_level


@dataclass(frozen=True)
class TerrainParams:
    res_m: float = 30.0
    buffer_m: float = 1000.0
    steep_deg: float = 30.0
    run_m: float = 1000.0
    above_lake_m: float = 10.0


@dataclass
class LakeTerrain:
    lake_level_m: float
    outlet_lon: float
    outlet_lat: float
    outlet_run_m: float
    outlet_drop_m: float
    outlet_slope_deg: float
    surface_outlet: bool  # False when the ground below the spill point does not fall (seepage, closed basin)
    steep_area_km2: float
    steep_share: float  # of the land above lake level within buffer_m
    outlet_path: list[list[float]]  # [lon, lat] of the first run_m below the outlet
    params: dict

    def to_dict(self) -> dict:
        return asdict(self)


def lake_terrain(outline_wgs84: dict | object, p: TerrainParams = TerrainParams()) -> LakeTerrain:
    geom = shape(outline_wgs84) if isinstance(outline_wgs84, dict) else outline_wgs84
    c = geom.centroid
    minx, miny, maxx, maxy = geom.bounds
    half_extent_m = max(maxx - minx, maxy - miny) * 111_000 / 2
    grid = grid_around(c.x, c.y, half_extent_m + p.buffer_m + p.run_m + 500, res=p.res_m)
    dem = read_dem(grid)
    lake = features.rasterize(
        [from_wgs84(geom, grid.crs)], out_shape=grid.shape, transform=grid.transform, all_touched=True
    ).astype(bool)
    if not lake.any():  # tiny lake: keep at least the centroid cell
        lake[grid.xy_to_rowcol(*from_wgs84(c, grid.crs).coords[0])] = True
    level = water_level(dem, lake)
    lake = spill_mask(dem, lake, grid.res)

    r0, c0 = outlet_cell(dem, lake)
    path = flow_path(dem, (r0, c0), grid.res, max_length_m=p.run_m, blocked=lake)
    run = float(path_length_m(path, grid.res)[-1])
    drop = float(level - dem[path[-1]])
    surface_outlet = drop > 2.0
    outlet_slope = math.degrees(math.atan2(drop, run)) if run > 0 and surface_outlet else 0.0

    slope = slope_deg(dem, grid.res)
    dist = ndimage.distance_transform_edt(~lake) * grid.res
    zone = (dist > 0) & (dist <= p.buffer_m) & (dem > level + p.above_lake_m)
    steep = zone & (slope >= p.steep_deg)
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
        surface_outlet=surface_outlet,
        steep_area_km2=round(float(steep.sum()) * cell_km2, 4),
        steep_share=round(float(steep.sum() / max(zone.sum(), 1)), 4),
        outlet_path=[lonlat(rc) for rc in path],
        params=asdict(p),
    )


def outlet_geojson(t: LakeTerrain) -> dict:
    """The outlet and the first stretch of its flow path, for maps."""
    return {
        "type": "FeatureCollection",
        "features": [
            {"type": "Feature", "geometry": mapping(Point(t.outlet_lon, t.outlet_lat)), "properties": {"kind": "outlet"}},
            {
                "type": "Feature",
                "geometry": {"type": "LineString", "coordinates": t.outlet_path},
                "properties": {"kind": "outlet_path", "run_m": t.outlet_run_m, "slope_deg": t.outlet_slope_deg},
            },
        ],
    }
