"""Grids, projections and small geometry helpers."""

from __future__ import annotations

from dataclasses import dataclass

import numpy as np
from affine import Affine
from pyproj import CRS, Transformer
from shapely.geometry import Polygon, box
from shapely.ops import transform as shp_transform


def utm_crs(lon: float, lat: float) -> CRS:
    zone = int((lon + 180) // 6) + 1
    return CRS.from_epsg((32600 if lat >= 0 else 32700) + zone)


@dataclass(frozen=True)
class Grid:
    """A north-up raster grid: what every band, mask and DEM gets resampled onto."""

    crs: CRS
    transform: Affine
    width: int
    height: int

    @property
    def shape(self) -> tuple[int, int]:
        return (self.height, self.width)

    @property
    def res(self) -> float:
        return self.transform.a

    @property
    def bounds(self) -> tuple[float, float, float, float]:
        left, top = self.transform.c, self.transform.f
        return (left, top - self.height * self.res, left + self.width * self.res, top)

    def polygon(self) -> Polygon:
        return box(*self.bounds)

    def bounds_wgs84(self) -> tuple[float, float, float, float]:
        return to_wgs84(self.polygon(), self.crs).bounds

    def xy_to_rowcol(self, x: float, y: float) -> tuple[int, int]:
        col, row = ~self.transform @ (x, y)
        return int(row), int(col)

    def pixel_area_m2(self) -> float:
        return self.res * self.res

    def with_resolution(self, res: float) -> Grid:
        left, bottom, right, top = self.bounds
        w = round((right - left) / res)
        h = round((top - bottom) / res)
        return Grid(self.crs, Affine(res, 0, left, 0, -res, top), w, h)

    def buffered(self, margin_m: float) -> Grid:
        left, bottom, right, top = self.bounds
        left, bottom, right, top = left - margin_m, bottom - margin_m, right + margin_m, top + margin_m
        w = round((right - left) / self.res)
        h = round((top - bottom) / self.res)
        return Grid(self.crs, Affine(self.res, 0, left, 0, -self.res, top), w, h)


def grid_around(lon: float, lat: float, radius_m: float, res: float = 10.0) -> Grid:
    """Square grid centred on a point, in the local UTM zone, snapped to the resolution."""
    crs = utm_crs(lon, lat)
    x, y = Transformer.from_crs(4326, crs, always_xy=True).transform(lon, lat)
    left = np.floor((x - radius_m) / res) * res
    top = np.ceil((y + radius_m) / res) * res
    n = round(2 * radius_m / res)
    return Grid(crs, Affine(res, 0, left, 0, -res, top), n, n)


def to_wgs84(geom, crs: CRS):
    t = Transformer.from_crs(crs, 4326, always_xy=True)
    return shp_transform(t.transform, geom)


def from_wgs84(geom, crs: CRS):
    t = Transformer.from_crs(4326, crs, always_xy=True)
    return shp_transform(t.transform, geom)


def point_in_crs(lon: float, lat: float, crs: CRS) -> tuple[float, float]:
    return Transformer.from_crs(4326, crs, always_xy=True).transform(lon, lat)
