"""Sentinel-2 L2A scene discovery on Earth Search (AWS Open Data, us-west-2)."""

from __future__ import annotations

import logging
import time
from dataclasses import dataclass
from datetime import date

from pystac import Item
from pystac_client import Client
from shapely.geometry import Polygon, shape

log = logging.getLogger(__name__)

EARTH_SEARCH = "https://earth-search.aws.element84.com/v1"
COLLECTION = "sentinel-2-l2a"

# Earth Search asset keys for the bands we use.
BANDS = {
    "green": "green",  # B03, 10 m
    "nir": "nir",  # B08, 10 m
    "swir16": "swir16",  # B11, 20 m
    "scl": "scl",  # scene classification, 20 m
}
RGB = ("red", "green", "blue")


@dataclass(frozen=True)
class Scene:
    item_id: str
    day: date
    cloud_cover: float
    baseline: str
    sun_azimuth: float
    sun_elevation: float
    hrefs: dict[str, str]
    scale: dict[str, float]
    offset: dict[str, float]

    @classmethod
    def from_item(cls, item: Item) -> Scene:
        p = item.properties
        # Earth Search sets earthsearch:boa_offset_applied when it has already removed the
        # processing-baseline-04.00 BOA offset from the pixels, yet still advertises offset -0.1
        # in raster:bands. Applying it twice drives dark water negative, so honour the flag.
        offset_applied = bool(p.get("earthsearch:boa_offset_applied", False))
        hrefs, scale, offset = {}, {}, {}
        for key in (*BANDS.values(), *RGB):
            asset = item.assets.get(key)
            if asset is None:
                continue
            hrefs[key] = asset.href
            rb = (asset.extra_fields.get("raster:bands") or [{}])[0]
            scale[key] = float(rb.get("scale", 0.0001 if key != "scl" else 1))
            offset[key] = 0.0 if offset_applied else float(rb.get("offset", 0.0))
        return cls(
            item_id=item.id,
            day=item.datetime.date(),
            cloud_cover=float(p.get("eo:cloud_cover", 100)),
            baseline=str(p.get("s2:processing_baseline", "00.00")),
            sun_azimuth=float(p.get("view:sun_azimuth", 160.0)),
            sun_elevation=float(p.get("view:sun_elevation", 45.0)),
            hrefs=hrefs,
            scale=scale,
            offset=offset,
        )


def _client() -> Client:
    for attempt in range(5):
        try:
            return Client.open(EARTH_SEARCH)
        except Exception as e:  # noqa: BLE001 - network flakiness
            log.warning("STAC open failed (%s), retrying", e)
            time.sleep(2**attempt)
    raise RuntimeError("Earth Search unreachable")


def search(aoi_wgs84: Polygon, start: date, end: date, max_cloud: float = 80) -> list[Item]:
    for attempt in range(5):
        try:
            res = _client().search(
                collections=[COLLECTION],
                intersects=aoi_wgs84.__geo_interface__,
                datetime=f"{start.isoformat()}T00:00:00Z/{end.isoformat()}T23:59:59Z",
                query={"eo:cloud_cover": {"lt": max_cloud}},
                max_items=2000,
            )
            return list(res.items())
        except Exception as e:  # noqa: BLE001
            log.warning("STAC search failed (%s), retrying", e)
            time.sleep(2**attempt)
    raise RuntimeError("STAC search kept failing")


def one_per_day(items: list[Item], aoi_wgs84: Polygon) -> list[Scene]:
    """Keep one scene per acquisition day: a tile that fully covers the AOI,
    newest processing baseline first (Earth Search holds reprocessed duplicates), then least cloud."""
    by_day: dict[date, list[Item]] = {}
    for it in items:
        if not shape(it.geometry).contains(aoi_wgs84):
            continue
        by_day.setdefault(it.datetime.date(), []).append(it)
    scenes = []
    for day, group in sorted(by_day.items()):
        group.sort(
            key=lambda it: (
                -float(it.properties.get("s2:processing_baseline", "0") or 0),
                it.properties.get("eo:cloud_cover", 100),
            )
        )
        scenes.append(Scene.from_item(group[0]))
    return scenes


def find_scenes(aoi_wgs84: Polygon, start: date, end: date, max_cloud: float = 80) -> list[Scene]:
    return one_per_day(search(aoi_wgs84, start, end, max_cloud), aoi_wgs84)
