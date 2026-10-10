"""Which settlements, bridges, roads and hydro projects a lake's flood could reach, and when.

Each OpenStreetMap asset is placed on the flood path: its distance downstream (projection onto
the path), its ground height relative to the screening flood level at that point, and whether the
flood in the inundation corridor reaches ground that low (one flood level, see assess). Results are ordered by arrival time, nearest first: the order in
which alerts fan out. All of it inherits the screening caveats of downstream.py, and OSM coverage
in the Himalaya is uneven, so a place missing from OSM is missing here.
"""

from __future__ import annotations

import logging
import math
from dataclasses import asdict, dataclass

import numpy as np
import shapely
from pyproj import CRS
from shapely.geometry import LineString
from shapely.prepared import prep

from .downstream import Downstream, level_along, sample
from .exposure import Asset, fetch_assets
from .geo import from_wgs84

log = logging.getLogger(__name__)

# A settlement point within this height above the flood level and this distance of the corridor is
# flagged "margin": OSM puts a village at one point, and houses spread above and below it.
MARGIN_HEIGHT_M = 10.0
MARGIN_DISTANCE_M = 300.0


@dataclass
class Impact:
    name: str | None
    name_hi: str | None
    kind: str
    subkind: str
    osm: str
    lon: float
    lat: float
    km: float  # along the flood path from the lake's spill point
    lateral_m: float  # from the path (river) line
    # ground minus the flood level there (see assess): negative exactly when status is "flooded";
    # None where the ground is lower than the flood level but the flood does not reach it
    height_above_flood_m: float | None
    flood_depth_m: float  # at the nearest station, above the river bed
    arrival_min_expected: float
    arrival_min_fast: float
    status: str  # flooded | margin | outside
    road_km_flooded: float | None = None


def assess(d: Downstream, assets: list[Asset], max_lateral_m: float = 3000.0) -> list[Impact]:
    """Each asset's outcome in one scenario.

    One flood level applies at any point: the level interpolated between stations at the nearest
    point of the path (downstream.level_along, the level the corridor was drawn with). Status and
    height_above_flood_m both come from it, so they cannot contradict each other:

    - flooded: some part of the asset lies in the corridor (a point: within half a DEM cell of it)
      AND the ground there is below the flood level. height_above_flood_m is negative: the lowest
      such ground minus the level there.
    - margin (not roads): not flooded, within MARGIN_DISTANCE_M of the corridor and no more than
      MARGIN_HEIGHT_M above the flood level (which includes low ground the flood does not reach).
    - outside: everything else.

    For an asset that is not flooded, height_above_flood_m is zero or positive, or None where its
    ground is lower than the flood level but the connected flood does not reach it (behind a ridge,
    beyond the wetted reach): the level was solved for the channel and means nothing there.
    """
    crs = CRS.from_user_input(d.crs)
    path = LineString(d.path_xy)
    corridor = from_wgs84(d.corridor, crs)
    near_corridor = prep(corridor.buffer(MARGIN_DISTANCE_M))
    station_m = np.array([s.km for s in d.stations]) * 1000
    res = d.grid.res if d.grid is not None else 30.0

    def heights(xy: np.ndarray) -> np.ndarray:
        """Ground minus the flood level at each point, to 0.1 m (NaN off the DEM)."""
        z = sample(d.dem, d.grid, xy[:, 0], xy[:, 1])
        along = shapely.line_locate_point(path, shapely.points(xy))
        return np.round(z - level_along(d.stations, along), 1)

    out: list[Impact] = []
    for a in assets:
        g = from_wgs84(a.geometry, crs)
        if g.distance(path) > max_lateral_m:
            continue
        # where the asset meets the flood first: its point nearest the river
        ref = g if g.geom_type == "Point" else _nearest_point(g, path)
        along_m = path.project(ref)
        km = along_m / 1000
        if km > station_m[-1] / 1000 + 0.5:
            continue
        # depth in the river and arrival come from the nearest station (unchanged); the flood level
        # the asset's ground is compared with does not: see heights()
        if d.station_xy is not None:
            i = int(np.argmin(np.hypot(d.station_xy[:, 0] - ref.x, d.station_xy[:, 1] - ref.y)))
        else:
            i = int(np.argmin(np.abs(station_m - along_m)))
        st = d.stations[i]
        # The part of the asset the connected flood can reach. A settlement or school is one OSM
        # point, taken as reached within half a DEM cell of the corridor; lines and areas (bridges,
        # roads, plants) where they enter it. Ground merely lower than the flood level, behind a
        # ridge or on the far side of the plateau a lake sits on, is not reached.
        if g.geom_type == "Point":
            reached = g if corridor.distance(g) <= res / 2 else None
        else:
            reached = g.intersection(corridor)
        h_in = (
            heights(_sample_points(reached, res / 2)) if reached is not None and not reached.is_empty else np.array([])
        )
        h_in = h_in[np.isfinite(h_in)]
        if h_in.size:
            raw = float(h_in.min())  # the lowest ground the flood can reach
        else:
            h_ref = float(heights(np.array([[ref.x, ref.y]]))[0])
            raw = h_ref if np.isfinite(h_ref) else None
        flooded = bool(h_in.size) and raw < 0
        if flooded:
            status = "flooded"
        elif a.kind != "road" and near_corridor.intersects(g) and raw is not None and raw <= MARGIN_HEIGHT_M:
            status = "margin"
        else:
            status = "outside"
        # lower than the flood level but not reached by it: the level does not apply, no height
        height = None if raw is None or (raw < 0 and not flooded) else raw + 0.0  # + 0.0: no "-0.0"
        lonlat = _lonlat(ref, crs)
        out.append(
            Impact(
                a.name,
                a.name_hi,
                a.kind,
                a.subkind,
                a.osm,
                lonlat[0],
                lonlat[1],
                round(km, 2),
                round(g.distance(path)),
                height,
                st.depth_m,
                st.arrival_min_expected,
                st.arrival_min_fast,
                status,
                round(g.intersection(corridor).length / 1000, 2) if a.kind in ("road", "bridge") and flooded else None,
            )
        )
    out.sort(key=lambda r: r.km)  # arrival order: nearest first, the order alerts fan out in
    return out


def _sample_points(geom, step: float) -> np.ndarray:
    """Points on `geom` at most `step` apart: a point itself, along lines, around and inside areas."""
    pts: list[tuple[float, float]] = []
    for part in getattr(geom, "geoms", [geom]):
        if part.is_empty:
            continue
        if part.geom_type == "Point":
            pts.append((part.x, part.y))
            continue
        if part.geom_type == "Polygon":
            inner = part.representative_point()
            pts.append((inner.x, inner.y))
            part = part.boundary
        if part.geom_type in ("LineString", "LinearRing", "MultiLineString"):
            for line in getattr(part, "geoms", [part]):
                n = max(1, math.ceil(line.length / step))
                pts += [line.interpolate(k / n, normalized=True).coords[0][:2] for k in range(n + 1)]
        else:  # a collection within a collection
            pts += map(tuple, _sample_points(part, step))
    return np.array(pts, float).reshape(-1, 2)


def _nearest_point(g, path):
    from shapely.ops import nearest_points

    return nearest_points(g, path)[0]


def _lonlat(pt, crs) -> tuple[float, float]:
    from .geo import to_wgs84

    p = to_wgs84(pt, crs)
    return round(p.x, 6), round(p.y, 6)


def fetch_along(d: Downstream, pad_deg: float = 0.03, chunk_km: float = 20.0) -> tuple[list[Asset], list[list[float]]]:
    """Assets in boxes along the path (one box over a 150 km diagonal is too big for Overpass).

    A chunk whose query keeps failing is skipped and returned as a gap [from_km, to_km]; cached
    chunks are reused, so a rerun fills the gaps.
    """
    seen: dict[str, Asset] = {}
    gaps: list[list[float]] = []
    lonlat = np.array([(s.lon, s.lat) for s in d.stations])
    km = np.array([s.km for s in d.stations])
    for start in np.arange(0, km[-1] + 1e-9, chunk_km):
        part = lonlat[(km >= start) & (km <= start + chunk_km)]
        if len(part) == 0:
            continue
        box = (
            part[:, 0].min() - pad_deg,
            part[:, 1].min() - pad_deg,
            part[:, 0].max() + pad_deg,
            part[:, 1].max() + pad_deg,
        )
        try:
            found = fetch_assets(tuple(round(b, 3) for b in box))
        except RuntimeError as e:
            log.warning("no OSM data for km %.0f-%.0f: %s", start, start + chunk_km, e)
            gaps.append([float(start), float(min(start + chunk_km, km[-1]))])
            continue
        for a in found:
            seen.setdefault(a.osm, a)
    return list(seen.values()), gaps


def merge_scenarios(per_scenario: dict[str, list[Impact]]) -> list[dict]:
    """One row per asset with each scenario's outcome and a summary status:
    in_flood_path (flooded in the expected scenario, whatever the severe one says), at_risk (not that,
    but flooded in the severe scenario or on the margin in either), outside (outside in both).
    Arrival times do not depend on the scenario."""
    rows: dict[str, dict] = {}
    for name, impacts in per_scenario.items():
        for i in impacts:
            row = rows.setdefault(
                i.osm,
                {
                    k: v
                    for k, v in asdict(i).items()
                    if k not in ("status", "height_above_flood_m", "flood_depth_m", "road_km_flooded")
                }
                | {"scenarios": {}},
            )
            row["scenarios"][name] = {
                "status": i.status,
                "height_above_flood_m": i.height_above_flood_m,
                "flood_depth_m": i.flood_depth_m,
                "road_km_flooded": i.road_km_flooded,
            }
    for row in rows.values():
        sc = row["scenarios"]
        if sc.get("expected", {}).get("status") == "flooded":
            row["status"] = "in_flood_path"
        elif any(v["status"] in ("flooded", "margin") for v in sc.values()):
            row["status"] = "at_risk"
        else:
            row["status"] = "outside"
    return sorted(rows.values(), key=lambda r: r["km"])


def run_downstream(lake_dir) -> dict:
    """Flood path, corridors and impacts for a lake from its latest risk record. Writes downstream.json
    (per-scenario stations and discharge, caveats), flood_path.geojson, corridor_<scenario>.geojson, impacts.json."""
    import json

    from shapely.geometry import mapping, shape

    from .downstream import compute_scenarios, path_geojson
    from .peak import peak_discharge

    risk = json.loads((lake_dir / "risk.json").read_text())["latest"]
    lake = shape(json.loads((lake_dir / f"{risk['area_year']}.geojson").read_text())["geometry"])
    t = risk["terrain"]
    peak = peak_discharge(risk["volume_m3"])
    ds = compute_scenarios(
        (t["outlet_lon"], t["outlet_lat"]), lake, {k: v["peak_m3s"] for k, v in peak["scenarios"].items()}
    )
    assets, gaps = fetch_along(ds["severe"])
    rows = merge_scenarios({name: assess(d, assets) for name, d in ds.items()})
    first = next(iter(ds.values()))
    out = {
        "lake_id": risk["lake_id"],
        "as_of_season": risk["as_of_season"],
        "volume_m3": risk["volume_m3"],
        "discharge": peak,
        "path_km": first.stations[-1].km if first.stations else 0,
        "exposure_gaps_km": gaps,  # stretches where OpenStreetMap could not be fetched: rerun to fill
        "params": first.params | {"margin_height_m": MARGIN_HEIGHT_M, "margin_distance_m": MARGIN_DISTANCE_M},
        "caveats": [
            "Screening estimate: Manning normal depth on 30 m DEM cross-sections, no hydrodynamic routing.",
            "Peak discharge from lake volume by empirical relations with large scatter; two scenarios shown.",
            "The DEM (2011-2015) has no river bathymetry; flood levels near the river are approximate.",
            "Arrival times assume a constant flood-front speed from observed Himalayan events.",
            "Places missing from OpenStreetMap are missing here.",
        ],
        "scenarios": {name: [asdict(s) for s in d.stations] for name, d in ds.items()},
    }
    (lake_dir / "downstream.json").write_text(json.dumps(out, ensure_ascii=False))
    (lake_dir / "flood_path.geojson").write_text(json.dumps(path_geojson(first)))
    for name, d in ds.items():
        # ~1 m coordinate precision: the files are for maps on slow phones, not for analysis
        geom = shapely.set_precision(d.corridor, 1e-5)
        corridor = {"type": "Feature", "geometry": mapping(geom), "properties": {"scenario": name}}
        (lake_dir / f"corridor_{name}.geojson").write_text(json.dumps(corridor))
    (lake_dir / "impacts.json").write_text(json.dumps(rows, ensure_ascii=False))
    return out | {"impacts": rows}
