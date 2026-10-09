"""What a glacial lake outburst flood could hit: settlements, bridges, roads, hydro plants, schools, clinics.

From OpenStreetMap via the Overpass API, for a lon/lat box. Raw Overpass JSON is cached on disk keyed
by the query text, so a repeat run (or a demo without network) reads the same snapshot. OSM coverage
in the Himalaya is uneven: absence of an asset here is not evidence that nothing is there.
"""

from __future__ import annotations

import hashlib
import json
import logging
import re
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field

from shapely.geometry import LineString, MultiPoint, Point, Polygon, mapping
from shapely.geometry.base import BaseGeometry
from shapely.ops import polygonize, unary_union

from .cache import cache_dir

log = logging.getLogger(__name__)

OVERPASS_ENDPOINTS = (
    "https://overpass-api.de/api/interpreter",
    "https://maps.mail.ru/osm/tools/overpass/api/interpreter",
    "https://overpass.kumi.systems/api/interpreter",
    "https://overpass.private.coffee/api/interpreter",
)
USER_AGENT = "aahat-glof-watch/0.1"

PLACES = ("city", "town", "village", "hamlet", "suburb")
ROADS = ("motorway", "trunk", "primary", "secondary", "tertiary")
FOOT_WAYS = ("footway", "path", "track", "steps")  # a bridge on these carries people, not trucks
AMENITIES = {"school": "school", "college": "school", "hospital": "health", "clinic": "health"}
KEEP_TAGS = (
    "name",
    "name:hi",
    "name:en",
    "place",
    "population",
    "highway",
    "bridge",
    "power",
    "plant:source",
    "generator:source",
    "plant:output:electricity",
    "waterway",
    "amenity",
    "operator",
    "ref",
    "wikidata",
)
# Many Himalayan plants are mapped with a name like "Seli HEP" but no plant:source. "HEP" must be a
# whole word, so names that merely contain the letters (e.g. "Shepherd") are not taken for hydro.
HYDRO_NAME = re.compile(r"hydro|\bh\.?e\.?p\b", re.IGNORECASE)


@dataclass
class Asset:
    osm_type: str  # "node" | "way" | "relation"
    osm_id: int
    kind: str  # settlement | bridge | road | hydro | school | health
    subkind: str  # e.g. village, road_bridge, primary, hydro_plant, dam, clinic
    name: str | None
    name_hi: str | None
    geometry: BaseGeometry  # WGS84
    tags: dict = field(default_factory=dict)

    @property
    def osm(self) -> str:
        return f"{self.osm_type}/{self.osm_id}"


def overpass_selectors(bounds: tuple[float, float, float, float]) -> list[str]:
    """One Overpass statement per asset class, in a lon/lat box (minlon, minlat, maxlon, maxlat)."""
    minlon, minlat, maxlon, maxlat = bounds
    bb = f"{minlat:.5f},{minlon:.5f},{maxlat:.5f},{maxlon:.5f}"  # Overpass wants south,west,north,east
    places, roads = "|".join(PLACES), "|".join(ROADS)
    amenities = "|".join(AMENITIES)
    return [
        f'nwr["place"~"^({places})$"]({bb});',
        f'way["bridge"]["bridge"!="no"]["highway"]({bb});nwr["man_made"="bridge"]({bb});',
        f'way["highway"~"^({roads})$"]({bb});',
        (
            f'nwr["power"="plant"]({bb});nwr["power"="generator"]["generator:source"="hydro"]({bb});'
            f'nwr["waterway"~"^(dam|weir)$"]({bb});'
        ),
        f'nwr["amenity"~"^({amenities})$"]({bb});',
    ]


def overpass_query(bounds: tuple[float, float, float, float]) -> str:
    """The whole request as one text (used as the cache key); sent as one small query per selector."""
    return "\n".join(overpass_selectors(bounds))


def _wrap(selector: str) -> str:
    return f"[out:json][timeout:90];({selector});out geom;"


def query_key(query: str) -> str:
    """Cache key for a query: the same text always maps to the same snapshot on disk."""
    return hashlib.sha1(query.encode()).hexdigest()[:16]


def _post_overpass(query: str, attempts: int = 3) -> dict:
    body = urllib.parse.urlencode({"data": query}).encode()
    err: Exception | None = None
    for attempt in range(attempts):
        for url in OVERPASS_ENDPOINTS:
            try:
                req = urllib.request.Request(url, data=body, headers={"User-Agent": USER_AGENT})
                with urllib.request.urlopen(req, timeout=150) as r:
                    data = json.load(r)
                # A server-side timeout still returns 200, with partial results and a remark: never cache that.
                remark = data.get("remark") or ""
                if "error" in remark.lower():
                    raise RuntimeError(f"Overpass: {remark.strip()}")
                return data
            except (urllib.error.URLError, TimeoutError, OSError, ValueError, RuntimeError) as e:
                err = e
                log.warning("Overpass %s failed (%s), attempt %d", url, e, attempt + 1)
        time.sleep(min(5 * 2**attempt, 60))  # every mirror failed: they are probably busy, back off
    raise RuntimeError("all Overpass endpoints failed") from err


def fetch_assets(bounds: tuple[float, float, float, float]) -> list[Asset]:
    """Exposed assets from OpenStreetMap in a lon/lat box: from the local regional extract if it covers
    the box (see osm_extract), otherwise from Overpass, cached on disk."""
    from . import osm_extract  # local extract first: no network, no server timeouts

    if osm_extract.covers(bounds):
        return parse_overpass(osm_extract.query(bounds))
    query = overpass_query(bounds)
    path = cache_dir() / "osm" / f"{query_key(query)}.json"
    if path.exists():
        return parse_overpass(json.loads(path.read_text()))
    # Public Overpass servers time out on big requests when busy, so send one small query per asset class.
    data = {"elements": []}
    for selector in overpass_selectors(bounds):
        data["elements"] += _post_overpass(_wrap(selector))["elements"]
        time.sleep(1)  # be polite: servers rate-limit bursts (HTTP 429/504)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data))
    return parse_overpass(data)


def classify(tags: dict) -> tuple[str, str] | None:
    """(kind, subkind) for an OSM tag set, or None if it is not an asset we track.

    Order matters: a highway that is a bridge is a bridge, not also a road, because the bridge is the
    part a flood takes out.
    """
    if tags.get("place") in PLACES:
        return "settlement", tags["place"]
    power = tags.get("power")
    if power == "plant" and (tags.get("plant:source") == "hydro" or HYDRO_NAME.search(tags.get("name", ""))):
        return "hydro", "hydro_plant"
    if power == "generator" and tags.get("generator:source") == "hydro":
        return "hydro", "hydro_plant"
    if tags.get("waterway") in ("dam", "weir"):
        return "hydro", tags["waterway"]
    highway = tags.get("highway")
    if highway and tags.get("bridge", "no") != "no":
        return "bridge", "footbridge" if highway in FOOT_WAYS else "road_bridge"
    if tags.get("man_made") == "bridge":
        return "bridge", "road_bridge"
    if tags.get("amenity") in AMENITIES:
        return AMENITIES[tags["amenity"]], tags["amenity"]
    if highway in ROADS:
        return "road", highway
    return None


def _coords(points: list | None) -> list[tuple[float, float]]:
    return [(p["lon"], p["lat"]) for p in points or [] if p]


def _is_area(tags: dict) -> bool:
    """A closed way is an area unless it is a linear feature drawn as a loop (e.g. a roundabout)."""
    if tags.get("area") == "no":
        return False
    if "highway" in tags or "barrier" in tags:
        return tags.get("area") == "yes"
    return True


def _way_geometry(el: dict) -> BaseGeometry | None:
    pts = _coords(el.get("geometry"))
    if len(pts) < 2:
        return None
    if len(pts) >= 4 and pts[0] == pts[-1] and _is_area(el.get("tags", {})):
        poly = Polygon(pts)
        return poly if poly.is_valid else poly.buffer(0)
    return LineString(pts)


def _relation_geometry(el: dict) -> BaseGeometry | None:
    """Polygon(s) from the outer rings; failing that, a centroid of whatever members have geometry."""
    lines, points = [], []
    for m in el.get("members", []):
        if m.get("type") == "way":
            pts = _coords(m.get("geometry"))
            if len(pts) >= 2 and m.get("role") in ("outer", ""):
                lines.append(LineString(pts))
        elif m.get("type") == "node" and "lat" in m:
            points.append(Point(m["lon"], m["lat"]))
    if lines:
        polys = list(polygonize(unary_union(lines)))  # outer rings are often split over several ways
        if polys:
            return unary_union(polys)
        return unary_union(lines).centroid
    if points:
        return MultiPoint(points).centroid
    return None


def _geometry(el: dict) -> BaseGeometry | None:
    t = el.get("type")
    if t == "node":
        return Point(el["lon"], el["lat"]) if "lat" in el and "lon" in el else None
    if t == "way":
        return _way_geometry(el)
    if t == "relation":
        return _relation_geometry(el)
    return None


def parse_overpass(data: dict) -> list[Asset]:
    """Assets from Overpass JSON (`out geom`). Elements we do not track or cannot place are skipped."""
    assets: list[Asset] = []
    seen: set[tuple[str, int]] = set()
    for el in data.get("elements", []):
        key = (el.get("type"), el.get("id"))
        tags = el.get("tags") or {}
        if key in seen or (cls := classify(tags)) is None:
            continue
        geom = _geometry(el)
        if geom is None or geom.is_empty:
            continue
        seen.add(key)
        assets.append(
            Asset(
                osm_type=el["type"],
                osm_id=int(el["id"]),
                kind=cls[0],
                subkind=cls[1],
                name=tags.get("name"),
                name_hi=tags.get("name:hi"),
                geometry=geom,
                tags={k: tags[k] for k in KEEP_TAGS if k in tags},
            )
        )
    return assets


def assets_geojson(assets: list[Asset]) -> dict:
    """Assets as a GeoJSON FeatureCollection, for maps."""
    return {
        "type": "FeatureCollection",
        "features": [
            {
                "type": "Feature",
                "geometry": mapping(a.geometry),
                "properties": {
                    "kind": a.kind,
                    "subkind": a.subkind,
                    "name": a.name,
                    "name_hi": a.name_hi,
                    "osm": a.osm,
                },
            }
            for a in assets
        ],
    }
