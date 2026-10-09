"""Read remote COGs straight onto a Grid (windowed HTTP range reads, no bulk downloads)."""

from __future__ import annotations

import logging
import time

import numpy as np
import rasterio
from rasterio.enums import Resampling
from rasterio.vrt import WarpedVRT

from .geo import Grid

log = logging.getLogger(__name__)

# GDAL settings for reading public COGs over HTTPS from AWS Open Data.
GDAL_ENV = {
    "AWS_NO_SIGN_REQUEST": "YES",
    "GDAL_DISABLE_READDIR_ON_OPEN": "EMPTY_DIR",
    "CPL_VSIL_CURL_ALLOWED_EXTENSIONS": ".tif,.TIF,.tiff",
    "GDAL_HTTP_MAX_RETRY": "6",
    "GDAL_HTTP_RETRY_DELAY": "1",
    "GDAL_HTTP_MULTIRANGE": "YES",
    "GDAL_HTTP_MERGE_CONSECUTIVE_RANGES": "YES",
    "VSI_CACHE": "TRUE",
    # Without these a stalled connection hangs forever; fail fast and let the retry loop take over.
    "GDAL_HTTP_CONNECTTIMEOUT": "20",
    "GDAL_HTTP_TIMEOUT": "120",
    "GDAL_HTTP_LOW_SPEED_TIME": "30",
    "GDAL_HTTP_LOW_SPEED_LIMIT": "2000",
}


def read_to_grid(
    href: str,
    grid: Grid,
    resampling: Resampling = Resampling.bilinear,
    dtype: str = "float32",
    nodata: float | None = None,
    attempts: int = 7,
) -> np.ndarray:
    """Warp one band of a raster (remote URL or local path) onto `grid`.

    Pixels outside the source are NaN (or 0 for ints). When the grid is at least twice as coarse as
    the source, the coarsest overview no finer than the grid is read instead: far fewer bytes.
    """
    fill = np.nan if np.dtype(dtype).kind == "f" else 0
    last_err: Exception | None = None
    for attempt in range(attempts):
        # GDAL caches byte ranges per URL, including a truncated block from a dropped connection.
        # A throwaway query parameter (ignored by S3) makes each retry a fresh, uncached read.
        url = href if attempt == 0 or "://" not in href else f"{href}{'&' if '?' in href else '?'}retry={attempt}"
        try:
            with rasterio.Env(**GDAL_ENV):
                level = _overview_level(url, grid.res)
                src_ctx = rasterio.open(url, overview_level=level) if level is not None else rasterio.open(url)
            with rasterio.Env(**GDAL_ENV), src_ctx as src:
                src_nodata = src.nodata if nodata is None else nodata
                with WarpedVRT(
                    src,
                    crs=grid.crs,
                    transform=grid.transform,
                    width=grid.width,
                    height=grid.height,
                    resampling=resampling,
                    src_nodata=src_nodata,
                    nodata=src_nodata,
                ) as vrt:
                    arr = vrt.read(1, masked=True)
            out = arr.astype(dtype).filled(fill)
            return out
        except rasterio.errors.RasterioIOError as e:  # flaky network: back off and retry
            last_err = e
            wait = min(2**attempt, 30)
            log.warning("read failed (%s), retry %d in %ds: %s", href.rsplit("/", 1)[-1], attempt + 1, wait, e)
            time.sleep(wait)
    raise RuntimeError(f"could not read {href}") from last_err


def _overview_level(url: str, target_res: float) -> int | None:
    """Index of the coarsest COG overview whose pixel size is still <= target_res, or None."""
    with rasterio.open(url) as src:
        native = abs(src.res[0])
        if src.crs is None or not src.crs.is_projected or target_res < 2 * native:
            return None
        factors = src.overviews(1)
    level = None
    for i, f in enumerate(factors):
        if native * f <= target_res + 1e-6:
            level = i
    return level
