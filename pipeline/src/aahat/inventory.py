"""Find glacial lakes worth monitoring: sweep high-altitude windows for large water bodies.

Each window is mapped like a monitored lake (water composite over the post-monsoon season, cloud,
snow and terrain shadow handled) but at 20 m from COG overviews, which is plenty for lakes of
0.1 km2 and more. Lakes above `min_elev_m` are kept, the same lake seen from overlapping windows is
listed once, and each gets the nearest named settlement for a human-readable label.
"""

from __future__ import annotations

import json
import logging
import math
from dataclasses import asdict, dataclass
from pathlib import Path

import numpy as np
from scipy import ndimage
from shapely.geometry import Point

from .dem import Terrain
from .geo import grid_around, to_wgs84
from .stac import find_scenes
from .timeseries import season
from .water import WaterParams, composite

log = logging.getLogger(__name__)

# 20 km windows over Himachal's glacial valleys (centre lat, lon) and the Tibetan inflow to the Sutlej.
WINDOWS: list[dict] = [
    {"name": "Baspa (Chitkul)", "lat": 31.32, "lon": 78.50},
    {"name": "Baspa (Sangla)", "lat": 31.30, "lon": 78.30},
    {"name": "Kinnaur (Kashang)", "lat": 31.60, "lon": 78.38},
    {"name": "Hangrang / Pooh", "lat": 31.95, "lon": 78.62},
    {"name": "Pin valley", "lat": 31.90, "lon": 78.05},
    {"name": "Upper Spiti (Losar)", "lat": 32.35, "lon": 77.85},
    {"name": "Pareechu (Tibet)", "lat": 32.40, "lon": 78.70},
    {"name": "Chandra (Bara Shigri)", "lat": 32.20, "lon": 77.70},
    {"name": "Parbati (Mantalai)", "lat": 31.85, "lon": 77.60},
    {"name": "Beas Kund / Hamta", "lat": 32.33, "lon": 77.20},
    {"name": "Miyar valley", "lat": 32.85, "lon": 76.85},
    {"name": "Baralacha / Suraj Tal", "lat": 32.75, "lon": 77.42},
    {"name": "Manimahesh (Ravi)", "lat": 32.40, "lon": 76.65},
]


@dataclass
class FoundLake:
    lat: float
    lon: float
    area_km2: float
    elev_m: float
    window: str
    near_place: str | None = None
    near_place_km: float | None = None


def find_lakes(
    lat: float,
    lon: float,
    radius_m: float,
    year: int,
    res: float = 20.0,
    max_scenes: int = 6,
    min_area_m2: float = 100_000,
    min_elev_m: float = 3500,
    window: str = "",
) -> list[FoundLake]:
    grid = grid_around(lon, lat, radius_m, res=res)
    terrain = Terrain(grid)
    start, end = season(year)
    scenes = find_scenes(to_wgs84(grid.polygon(), grid.crs), start, end)
    if not scenes:
        return []
    comp = composite(scenes, grid, terrain, WaterParams(), max_scenes=max_scenes)
    labels, n = ndimage.label(comp.water, structure=np.ones((3, 3)))
    if n == 0:
        return []
    idx = np.arange(1, n + 1)
    areas = ndimage.sum_labels(comp.water, labels, idx) * grid.pixel_area_m2()
    centers = ndimage.center_of_mass(comp.water, labels, idx)
    found = []
    for i, area, (r, c) in zip(idx, areas, centers):
        if area < min_area_m2:
            continue
        elev = float(np.nanmedian(terrain.dem[labels == i]))
        if not math.isfinite(elev) or elev < min_elev_m:
            continue
        x, y = grid.transform @ (c + 0.5, r + 0.5)
        pt = to_wgs84(Point(x, y), grid.crs)
        found.append(FoundLake(round(pt.y, 5), round(pt.x, 5), round(area / 1e6, 4), round(elev), window))
    return found


def _km(a_lat, a_lon, b_lat, b_lon) -> float:
    dy = (a_lat - b_lat) * 111.0
    dx = (a_lon - b_lon) * 111.0 * math.cos(math.radians((a_lat + b_lat) / 2))
    return math.hypot(dx, dy)


def dedupe(lakes: list[FoundLake], within_km: float = 0.5) -> list[FoundLake]:
    """One entry per lake when overlapping windows both saw it (keep the larger outline)."""
    kept: list[FoundLake] = []
    for lake in sorted(lakes, key=lambda x: -x.area_km2):
        if all(_km(lake.lat, lake.lon, k.lat, k.lon) > within_km for k in kept):
            kept.append(lake)
    return kept


def label_with_places(lakes: list[FoundLake], places_path: Path) -> None:
    """Attach the nearest named settlement from places/index.json (if built)."""
    if not places_path.exists():
        return
    places = json.loads(places_path.read_text())["places"]
    lats = np.array([p["lat"] for p in places])
    lons = np.array([p["lon"] for p in places])
    for lake in lakes:
        d = np.hypot((lats - lake.lat) * 111.0, (lons - lake.lon) * 111.0 * math.cos(math.radians(lake.lat)))
        i = int(np.argmin(d))
        lake.near_place, lake.near_place_km = places[i]["name"], round(float(d[i]), 1)


def sweep(windows: list[dict], year: int, out_dir: Path, min_area_m2: float = 50_000) -> list[FoundLake]:
    """Each 20 km window is mapped as four 10 km sub-windows: a 20 km box straddling two Sentinel-2 tiles
    lies fully inside neither, which would leave the window without any scene."""
    found: list[FoundLake] = []
    for w in windows:
        log.info("sweep: %s", w["name"])
        dlat, dlon = 5 / 111.0, 5 / (111.0 * math.cos(math.radians(w["lat"])))
        for sy in (-1, 1):
            for sx in (-1, 1):
                try:
                    found += find_lakes(
                        w["lat"] + sy * dlat,
                        w["lon"] + sx * dlon,
                        5_000,
                        year,
                        min_area_m2=min_area_m2,
                        window=w["name"],
                    )
                except Exception as e:  # noqa: BLE001 - one bad window should not stop the sweep
                    log.warning("sweep window %s failed: %s", w["name"], e)
    lakes = dedupe(found)
    label_with_places(lakes, out_dir / "places" / "index.json")
    path = out_dir / "inventory" / f"candidates_{year}.json"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps([asdict(x) for x in lakes], indent=1, ensure_ascii=False))
    return lakes
