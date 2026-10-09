import math

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
    wse, top, capped = normal_depth_wse(offsets, ground, q, slope, n)
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
    d = Downstream(crs.to_string(), path, stations, to_wgs84(corridor_utm, crs), {"corridor_m": 2000}, {}, grid, dem, None)

    def asset(kind, geom, name):
        return Asset("node", hash(name) % 10_000, kind, "village" if kind == "settlement" else "road_bridge", name, None, to_wgs84(geom, crs))

    assets = [
        asset("settlement", Point(x0 + 6000, y0 + 50), "low village"),  # 5 m above river, inside corridor
        asset("settlement", Point(x0 + 2000, y0 + 150), "terrace village"),  # ground 1015 m: 5 m above the flood, 50 m out
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
