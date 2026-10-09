"""Water mapping from Sentinel-2 L2A with cloud, snow and terrain-shadow handling.

Per scene, a pixel is *observed* only if it is clear (SCL not cloud, cloud shadow, cirrus,
snow/ice, saturated or nodata) and not in DEM-computed terrain shadow. An observed pixel is
*water* if NDWI (green/NIR, both native 10 m) is above threshold. Snow, ice, rock and
vegetation all sit well below it. MNDWI (green/SWIR) is available as an extra guard but off by
default: on clear, dark lakes (Chandra Tal) green is so low that MNDWI drops below 0.3 even
mid-lake, and the 20 m SWIR band smears bright shore into 10 m edge pixels.

Per season, the composite marks water where a pixel was water in at least `min_freq` of its
clear observations. Steep pixels are never water. The share of a lake observed at least
`min_obs` times is reported as its coverage, and low coverage flags the year as partial.
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
    # Calibrated on five lakes (turbid proglacial Gepang Gath/Samudra Tapu, clear Chandra Tal,
    # Kya Tso, Lam Dal): lake NDWI 0.6-1.0; bare land NDWI 99th pct 0.05; glacier ice mostly < 0.1.
    # NDWI > 0.3 alone matched the dark-water footprint of Chandra Tal (0.447 vs 0.465 km2) where
    # adding MNDWI > 0.3 kept only 0.012 km2, and changed Gepang Gath by under 5%.
    ndwi_min: float = 0.30
    mndwi_min: float | None = None
    max_slope_deg: float = 25.0
    min_obs: int = 2
    min_freq: float = 0.5
    close_px: int = 2  # bridge brash ice and iceberg strings up to ~2*close_px pixels wide
    bridge_px: int = 10  # connect lake parts across never-seen pixels (cloud on the lake) up to this far from water


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
    min_obs: int = WaterParams.min_obs  # observations required per pixel (fewer if the season had fewer scenes)


def _reflectance(scene: Scene, key: str, grid: Grid, resampling=Resampling.bilinear) -> np.ndarray:
    dn = read_to_grid(scene.hrefs[key], grid, resampling, nodata=0)
    return dn * scene.scale[key] + scene.offset[key]


def read_clear(scene: Scene, grid: Grid) -> np.ndarray:
    """Clear-sky, snow-free mask from the cheap 20 m SCL band."""
    scl = read_to_grid(scene.hrefs["scl"], grid, Resampling.nearest, dtype="uint8", nodata=0)
    return ~np.isin(scl, SCL_UNUSABLE)


def observe(scene: Scene, clear: np.ndarray, grid: Grid, terrain: Terrain, p: WaterParams) -> SceneObs:
    keys = ("green", "nir", "swir16") if p.mndwi_min is not None else ("green", "nir")
    with ThreadPoolExecutor(len(keys)) as pool:  # separate HTTP connections; throughput is often per-connection
        bands = dict(zip(keys, pool.map(lambda k: _reflectance(scene, k, grid), keys)))
    green, nir = bands["green"], bands["nir"]
    with np.errstate(divide="ignore", invalid="ignore"):
        ndwi = (green - nir) / (green + nir)
    shadow = terrain.shadow(scene.sun_azimuth, scene.sun_elevation)
    observed = clear & ~shadow & np.isfinite(ndwi)
    water = observed & (ndwi > p.ndwi_min)
    if p.mndwi_min is not None:
        with np.errstate(divide="ignore", invalid="ignore"):
            mndwi = (green - bands["swir16"]) / (green + bands["swir16"])
        water &= mndwi > p.mndwi_min
    return SceneObs(scene.day, scene.item_id, observed, water, float(observed.mean()))


def composite(
    scenes: list[Scene],
    grid: Grid,
    terrain: Terrain,
    p: WaterParams = WaterParams(),
    max_scenes: int = 12,
    min_clear: float = 0.2,
    focus: np.ndarray | None = None,
    workers: int = 6,
) -> Composite:
    """Rank candidate scenes by how clear the `focus` area (default: whole grid) actually is (SCL),
    then map water on the best few. A scene with clouds on the hills but a clear lake is useful."""
    with ThreadPoolExecutor(workers) as pool:
        clears = list(pool.map(lambda s: read_clear(s, grid), scenes))
    frac = [float(c[focus].mean() if focus is not None else c.mean()) for c in clears]
    ranked = sorted(zip(scenes, clears, frac), key=lambda scf: -scf[2])
    chosen = [(s, c) for s, c, f in ranked if f >= min_clear][:max_scenes]
    chosen.sort(key=lambda sc: sc[0].day)
    log.info("%d/%d scenes clear enough; using %d", sum(f >= min_clear for f in frac), len(scenes), len(chosen))
    with ThreadPoolExecutor(workers) as pool:
        obs = list(pool.map(lambda sc: observe(sc[0], sc[1], grid, terrain, p), chosen))
    obs_count = np.zeros(grid.shape, dtype="int16")
    water_count = np.zeros(grid.shape, dtype="int16")
    for o in obs:
        obs_count += o.observed
        water_count += o.water
    with np.errstate(divide="ignore", invalid="ignore"):
        freq = np.where(obs_count > 0, water_count / obs_count, np.nan).astype("float32")
    # Map from every clearly seen pixel (majority vote over its looks), so a lake half-hidden in all
    # but one scene is not truncated. `min_obs` (lowered when the season had fewer scenes) is the
    # confidence bar: the share of the lake seen that often becomes its `coverage`.
    min_obs = max(1, min(p.min_obs, len(chosen)))
    water = (obs_count >= 1) & (freq >= p.min_freq) & (terrain.slope <= p.max_slope_deg)
    return Composite(grid, water, obs_count, freq, obs, [s for s, _ in chosen], min_obs)


@dataclass
class LakeExtent:
    mask: np.ndarray
    area_m2: float
    perimeter_m: float
    uncertainty_m2: float  # +/- half a pixel along the shoreline
    polygon: object  # shapely geometry in grid CRS
    coverage: float  # fraction of the lake (plus a 2 px rim) observed at least comp.min_obs times


def extract_lake(comp: Composite, seed_xy: tuple[float, float], p: WaterParams, search_m: float = 300.0) -> LakeExtent | None:
    """The water body at the seed point: the largest connected water component within `search_m`.

    Parts of a lake split by a cloud stay one lake: connectivity may pass through pixels that were
    never clearly seen (within `bridge_px` of water), but never through pixels seen to be dry.
    """
    grid = comp.grid
    unseen = comp.obs_count == 0
    bridge = np.zeros_like(unseen)
    if p.bridge_px:
        bridge = unseen & ndimage.binary_dilation(comp.water, iterations=p.bridge_px)
    labels, n = ndimage.label(comp.water | bridge, structure=np.ones((3, 3)))
    if n == 0:
        return None
    r0, c0 = grid.xy_to_rowcol(*seed_xy)
    rad = int(search_m / grid.res)
    rr, cc = np.ogrid[: grid.height, : grid.width]
    disk = (rr - r0) ** 2 + (cc - c0) ** 2 <= rad**2
    near = np.unique(labels[disk & comp.water & (labels > 0)])
    if near.size == 0:
        return None
    water_px = ndimage.sum_labels(comp.water, labels, near)
    lake = (labels == near[int(np.argmax(water_px))]) & comp.water
    # Icebergs and brash ice at a calving front belong to the lake: close narrow gaps, fill holes.
    if p.close_px:
        pad = p.close_px + 1
        closed = ndimage.binary_closing(np.pad(lake, pad), ndimage.generate_binary_structure(2, 1), p.close_px)
        lake = closed[pad:-pad, pad:-pad]
    lake = ndimage.binary_fill_holes(lake)
    polys = [shape(g) for g, v in features.shapes(lake.astype("uint8"), mask=lake, transform=grid.transform) if v == 1]
    poly = unary_union(polys)
    area = float(lake.sum()) * grid.pixel_area_m2()
    perim = float(poly.length)
    rim = ndimage.binary_dilation(lake, iterations=2)
    coverage = float((comp.obs_count[rim] >= comp.min_obs).mean())
    return LakeExtent(lake, area, perim, perim * grid.res / 2, poly, coverage)
