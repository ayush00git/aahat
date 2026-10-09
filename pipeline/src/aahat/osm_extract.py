"""Exposure assets from a downloaded OpenStreetMap extract (Geofabrik), as a robust alternative to Overpass.

The public Overpass servers time out under load, which stalls a run for many minutes. A regional
extract is downloaded once, filtered once to the asset classes we track, and kept on disk; any box
query is then a quick in-memory filter, returning Overpass-shaped JSON so exposure.parse_overpass
handles both sources the same way. Relations are skipped (settlements, bridges, roads and plants are
almost always nodes or ways in this region).
"""

from __future__ import annotations

import json
import logging
from functools import lru_cache
from pathlib import Path

from .cache import cache_dir, cached_file
from .exposure import classify

log = logging.getLogger(__name__)

# Himachal Pradesh, Jammu & Kashmir, Ladakh, Punjab, Haryana, Delhi, Chandigarh, Uttarakhand, UP.
EXTRACT_URL = "https://download.geofabrik.de/asia/india/northern-zone-261008.osm.pbf"
TRACKED_KEYS = ("place", "highway", "man_made", "power", "waterway", "amenity")
# Coverage box of the extract (from its file header), so a fresh machine knows to download it.
EXTRACT_BOUNDS = (69.169911, 23.046881, 80.167546, 36.137884)


def _filtered_path(url: str) -> Path:
    return cache_dir() / "osm" / f"extract_{Path(url).stem}.json"


def build_filtered(url: str = EXTRACT_URL) -> Path:
    """Download the extract (resumable) and keep only elements classify() tracks, with geometry."""
    out = _filtered_path(url)
    if out.exists():
        return out
    import osmium

    pbf = cached_file(url, workers=8)
    if pbf is None:
        raise RuntimeError(f"extract not found: {url}")
    elements = []
    header_box = osmium.io.Reader(str(pbf), osmium.osm.osm_entity_bits.NOTHING).header().box()
    processor = osmium.FileProcessor(str(pbf)).with_locations().with_filter(osmium.filter.KeyFilter(*TRACKED_KEYS))
    for obj in processor:
        tags = dict(obj.tags)
        if classify(tags) is None:
            continue
        if obj.is_node():
            elements.append(
                {"type": "node", "id": obj.id, "lat": obj.location.lat, "lon": obj.location.lon, "tags": tags}
            )
        elif obj.is_way():
            try:
                geom = [{"lat": n.location.lat, "lon": n.location.lon} for n in obj.nodes]
            except osmium.InvalidLocationError:
                continue
            elements.append({"type": "way", "id": obj.id, "tags": tags, "geometry": geom})
    bounds = [
        header_box.bottom_left.lon,
        header_box.bottom_left.lat,
        header_box.top_right.lon,
        header_box.top_right.lat,
    ]
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps({"source": url, "bounds": bounds, "elements": elements}))
    log.info("filtered %s: %d tracked elements", Path(url).name, len(elements))
    return out


@lru_cache(maxsize=2)
def _load(path: str) -> dict:
    data = json.loads(Path(path).read_text())
    for el in data["elements"]:  # precompute each element's lon/lat box for quick queries
        if el["type"] == "node":
            el["_box"] = (el["lon"], el["lat"], el["lon"], el["lat"])
        else:
            lons = [p["lon"] for p in el["geometry"]]
            lats = [p["lat"] for p in el["geometry"]]
            el["_box"] = (min(lons), min(lats), max(lons), max(lats))
    return data


def covers(bounds: tuple[float, float, float, float], url: str = EXTRACT_URL) -> bool:
    """True if the extract's coverage box contains `bounds` (it is downloaded on first use)."""
    path = _filtered_path(url)
    b = _load(str(path))["bounds"] if path.exists() else (EXTRACT_BOUNDS if url == EXTRACT_URL else None)
    if b is None:
        return False
    return b[0] <= bounds[0] and b[1] <= bounds[1] and bounds[2] <= b[2] and bounds[3] <= b[3]


def query(bounds: tuple[float, float, float, float], url: str = EXTRACT_URL) -> dict:
    """Overpass-shaped JSON of tracked elements whose box intersects `bounds` (minlon, minlat, maxlon, maxlat)."""
    data = _load(str(build_filtered(url)))
    x0, y0, x1, y1 = bounds
    hits = [
        {k: v for k, v in el.items() if k != "_box"}
        for el in data["elements"]
        if el["_box"][0] <= x1 and el["_box"][2] >= x0 and el["_box"][1] <= y1 and el["_box"][3] >= y0
    ]
    return {"elements": hits, "source": data["source"]}
