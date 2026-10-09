"""Where a lake's outburst flood would go, how high it could rise, and when it would arrive.

SCREENING ESTIMATES, not hydrodynamic modelling:

1. Flow path: steepest descent from the lake's spill point over the Copernicus GLO-30 DEM, traced
   in moving 60 km windows (pits escaped by local priority flood, see hydro.py), up to `max_km`.
2. Stations every `station_m` along the path. The DEM's river-surface profile is made
   monotonic (running minimum) and its slope taken over +/- `slope_window_m`.
3. Flood level per station: a DEM cross-section perpendicular to the path, and the water
   surface at which Manning's equation (normal depth) carries the peak discharge there.
4. Inundation corridor: DEM cells within `corridor_m` of the path that lie below the flood
   level of their nearest station and connect to the river.
5. Arrival time: along-path distance divided by a flood-front speed taken from observed events.

The DEM is a surface model of 2011-2015 with no river bathymetry, and Manning normal depth ignores
storage, backwater, debris bulking and the flood wave's shape, so depths and extents are first-order.
"""

from __future__ import annotations

import logging
import math
from dataclasses import asdict, dataclass, field

import numpy as np
from affine import Affine
from rasterio import features
from scipy import ndimage
from shapely.geometry import LineString, Point, mapping, shape
from shapely.ops import unary_union

from .dem import read_dem
from .geo import Grid, from_wgs84, to_wgs84, utm_crs
from .hydro import flow_path

log = logging.getLogger(__name__)


@dataclass(frozen=True)
class DownstreamParams:
    res_m: float = 30.0
    max_km: float = 150.0
    window_m: float = 30_000.0  # half-size of each tracing window
    station_m: float = 250.0
    slope_window_m: float = 1000.0
    section_half_m: float = 2000.0  # cross-section half-width
    min_slope: float = 0.002  # floor for Manning's S on flat reaches
    manning_n: float = 0.07
    corridor_m: float = 2000.0
    # flood-front speeds (m/s) for arrival times: expected and fast cases
    speed_expected: float = 8.0
    speed_fast: float = 10.0


@dataclass
class Station:
    km: float
    lon: float
    lat: float
    bed_m: float
    slope: float
    discharge_m3s: float
    wse_m: float  # flood water surface elevation
    depth_m: float
    width_m: float
    arrival_min_expected: float
    arrival_min_fast: float
    section_capped: bool  # the water reached the end of the cross-section: depth may be understated


@dataclass
class Downstream:
    crs: str
    path_xy: list[tuple[float, float]]  # in crs
    stations: list[Station]
    corridor: object  # shapely geometry in WGS84
    params: dict
    discharge: dict = field(default_factory=dict)
    # working arrays for impact assessment (not serialised)
    grid: Grid | None = field(default=None, repr=False)
    dem: np.ndarray | None = field(default=None, repr=False)
    station_xy: np.ndarray | None = field(default=None, repr=False)


# ----------------------------------------------------------------------------------------------
# 1. Flow path


def trace_path(start_lonlat: tuple[float, float], blocked_wgs84, p: DownstreamParams) -> tuple[str, np.ndarray]:
    """Trace downhill from `start_lonlat` for up to p.max_km; returns (crs, Nx2 array of xy in that crs).

    `blocked_wgs84` (the lake and its outlet reach) is never entered.
    """
    crs = utm_crs(*start_lonlat)
    x, y = from_wgs84(Point(start_lonlat), crs).coords[0]
    path_xy: list[tuple[float, float]] = [(x, y)]
    length = 0.0
    blocked_geoms = [from_wgs84(blocked_wgs84, crs)] if blocked_wgs84 is not None else []
    for _ in range(20):  # windows
        cx, cy = path_xy[-1]
        n = round(2 * p.window_m / p.res_m)
        left = math.floor((cx - p.window_m) / p.res_m) * p.res_m
        top = math.ceil((cy + p.window_m) / p.res_m) * p.res_m
        grid = Grid(crs, Affine(p.res_m, 0, left, 0, -p.res_m, top), n, n)
        dem = read_dem(grid)
        blocked = np.zeros(grid.shape, bool)
        previous = blocked_geoms + ([LineString(path_xy[:-3])] if len(path_xy) > 4 else [])
        if previous:
            blocked = features.rasterize(
                previous, out_shape=grid.shape, transform=grid.transform, all_touched=True
            ).astype(bool)
        start = grid.xy_to_rowcol(cx, cy)
        blocked[start] = False
        cells = flow_path(dem, start, grid.res, max_length_m=p.max_km * 1000 - length, blocked=blocked)
        xy = [grid.transform @ (c + 0.5, r + 0.5) for r, c in cells[1:]]
        if not xy:
            break
        seg = np.vstack([path_xy[-1:], xy])
        length += float(np.hypot(*np.diff(seg, axis=0).T).sum())
        path_xy += xy
        r, c = cells[-1]
        at_edge = r in (0, grid.height - 1) or c in (0, grid.width - 1)
        if length >= p.max_km * 1000 or not at_edge:
            break
    return crs.to_string(), np.array(path_xy)


# ----------------------------------------------------------------------------------------------
# 2-3. Stations and Manning normal depth on DEM cross-sections


def manning_discharge(
    offsets: np.ndarray, ground: np.ndarray, wse: float, slope: float, n: float, centre: int | None = None
) -> tuple[float, float, bool]:
    """Discharge (m3/s) and top width (m) of the section for water surface `wse`.

    Only the wetted span connected to the section centre counts (no flow across a ridge into
    another channel). Returns (Q, width, capped) where capped means the water reached an end.
    """
    mid = len(ground) // 2 if centre is None else centre
    if ground[mid] >= wse:
        return 0.0, 0.0, False
    wet = ground < wse
    lo = mid
    while lo > 0 and wet[lo - 1]:
        lo -= 1
    hi = mid
    while hi < len(ground) - 1 and wet[hi + 1]:
        hi += 1
    capped = lo == 0 or hi == len(ground) - 1
    o, z = offsets[lo : hi + 1], ground[lo : hi + 1]
    d = wse - z
    dx = np.gradient(o) if len(o) > 1 else np.array([1.0])
    area = float(np.sum(d * dx))
    perim = float(np.sum(np.hypot(np.diff(o), np.diff(z)))) + 2 * float(d[0] + d[-1]) / 2
    if area <= 0 or perim <= 0:
        return 0.0, 0.0, capped
    radius = area / perim
    q = area * radius ** (2 / 3) * math.sqrt(slope) / n
    return q, float(o[-1] - o[0]), capped


def channel_centre(ground: np.ndarray, search: int = 3) -> int:
    """The lowest cell within `search` cells of the section middle: the river, if the path is slightly off."""
    mid = len(ground) // 2
    lo = max(0, mid - search)
    return lo + int(np.argmin(ground[lo : mid + search + 1]))


def normal_depth_wse(offsets, ground, discharge, slope, n, max_depth=150.0) -> tuple[float, float, bool]:
    """Water surface at which the section carries `discharge` (bisection). Returns (wse, width, capped)."""
    centre = channel_centre(ground)
    bed = float(ground[centre])
    lo, hi = bed, bed + max_depth
    for _ in range(60):
        mid = (lo + hi) / 2
        q, _, _ = manning_discharge(offsets, ground, mid, slope, n, centre)
        if q < discharge:
            lo = mid
        else:
            hi = mid
    q, width, capped = manning_discharge(offsets, ground, hi, slope, n, centre)
    return hi, width, capped


def _resample(path_xy: np.ndarray, step: float) -> tuple[np.ndarray, np.ndarray]:
    seg = np.hypot(*np.diff(path_xy, axis=0).T)
    chain = np.concatenate([[0.0], np.cumsum(seg)])
    s = np.arange(0.0, chain[-1], step)
    return s, np.column_stack([np.interp(s, chain, path_xy[:, 0]), np.interp(s, chain, path_xy[:, 1])])


def mosaic_grid(crs, xy: np.ndarray, margin_m: float, res: float) -> Grid:
    from pyproj import CRS

    crs = CRS.from_user_input(crs)
    left = math.floor((xy[:, 0].min() - margin_m) / res) * res
    right = math.ceil((xy[:, 0].max() + margin_m) / res) * res
    bottom = math.floor((xy[:, 1].min() - margin_m) / res) * res
    top = math.ceil((xy[:, 1].max() + margin_m) / res) * res
    return Grid(crs, Affine(res, 0, left, 0, -res, top), round((right - left) / res), round((top - bottom) / res))


def sample(dem: np.ndarray, grid: Grid, xs: np.ndarray, ys: np.ndarray) -> np.ndarray:
    cols, rows = ~grid.transform @ (xs, ys)
    return ndimage.map_coordinates(dem, [rows - 0.5, cols - 0.5], order=1, mode="nearest")


def discharge_at(km: float, peak_m3s: float) -> float:
    """Peak discharge at `km` downstream. No attenuation yet: conservative (keeps the breach peak)."""
    return peak_m3s


def route(
    crs: str, path_xy: np.ndarray, peak_m3s: float, p: DownstreamParams
) -> tuple[list[Station], Grid, np.ndarray, np.ndarray]:
    grid = mosaic_grid(crs, path_xy, p.section_half_m + p.corridor_m + 500, p.res_m)
    dem = read_dem(grid)
    s, sxy = _resample(path_xy, p.station_m)
    bed = sample(dem, grid, sxy[:, 0], sxy[:, 1])
    bed = np.minimum.accumulate(np.nan_to_num(bed, nan=np.inf))  # rivers only go down: used for slope
    # slope over +/- slope_window_m
    k = max(1, int(p.slope_window_m / p.station_m))
    slope = np.empty_like(bed)
    for i in range(len(bed)):
        a, b = max(0, i - k), min(len(bed) - 1, i + k)
        slope[i] = max((bed[a] - bed[b]) / max(s[b] - s[a], 1.0), p.min_slope)
    # direction: tangent over +/- 2 stations
    stations = []
    offsets = np.arange(-p.section_half_m, p.section_half_m + p.res_m, p.res_m)
    for i in range(len(s)):
        a, b = max(0, i - 2), min(len(s) - 1, i + 2)
        tx, ty = sxy[b] - sxy[a]
        norm = math.hypot(tx, ty) or 1.0
        nx, ny = -ty / norm, tx / norm
        ground = sample(dem, grid, sxy[i, 0] + offsets * nx, sxy[i, 1] + offsets * ny)
        q = discharge_at(s[i] / 1000, peak_m3s)
        wse, width, capped = normal_depth_wse(offsets, ground, q, float(slope[i]), p.manning_n)
        pt = to_wgs84(Point(*sxy[i]), grid.crs)
        stations.append(
            Station(
                km=round(s[i] / 1000, 3),
                lon=round(pt.x, 6),
                lat=round(pt.y, 6),
                bed_m=round(float(bed[i]), 1),
                slope=round(float(slope[i]), 4),
                discharge_m3s=round(q),
                wse_m=round(wse, 1),
                depth_m=round(wse - float(ground[channel_centre(ground)]), 1),
                width_m=round(width),
                arrival_min_expected=round(s[i] / p.speed_expected / 60, 1),
                arrival_min_fast=round(s[i] / p.speed_fast / 60, 1),
                section_capped=capped,
            )
        )
    return stations, grid, dem, sxy


# ----------------------------------------------------------------------------------------------
# 4. Inundation corridor


def corridor(stations: list[Station], grid: Grid, dem: np.ndarray, sxy: np.ndarray, p: DownstreamParams):
    """Cells below their nearest station's flood level, within p.corridor_m of the path, connected to it."""
    path_mask = np.zeros(grid.shape, bool)
    station_id = np.full(grid.shape, -1, int)
    for i, (x, y) in enumerate(sxy):
        r, c = grid.xy_to_rowcol(x, y)
        if 0 <= r < grid.height and 0 <= c < grid.width:
            path_mask[r, c] = True
            station_id[r, c] = i
    dist, (ir, ic) = ndimage.distance_transform_edt(~path_mask, return_indices=True)
    nearest = station_id[ir, ic]
    wse = np.array([st.wse_m for st in stations])[nearest]
    wet = (dist * grid.res <= p.corridor_m) & np.isfinite(dem) & (dem < wse)
    wet |= path_mask
    labels, _ = ndimage.label(wet, structure=np.ones((3, 3)))
    wet = np.isin(labels, np.unique(labels[path_mask]))
    polys = [shape(g) for g, v in features.shapes(wet.astype("uint8"), mask=wet, transform=grid.transform) if v == 1]
    geom = unary_union(polys).simplify(grid.res / 2)
    return to_wgs84(geom, grid.crs)


def compute_downstream(
    start_lonlat, blocked_wgs84, peak_m3s: float, p: DownstreamParams = DownstreamParams()
) -> Downstream:
    crs, path_xy = trace_path(start_lonlat, blocked_wgs84, p)
    log.info("traced %.1f km", float(np.hypot(*np.diff(path_xy, axis=0).T).sum()) / 1000)
    stations, grid, dem, sxy = route(crs, path_xy, peak_m3s, p)
    geom = corridor(stations, grid, dem, sxy, p)
    return Downstream(crs, [tuple(map(float, xy)) for xy in path_xy], stations, geom, asdict(p), {}, grid, dem, sxy)


def path_geojson(d: Downstream) -> dict:
    from pyproj import CRS

    line = to_wgs84(LineString(d.path_xy), CRS.from_user_input(d.crs))
    return {
        "type": "Feature",
        "geometry": mapping(line.simplify(0.0001)),
        "properties": {"km": d.stations[-1].km if d.stations else 0},
    }
