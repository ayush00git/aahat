"""Water mapping from Sentinel-2 L2A with cloud, snow and terrain-shadow handling.

Per scene, a pixel is *observed* only if it is clear (SCL not cloud, cloud shadow, cirrus,
snow/ice, saturated or nodata) and not in DEM-computed terrain shadow. An observed pixel is
*water* if both NDWI (green/NIR) and MNDWI (green/SWIR) are above threshold. MNDWI separates
turbid glacial water from bare rock; NDWI rejects snow and ice, whose MNDWI is also high.

Per season, the composite marks water where a pixel was water in at least `min_freq` of its
clear observations (and was observed at least `min_obs` times). Steep pixels are never water.
"""

from __future__ import annotations

import logging
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, field
from datetime import date

import numpy as np
from rasterio import features
from rasterio.enums import Resampling
from scipy import ndimage
from shapely.geometry import shape
from shapely.ops import unary_union

from .dem import Terrain
from .geo import Grid
from .raster import read_to_grid
from .stac import Scene

log = logging.getLogger(__name__)

# Sentinel-2 Scene Classification Layer codes that make a pixel unusable.
SCL_UNUSABLE = (0, 1, 3, 8, 9, 10, 11)  # nodata, saturated, cloud shadow, cloud med/high, cirrus, snow


@dataclass(frozen=True)
class WaterParams:
    ndwi_min: float = 0.10
    mndwi_min: float = 0.20
    max_slope_deg: float = 25.0
    min_obs: int = 2
    min_freq: float = 0.5


@dataclass
class SceneObs:
    day: date
    item_id: str
    observed: np.ndarray  # bool
    water: np.ndarray  # bool
    clear_fraction: float


@dataclass
class Composite:
    grid: Grid
    water: np.ndarray
    obs_count: np.ndarray
    water_freq: np.ndarray
    scenes: list[SceneObs] = field(default_factory=list)
    inputs: list[Scene] = field(default_factory=list)


def _reflectance(scene: Scene, key: str, grid: Grid, resampling=Resampling.bilinear) -> np.ndarray:
    dn = read_to_grid(scene.hrefs[key], grid, resampling, nodata=0)
    return dn * scene.scale[key] + scene.offset[key]


def observe(scene: Scene, grid: Grid, terrain: Terrain, p: WaterParams) -> SceneObs:
    scl = read_to_grid(scene.hrefs["scl"], grid, Resampling.nearest, dtype="uint8", nodata=0)
    clear = ~np.isin(scl, SCL_UNUSABLE)
    if clear.mean() < 0.05:  # skip the band reads for a fully cloudy/snowy AOI
        return SceneObs(scene.day, scene.item_id, clear, np.zeros_like(clear), float(clear.mean()))
    green = _reflectance(scene, "green", grid)
    nir = _reflectance(scene, "nir", grid)
    swir = _reflectance(scene, "swir16", grid)
    with np.errstate(divide="ignore", invalid="ignore"):
        ndwi = (green - nir) / (green + nir)
        mndwi = (green - swir) / (green + swir)
    shadow = terrain.shadow(scene.sun_azimuth, scene.sun_elevation)
    observed = clear & ~shadow & np.isfinite(ndwi) & np.isfinite(mndwi)
    water = observed & (ndwi > p.ndwi_min) & (mndwi > p.mndwi_min)
    return SceneObs(scene.day, scene.item_id, observed, water, float(observed.mean()))


def composite(
    scenes: list[Scene], grid: Grid, terrain: Terrain, p: WaterParams = WaterParams(), workers: int = 6
) -> Composite:
    with ThreadPoolExecutor(workers) as pool:
        obs = list(pool.map(lambda s: observe(s, grid, terrain, p), scenes))
    obs_count = np.zeros(grid.shape, dtype="int16")
    water_count = np.zeros(grid.shape, dtype="int16")
    for o in obs:
        obs_count += o.observed
        water_count += o.water
    with np.errstate(divide="ignore", invalid="ignore"):
        freq = np.where(obs_count > 0, water_count / obs_count, np.nan).astype("float32")
    water = (obs_count >= p.min_obs) & (freq >= p.min_freq) & (terrain.slope <= p.max_slope_deg)
    return Composite(grid, water, obs_count, freq, obs, list(scenes))


@dataclass
class LakeExtent:
    mask: np.ndarray
    area_m2: float
    perimeter_m: float
    uncertainty_m2: float  # +/- half a pixel along the shoreline
    polygon: object  # shapely geometry in grid CRS
    coverage: float  # fraction of the lake (plus a 2 px rim) observed >= min_obs times


def extract_lake(comp: Composite, seed_xy: tuple[float, float], p: WaterParams, search_m: float = 300.0) -> LakeExtent | None:
    """The water body at the seed point: the largest connected water component within `search_m`."""
    grid = comp.grid
    labels, n = ndimage.label(comp.water, structure=np.ones((3, 3)))
    if n == 0:
        return None
    r0, c0 = grid.xy_to_rowcol(*seed_xy)
    rad = int(search_m / grid.res)
    rr, cc = np.ogrid[: grid.height, : grid.width]
    disk = (rr - r0) ** 2 + (cc - c0) ** 2 <= rad**2
    near = np.unique(labels[disk & (labels > 0)])
    if near.size == 0:
        return None
    sizes = ndimage.sum_labels(np.ones_like(labels), labels, near)
    lake = labels == near[int(np.argmax(sizes))]
    lake = ndimage.binary_fill_holes(lake)  # icebergs and floating ice belong to the lake
    polys = [shape(g) for g, v in features.shapes(lake.astype("uint8"), mask=lake, transform=grid.transform) if v == 1]
    poly = unary_union(polys)
    area = float(lake.sum()) * grid.pixel_area_m2()
    perim = float(poly.length)
    rim = ndimage.binary_dilation(lake, iterations=2)
    coverage = float((comp.obs_count[rim] >= p.min_obs).mean())
    return LakeExtent(lake, area, perim, perim * grid.res / 2, poly, coverage)
