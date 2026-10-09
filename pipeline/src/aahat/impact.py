"""Which settlements, bridges, roads and hydro projects a lake's flood could reach, and when.

Each OpenStreetMap asset is placed on the flood path: its distance downstream (projection onto
the path), its ground height relative to the screening flood level at that point, and whether it
lies in the inundation corridor. Results are ordered by arrival time, nearest first: the order in
which alerts fan out. All of it inherits the screening caveats of downstream.py, and OSM coverage
in the Himalaya is uneven, so a place missing from OSM is missing here.
"""

from __future__ import annotations

import logging
from dataclasses import asdict, dataclass

import numpy as np
from pyproj import CRS
from shapely.geometry import LineString
from shapely.prepared import prep

from .downstream import Downstream, sample
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
    height_above_flood_m: float | None  # ground minus screening flood level; negative = under water
    flood_depth_m: float  # at the nearest station, above the river bed
    arrival_min_expected: float
    arrival_min_fast: float
    status: str  # flooded | margin | outside
    road_km_flooded: float | None = None


def assess(d: Downstream, assets: list[Asset], max_lateral_m: float = 3000.0) -> list[Impact]:
    crs = CRS.from_user_input(d.crs)
    path = LineString(d.path_xy)
    corridor = from_wgs84(d.corridor, crs)
    near_corridor = prep(corridor.buffer(MARGIN_DISTANCE_M))
    inside = prep(corridor)
    station_km = np.array([s.km for s in d.stations])
    out: list[Impact] = []
    for a in assets:
        g = from_wgs84(a.geometry, crs)
        if g.distance(path) > max_lateral_m:
            continue
        # where the asset meets the flood first: its point nearest the river
        ref = g if g.geom_type == "Point" else _nearest_point(g, path)
        km = path.project(ref) / 1000
        if km > station_km[-1] + 0.5:
            continue
        # the flood level that applies here is the nearest station's, as for the corridor itself (on a
        # bend the station at the projected km can be a different reach)
        if d.station_xy is not None:
            i = int(np.argmin(np.hypot(d.station_xy[:, 0] - ref.x, d.station_xy[:, 1] - ref.y)))
        else:
            i = int(np.clip(np.searchsorted(station_km, km), 0, len(station_km) - 1))
        st = d.stations[i]
        z = float(sample(d.dem, d.grid, np.array([ref.x]), np.array([ref.y]))[0])
        height = None if not np.isfinite(z) else round(z - st.wse_m, 1)
        below = height is not None and height <= 0 and g.distance(path) <= d.params["corridor_m"]
        # A settlement or school is one OSM point: flooded only if its ground is below the flood level.
        # Lines and areas (bridges, roads, plants) count as flooded where they enter the corridor.
        flooded = below if g.geom_type == "Point" else (inside.intersects(g) or below)
        if flooded:
            status = "flooded"
        elif (
            a.kind not in ("road",) and near_corridor.intersects(g) and height is not None and height <= MARGIN_HEIGHT_M
        ):
            status = "margin"
        else:
            status = "outside"
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
    in_flood_path (flooded in the expected scenario), at_risk (flooded only in the severe one, or on
    the margin in either), outside. Arrival times do not depend on the scenario."""
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

    import shapely
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
