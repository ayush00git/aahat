"""Glacier outlines from the Randolph Glacier Inventory (RGI 7.0), via the public GLIMS WFS.

RGI 7.0 outlines date from around 2000 (the `src_date` of each outline), i.e. before every season
we score, so using them adds no hindsight. Glaciers have retreated since, so lake-glacier contact
can be overstated where a glacier has pulled back from its lake; debris-covered tongues, which
Sentinel-2 sees as rock, are included.
https://doi.org/10.5067/F6JMOVY5NAVZ
"""

from __future__ import annotations

import hashlib
import json
import logging
import time
import urllib.parse
import urllib.request
from dataclasses import dataclass

from shapely.geometry import shape

from .cache import cache_dir
from .geo import from_wgs84, utm_crs

log = logging.getLogger(__name__)

GLIMS_WFS = "https://www.glims.org/geoserver/GLIMS/ows"
# Himachal and its cross-border catchments: South Asia West, plus Central Asia (Tibetan plateau).
RGI_LAYERS = (
    "GLIMS:RGI2000-v7.0-G-14_south_asia_west_epsg3857",
    "GLIMS:RGI2000-v7.0-G-13_central_asia_epsg3857",
)
RGI_DOI = "https://doi.org/10.5067/F6JMOVY5NAVZ"


def rgi_features(bounds: tuple[float, float, float, float]) -> list[dict]:
    """RGI 7.0 glacier features (GeoJSON, WGS84) intersecting a lon/lat box, cached on disk."""
    key = hashlib.sha1(json.dumps([RGI_LAYERS, [round(b, 4) for b in bounds]]).encode()).hexdigest()[:16]
    path = cache_dir() / "rgi" / f"{key}.json"
    if path.exists():
        return json.loads(path.read_text())
    features: list[dict] = []
    for layer in RGI_LAYERS:
        q = urllib.parse.urlencode(
            {
                "service": "WFS",
                "version": "1.0.0",
                "request": "GetFeature",
                "typeName": layer,
                "outputFormat": "application/json",
                "srsName": "EPSG:4326",
                "bbox": ",".join(f"{b:.5f}" for b in bounds) + ",EPSG:4326",
                "maxFeatures": "2000",
            }
        )
        for attempt in range(6):
            try:
                with urllib.request.urlopen(f"{GLIMS_WFS}?{q}", timeout=90) as r:
                    features += json.load(r)["features"]
                break
            except Exception as e:  # noqa: BLE001 - flaky network
                log.warning("RGI query failed (%s), retry %d", e, attempt + 1)
                time.sleep(min(2**attempt, 20))
        else:
            raise RuntimeError(f"could not fetch RGI layer {layer}")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(features))
    return features


@dataclass
class GlacierProximity:
    distance_m: float | None  # None: no glacier within the search radius
    rgi_id: str | None
    glacier_area_km2: float | None
    outline_date: str | None


def nearest_glacier(lake_wgs84, search_m: float = 3000.0) -> GlacierProximity:
    """Distance from a lake outline to the nearest RGI glacier outline (0 if they touch or overlap)."""
    c = lake_wgs84.centroid
    pad = search_m / 111_000 * 1.5
    minx, miny, maxx, maxy = lake_wgs84.bounds
    feats = rgi_features((minx - pad, miny - pad, maxx + pad, maxy + pad))
    crs = utm_crs(c.x, c.y)
    lake = from_wgs84(lake_wgs84, crs)
    best = GlacierProximity(None, None, None, None)
    for f in feats:
        g = from_wgs84(shape(f["geometry"]).buffer(0), crs)
        d = lake.distance(g)
        if d <= search_m and (best.distance_m is None or d < best.distance_m):
            p = f["properties"]
            best = GlacierProximity(
                round(d, 1), p.get("rgi_id"), round(float(p.get("area_km2") or 0), 3), str(p.get("src_date", ""))[:10]
            )
    return best


def glaciers_geojson(lake_wgs84, search_m: float = 3000.0) -> dict:
    """Glacier outlines around a lake, for maps."""
    pad = search_m / 111_000 * 1.5
    minx, miny, maxx, maxy = lake_wgs84.bounds
    feats = rgi_features((minx - pad, miny - pad, maxx + pad, maxy + pad))
    area = lake_wgs84.buffer(pad)
    keep = [f for f in feats if shape(f["geometry"]).buffer(0).intersects(area)]
    return {
        "type": "FeatureCollection",
        "features": [
            {"type": "Feature", "geometry": f["geometry"], "properties": {"rgi_id": f["properties"].get("rgi_id")}}
            for f in keep
        ],
    }
