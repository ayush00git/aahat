import math
from types import SimpleNamespace

import numpy as np

from aahat.downstream import manning_discharge, normal_depth_wse


def rectangular(width=60.0, wall=200.0, step=10.0, half=300.0):
    offsets = np.arange(-half, half + step, step)
    ground = np.where(np.abs(offsets) <= width / 2, 0.0, wall)
    return offsets, ground


def test_normal_depth_matches_rectangular_channel_formula():
    offsets, ground = rectangular()
    width = 70.0  # the 10 m sampling makes the wet span 7 cells
    depth, slope, n = 5.0, 0.01, 0.07
    area, perim = width * depth, width + 2 * depth
    q = area * (area / perim) ** (2 / 3) * math.sqrt(slope) / n
    wse, _, capped = normal_depth_wse(offsets, ground, q, slope, n)
    assert abs(wse - depth) < 0.3
    assert not capped


def test_more_discharge_means_higher_water():
    offsets = np.arange(-1000, 1010, 30.0)
    ground = np.abs(offsets) * 0.2  # V-shaped valley
    low = normal_depth_wse(offsets, ground, 1_000, 0.01, 0.07)[0]
    high = normal_depth_wse(offsets, ground, 40_000, 0.01, 0.07)[0]
    assert high > low > 0


def test_water_does_not_cross_a_ridge_into_the_next_channel():
    offsets = np.arange(-600, 610, 30.0)
    ground = np.abs(offsets) * 0.1
    ground[offsets > 200] = 100.0  # ridge ...
    ground[offsets > 400] = 0.0  # ... then another, empty channel
    q, width, _ = manning_discharge(offsets, ground, 15.0, 0.01, 0.07)
    assert width < 400  # only the central channel is wet
    assert q > 0


def test_assess_flags_flooded_margin_and_outside_assets_in_arrival_order():
    from affine import Affine
    from pyproj import CRS
    from shapely.geometry import LineString, Point, box

    from aahat.downstream import Downstream, Station
    from aahat.exposure import Asset
    from aahat.geo import Grid, to_wgs84
    from aahat.impact import assess

    crs = CRS.from_epsg(32643)
    x0, y0 = 700_000.0, 3_600_000.0
    grid = Grid(crs, Affine(30, 0, x0 - 3000, 0, -30, y0 + 3000), 400, 200)
    # valley along +x: river at y0, ground rises 0.1 m per m away from it
    xs = x0 - 3000 + 30 * (np.arange(400) + 0.5)
    ys = y0 + 3000 - 30 * (np.arange(200) + 0.5)
    dem = (1000 + 0.1 * np.abs(ys - y0))[:, None] + 0 * xs[None, :]
    path = [(x0 + i * 100.0, y0) for i in range(81)]  # 8 km east
    stations = [
        Station(km, 0, 0, 1000, 0.01, 5000, 1010.0, 10.0, 200, km * 1000 / 8 / 60, km * 1000 / 10 / 60, False)
        for km in np.arange(0, 8.25, 0.25)
    ]
    corridor_utm = box(x0, y0 - 100, x0 + 8000, y0 + 100)  # flood level 1010 m -> 100 m either side
    d = Downstream(
        crs.to_string(), path, stations, to_wgs84(corridor_utm, crs), {"corridor_m": 2000}, {}, grid, dem, None
    )

    def asset(kind, geom, name):
        return Asset(
            "node",
            hash(name) % 10_000,
            kind,
            "village" if kind == "settlement" else "road_bridge",
            name,
            None,
            to_wgs84(geom, crs),
        )

    assets = [
        asset("settlement", Point(x0 + 6000, y0 + 50), "low village"),  # 5 m above river, inside corridor
        asset(
            "settlement", Point(x0 + 2000, y0 + 150), "terrace village"
        ),  # ground 1015 m: 5 m above the flood, 50 m out
        asset("settlement", Point(x0 + 4000, y0 + 1500), "high village"),  # +150 m
        asset("bridge", LineString([(x0 + 3000, y0 - 150), (x0 + 3000, y0 + 150)]), "bridge"),
        asset("settlement", Point(x0 + 4000, y0 + 9000), "far away"),
    ]
    out = assess(d, assets)
    by = {i.name: i for i in out}
    assert "far away" not in by
    assert by["low village"].status == "flooded"
    assert by["bridge"].status == "flooded"
    assert by["high village"].status == "outside"
    assert by["terrace village"].status == "margin"  # within 10 m above the flood and 300 m of the corridor
    assert [i.name for i in out] == ["terrace village", "bridge", "high village", "low village"]  # by km
    assert by["bridge"].arrival_min_fast < by["low village"].arrival_min_fast


def test_merge_scenarios_summary_status():
    from aahat.impact import Impact, merge_scenarios

    def imp(osm, km, status):
        return Impact("v", None, "settlement", "village", osm, 0, 0, km, 0, 1.0, 5.0, 10, 8, status)

    rows = merge_scenarios(
        {
            "expected": [imp("node/1", 5, "flooded"), imp("node/2", 3, "outside"), imp("node/3", 9, "outside")],
            "severe": [imp("node/1", 5, "flooded"), imp("node/2", 3, "flooded"), imp("node/3", 9, "outside")],
        }
    )
    assert [r["osm"] for r in rows] == ["node/2", "node/1", "node/3"]  # by km
    assert [r["status"] for r in rows] == ["at_risk", "in_flood_path", "outside"]
    assert rows[0]["scenarios"]["severe"]["status"] == "flooded"


def test_attenuation_halves_after_about_29_km():
    from aahat.downstream import discharge_at

    assert abs(discharge_at(42.4 * math.log(2), 1000, 42.4) - 500) < 1


# ---- synthetic terrain: a lake on a ridge-top plateau that drains into a V-shaped valley ----------

X0, Y0 = 700_020.0, 3_600_000.0  # multiples of the 30 m cell
SIDE = 0.1  # valley side slope: the floor is 2 * depth / SIDE wide at a given depth
RIDGE_Y = 400.0  # crest of the valley's left wall, 40 m above the river


def plateau_and_valley(grid):
    """x < 0: a flat plateau at 4000 m (the lake sits on it) whose back and sides fall away steeply.
    x >= 0: a V valley running +x, falling 5 m per 100 m. Beyond the crest of its left wall the
    ground drops 0.8 m per m into lower country: lower than the flood, but behind the ridge."""
    cols, rows = np.meshgrid(np.arange(grid.width) + 0.5, np.arange(grid.height) + 0.5)
    x, y = grid.transform @ (cols, rows)
    x, y = x - X0, y - Y0
    river = 4000 - 0.05 * x
    valley = np.where(
        y <= RIDGE_Y, river + SIDE * np.abs(y), river + SIDE * RIDGE_Y - 0.8 * (y - RIDGE_Y)
    )  # left wall, crest, then down the far side
    off_plateau = np.maximum(0, np.abs(y) - 600) + np.maximum(0, -x - 600)
    plateau = 4000 - 0.3 * off_plateau
    return np.where(x < 0, plateau, valley).astype("float32")


def routed_plateau(monkeypatch, peak=2000.0):
    from pyproj import CRS
    from shapely.geometry import Point

    from aahat import downstream as ds
    from aahat.geo import from_wgs84

    monkeypatch.setattr(ds, "read_dem", plateau_and_valley)
    p = ds.DownstreamParams()
    crs = CRS.from_epsg(32643)
    path = np.array([(X0 + 15.0 + i * 30.0, Y0 + 15.0) for i in range(134)])  # 4 km down the valley
    lake = Point(X0 - 200, Y0).buffer(190)  # on the plateau, its shore at the spill point
    stations, grid, dem, sxy = ds.route(crs.to_string(), path, peak, p, lake)
    flooded = from_wgs84(ds.corridor(stations, grid, dem, sxy, p, path, lake), crs)
    return SimpleNamespace(
        ds=ds, p=p, crs=crs, path=path, lake=lake, stations=stations, grid=grid, dem=dem, flooded=flooded
    )


def test_corridor_from_a_plateau_lake_is_no_wider_than_the_valley_floor(monkeypatch):
    t = routed_plateau(monkeypatch)
    lake, stations, grid, flooded = t.lake, t.stations, t.grid, t.flooded
    depth = max(s.depth_m for s in stations)
    assert 3 < depth < 10
    half_floor = depth / SIDE  # where the valley side reaches the flood level
    minx, miny, _, maxy = flooded.bounds
    assert maxy - (Y0 + 15) <= half_floor + 1.5 * grid.res
    assert (Y0 + 15) - miny <= half_floor + 1.5 * grid.res
    assert max(s.left_m + s.right_m for s in stations) <= 2 * half_floor + 2 * grid.res
    # it starts at the spill point: nothing on the plateau behind it, and not the lake
    assert minx >= X0 - grid.res
    assert flooded.intersection(lake).area < grid.res**2
    assert flooded.area > 0.5 * 4000 * 2 * 3 / SIDE  # and it is still a real corridor down the valley


def test_nothing_floods_behind_a_ridge_higher_than_the_flood(monkeypatch):
    from shapely.geometry import box

    t = routed_plateau(monkeypatch)
    stations, grid, dem, flooded = t.stations, t.grid, t.dem, t.flooded
    behind = box(X0 - 3000, Y0 + RIDGE_Y, X0 + 6000, Y0 + 3000)
    # the ground there IS lower than the flood level and within corridor_m of the river ...
    cols, rows = np.meshgrid(np.arange(grid.width) + 0.5, np.arange(grid.height) + 0.5)
    x, y = grid.transform @ (cols, rows)
    low = (y > Y0 + RIDGE_Y + 100) & (y < Y0 + 1500) & (x > X0 + 1000) & (x < X0 + 1200)
    assert (dem[low] < stations[4].wse_m).all()
    # ... but the ridge is higher than the water, so none of it is flooded
    assert flooded.intersection(behind).area == 0


def test_wet_extent_stops_where_the_section_first_rises_above_the_water():
    from aahat.downstream import channel_centre, wet_extent

    offsets = np.arange(-600, 610, 30.0)
    ground = np.abs(offsets) * 0.1
    ground[offsets > 200] = 100.0  # ridge ...
    ground[offsets > 400] = 0.0  # ... then lower ground behind it
    left, right = wet_extent(offsets, ground, 15.0, channel_centre(ground))
    assert left == 120 + 15  # the last wet sample towards the ridge, plus half a step
    assert right == 120 + 15
    assert wet_extent(offsets, ground, -1.0, channel_centre(ground)) == (15.0, 15.0)  # dry: just the channel cell


def test_lake_cells_on_a_cross_section_carry_no_flow(monkeypatch):
    t = routed_plateau(monkeypatch)
    ds, p, crs, path, stations = t.ds, t.p, t.crs, t.path, t.stations
    from shapely.geometry import box

    # a "lake" lying across the valley floor to the right of the path at every station
    slab = box(X0 - 100, Y0 - 2000, X0 + 5000, Y0 - 90)
    walled, *_ = ds.route(crs.to_string(), path, 2000.0, p, slab)
    assert max(s.right_m for s in walled[2:10]) <= 90 + 30
    assert walled[5].wse_m >= stations[5].wse_m  # less room for the same discharge: not lower


def test_assets_lower_than_the_flood_but_outside_the_corridor_are_not_flooded():
    from affine import Affine
    from pyproj import CRS
    from shapely.geometry import LineString, Point, box

    from aahat.downstream import Downstream, Station
    from aahat.exposure import Asset
    from aahat.geo import Grid, to_wgs84
    from aahat.impact import assess

    crs = CRS.from_epsg(32643)
    x0, y0 = 700_000.0, 3_600_000.0
    grid = Grid(crs, Affine(30, 0, x0 - 3000, 0, -30, y0 + 3000), 400, 200)
    xs = x0 - 3000 + 30 * (np.arange(400) + 0.5)
    ys = y0 + 3000 - 30 * (np.arange(200) + 0.5)
    d_river = np.abs(ys - y0)
    # valley walls rise to 1050 m at 500 m from the river, then the ground falls to 900 m: lower than the flood
    profile = np.where(d_river <= 500, 1000 + 0.1 * d_river, 900.0)
    dem = profile[:, None] + 0 * xs[None, :]
    path = [(x0 + i * 100.0, y0) for i in range(81)]
    stations = [
        Station(km, 0, 0, 1000, 0.01, 5000, 1010.0, 10.0, 200, km * 1000 / 8 / 60, km * 1000 / 10 / 60, False, 100, 100)
        for km in np.arange(0, 8.25, 0.25)
    ]
    corridor_utm = box(x0, y0 - 100, x0 + 8000, y0 + 100)
    d = Downstream(
        crs.to_string(), path, stations, to_wgs84(corridor_utm, crs), {"corridor_m": 2000}, {}, grid, dem, None
    )

    def asset(n, kind, geom, name):
        return Asset(
            "node", n, kind, "village" if kind == "settlement" else "footbridge", name, None, to_wgs84(geom, crs)
        )

    out = assess(
        d,
        [
            asset(1, "settlement", Point(x0 + 50, y0 + 1200), "village behind the ridge"),
            asset(2, "bridge", LineString([(x0 + 10, y0 + 1500), (x0 + 40, y0 + 1500)]), "footbridge behind the ridge"),
            asset(3, "settlement", Point(x0 + 3000, y0 + 40), "village on the river"),
        ],
    )
    by = {i.name: i for i in out}
    # lower than the water and within 2 km, but not reached: the level does not apply, so no height
    assert by["village behind the ridge"].height_above_flood_m is None
    assert by["village behind the ridge"].status == "outside"
    assert by["footbridge behind the ridge"].status == "outside"
    assert by["village on the river"].status == "flooded"
    assert [i.name for i in out if i.status != "outside"] == ["village on the river"]  # the first exposed asset


def test_small_lakes_have_one_scenario_and_say_so():
    from aahat.peak import peak_discharge

    small = peak_discharge(24_100)  # a 0.006 km2 lake: Evans 151 m3/s, Huggel 22 m3/s
    assert small["scenarios_identical"]
    assert small["scenarios"]["expected"] == small["scenarios"]["severe"]
    assert small["scenarios"]["severe"]["relation_key"] == "evans1986"
    assert small["all"]["evans1986"] > small["all"]["huggel2002"]
    big = peak_discharge(38_700_000)
    assert not big["scenarios_identical"]
    assert big["scenarios"]["severe"]["relation_key"] == "huggel2002"
    assert big["scenarios"]["severe"]["peak_m3s"] > big["scenarios"]["expected"]["peak_m3s"]
