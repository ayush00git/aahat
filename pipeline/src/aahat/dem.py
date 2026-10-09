"""Copernicus DEM GLO-30 from AWS Open Data, plus terrain derivatives (slope, cast shadow)."""

from __future__ import annotations

import math

import numpy as np
from rasterio.enums import Resampling
from rasterio.warp import reproject

from .cache import cached_file
from .geo import Grid
from .raster import read_to_grid

DEM_BUCKET = "https://copernicus-dem-30m.s3.eu-central-1.amazonaws.com"


def tile_href(lat: int, lon: int) -> str:
    ns = "N" if lat >= 0 else "S"
    ew = "E" if lon >= 0 else "W"
    name = f"Copernicus_DSM_COG_10_{ns}{abs(lat):02d}_00_{ew}{abs(lon):03d}_00_DEM"
    return f"{DEM_BUCKET}/{name}/{name}.tif"


def tiles_for(grid: Grid) -> list[str]:
    lon0, lat0, lon1, lat1 = grid.bounds_wgs84()
    return [
        tile_href(lat, lon)
        for lat in range(math.floor(lat0), math.floor(lat1) + 1)
        for lon in range(math.floor(lon0), math.floor(lon1) + 1)
    ]


def read_dem(grid: Grid) -> np.ndarray:
    """Elevation (m, EGM2008) on `grid`, mosaicking 1x1 degree tiles as needed.

    Whole tiles are cached on local disk: they are reused across lakes and for downstream routing.
    Tiles that do not exist (open water, no coverage) are skipped.
    """
    out = np.full(grid.shape, np.nan, dtype="float32")
    for href in tiles_for(grid):
        path = cached_file(href)
        if path is None:
            continue
        part = read_to_grid(str(path), grid, Resampling.bilinear)
        fill = np.isnan(out) & ~np.isnan(part)
        out[fill] = part[fill]
    return out


def slope_deg(dem: np.ndarray, res: float) -> np.ndarray:
    dzdy, dzdx = np.gradient(dem, res)
    return np.degrees(np.arctan(np.hypot(dzdx, dzdy))).astype("float32")


def cast_shadow(dem: np.ndarray, res: float, sun_azimuth: float, sun_elevation: float) -> np.ndarray:
    """True where terrain blocks the sun (cast shadow and self-shadow), by marching toward the sun.

    Azimuth is degrees clockwise from north; the grid is north-up.
    """
    z = np.where(np.isnan(dem), -1e4, dem).astype("float32")
    tan_el = math.tan(math.radians(max(sun_elevation, 1.0)))
    relief = float(np.nanmax(dem) - np.nanmin(dem))
    max_dist = relief / tan_el
    az = math.radians(sun_azimuth)
    dcol, drow = math.sin(az), -math.cos(az)  # unit step toward the sun in (col, row)
    h, w = z.shape
    shadow = np.zeros(z.shape, dtype=bool)
    steps = int(max_dist / res) + 1
    for k in range(1, steps + 1):
        dr, dc = round(drow * k), round(dcol * k)
        if abs(dr) >= h or abs(dc) >= w:
            break
        # shifted[r, c] = z[r + dr, c + dc] (the terrain k steps toward the sun)
        shifted = np.full_like(z, -1e4)
        rs, re = max(0, -dr), min(h, h - dr)
        cs, ce = max(0, -dc), min(w, w - dc)
        shifted[rs:re, cs:ce] = z[rs + dr : re + dr, cs + dc : ce + dc]
        dist = res * math.hypot(dr, dc)
        shadow |= shifted > z + dist * tan_el
    return shadow


def resample_mask(mask: np.ndarray, src: Grid, dst: Grid) -> np.ndarray:
    out = np.zeros(dst.shape, dtype="uint8")
    reproject(
        mask.astype("uint8"),
        out,
        src_transform=src.transform,
        src_crs=src.crs,
        dst_transform=dst.transform,
        dst_crs=dst.crs,
        resampling=Resampling.nearest,
    )
    return out.astype(bool)


class Terrain:
    """DEM context for one AOI: a coarse buffered DEM for shadows, and slope on the AOI grid."""

    def __init__(self, grid: Grid, shadow_margin_m: float = 6000.0):
        self.grid = grid
        self.coarse = grid.with_resolution(30.0).buffered(shadow_margin_m)
        self.dem_coarse = read_dem(self.coarse)
        self.dem = read_dem(grid)
        self.slope = slope_deg(self.dem, grid.res)

    def shadow(self, sun_azimuth: float, sun_elevation: float) -> np.ndarray:
        s = cast_shadow(self.dem_coarse, self.coarse.res, sun_azimuth, sun_elevation)
        return resample_mask(s, self.coarse, self.grid)
