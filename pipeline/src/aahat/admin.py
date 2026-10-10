"""Districts and states from OpenStreetMap admin boundaries, so two "Hamirpur"s can be told apart.

In India boundary=administrative relations with admin_level 4 are states and union territories and
admin_level 5 are districts. They are assembled into polygons once from the same Geofabrik extract
as the exposure data and cached as GeoJSON; assigning a place is then a point-in-polygon lookup.

The extract is cut at its edge, so states that are only partly inside it (Uttarakhand, Uttar
Pradesh, Tibet) have broken rings and no polygon. Their districts that are complete still get a
state, from the state relation's `subarea` members or the district's own `is_in:state` tag.
"""

from __future__ import annotations

import json
import logging
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path

import numpy as np
import shapely
from shapely import STRtree
from shapely.geometry import shape
from shapely.geometry.base import BaseGeometry

from .cache import cache_dir, cached_file
from .osm_extract import EXTRACT_URL

log = logging.getLogger(__name__)

STATE, DISTRICT = 4, 5


@dataclass
class AdminArea:
    name: str
    name_hi: str | None
    admin_level: int  # 4 state / union territory, 5 district
    state: str | None  # for a district: the state it belongs to, if known
    geometry: BaseGeometry  # WGS84


def _admin_path(url: str) -> Path:
    return cache_dir() / "osm" / f"admin_{Path(url).stem}.json"


def _read_boundaries(pbf: Path) -> tuple[list[dict], dict[int, str]]:
    """(areas, district relation id -> state name) from the extract. Only admin_level 4/5 relations
    are assembled, so memory is the node-location index (about 0.7 GB for northern India) and little else."""
    import osmium

    levels = osmium.filter.TagFilter(("admin_level", str(STATE)), ("admin_level", str(DISTRICT)))
    wkb = osmium.geom.WKBFactory()
    areas: list[dict] = []
    state_of: dict[int, str] = {}
    for obj in osmium.FileProcessor(str(pbf)).with_areas(levels).with_filter(osmium.filter.KeyFilter("admin_level")):
        tags = dict(obj.tags)
        name = tags.get("name:en") or tags.get("name")
        if tags.get("boundary") != "administrative" or not name:
            continue
        if obj.is_relation() and tags["admin_level"] == str(STATE):
            # works even when the state's own ring is cut by the extract edge
            state_of.update({m.ref: name for m in obj.members if m.type == "r" and m.role == "subarea"})
        elif obj.is_area() and not obj.from_way() and tags["admin_level"] in (str(STATE), str(DISTRICT)):
            try:
                geom = shapely.from_wkb(wkb.create_multipolygon(obj))
            except RuntimeError:  # a ring broken at the extract edge
                continue
            areas.append(
                {
                    "osm_id": obj.orig_id(),
                    "name": name,
                    "name_hi": tags.get("name:hi"),
                    "admin_level": int(tags["admin_level"]),
                    "state": tags.get("is_in:state"),
                    "geometry": geom,
                }
            )
    return areas, state_of


def build_admin(url: str = EXTRACT_URL) -> Path:
    """Assemble state and district polygons from the extract once; cached as GeoJSON."""
    out = _admin_path(url)
    if out.exists():
        return out
    pbf = cached_file(url, workers=8)
    if pbf is None:
        raise RuntimeError(f"extract not found: {url}")
    areas, state_of = _read_boundaries(pbf)
    states = [a for a in areas if a["admin_level"] == STATE]
    districts = [a for a in areas if a["admin_level"] == DISTRICT]
    # A district's state: the state polygon it lies in, else what OSM says (subarea member, is_in:state).
    inside = containing([d["geometry"].representative_point() for d in districts], [s["geometry"] for s in states])
    for d, i in zip(districts, inside):
        d["state"] = states[i]["name"] if i >= 0 else state_of.get(d["osm_id"], d["state"])
    for s in states:
        s["state"] = None
    feats = [
        {
            "type": "Feature",
            # 1e-5 degrees is about 1 m: plenty for villages, and a third of the file size
            "geometry": json.loads(shapely.to_geojson(shapely.set_precision(a["geometry"], 1e-5))),
            "properties": {k: a[k] for k in ("osm_id", "name", "name_hi", "admin_level", "state")},
        }
        for a in areas
    ]
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps({"type": "FeatureCollection", "source": url, "features": feats}, ensure_ascii=False))
    log.info("admin areas from %s: %d states, %d districts", Path(url).name, len(states), len(districts))
    return out


@lru_cache(maxsize=2)
def load_admin(url: str = EXTRACT_URL) -> list[AdminArea]:
    feats = json.loads(build_admin(url).read_text())["features"]
    return [
        AdminArea(
            f["properties"]["name"],
            f["properties"].get("name_hi"),
            f["properties"]["admin_level"],
            f["properties"].get("state"),
            shape(f["geometry"]),
        )
        for f in feats
    ]


def containing(points: list[BaseGeometry], polygons: list[BaseGeometry]) -> list[int]:
    """For each point the index of the polygon it lies in, or -1. Where polygons overlap (an enclave
    mapped inside its neighbour) the smallest one wins."""
    best = [-1] * len(points)
    if not points or not polygons:
        return best
    sizes = [g.area for g in polygons]
    pi, gi = STRtree(polygons).query(np.array(points, dtype=object), predicate="within")
    for p, g in zip(pi.tolist(), gi.tolist()):
        if best[p] < 0 or sizes[g] < sizes[best[p]]:
            best[p] = g
    return best


def assign(lonlat: list[tuple[float, float]], areas: list[AdminArea]) -> list[tuple[str | None, str | None]]:
    """(district, state) for each lon/lat point; None where no boundary contains it (e.g. Tibet)."""
    points = list(shapely.points(np.array(lonlat, dtype=float).reshape(-1, 2)))
    districts = [a for a in areas if a.admin_level == DISTRICT]
    states = [a for a in areas if a.admin_level == STATE]
    in_district = containing(points, [a.geometry for a in districts])
    in_state = containing(points, [a.geometry for a in states])
    out = []
    for d, s in zip(in_district, in_state):
        district = districts[d] if d >= 0 else None
        state = states[s].name if s >= 0 else (district.state if district else None)
        out.append((district.name if district else None, state))
    return out
