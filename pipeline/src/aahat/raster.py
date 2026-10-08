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
}


def read_to_grid(
    href: str,
    grid: Grid,
    resampling: Resampling = Resampling.bilinear,
    dtype: str = "float32",
    nodata: float | None = None,
    attempts: int = 4,
) -> np.ndarray:
    """Warp one band of a remote raster onto `grid`. Pixels outside the source are NaN (or 0 for ints)."""
    fill = np.nan if np.dtype(dtype).kind == "f" else 0
    last_err: Exception | None = None
    for attempt in range(attempts):
        try:
            with rasterio.Env(**GDAL_ENV), rasterio.open(href) as src:
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
            wait = 2**attempt
            log.warning("read failed (%s), retry %d in %ds: %s", href.rsplit("/", 1)[-1], attempt + 1, wait, e)
            time.sleep(wait)
    raise RuntimeError(f"could not read {href}") from last_err
