"""An asset's status and its height above the flood come from one flood level and cannot disagree."""

import itertools
from dataclasses import asdict

import numpy as np
import pytest
from affine import Affine
from pyproj import CRS
from shapely.geometry import LineString, Point, box
from test_downstream import X0, Y0, routed_plateau

from aahat.downstream import Downstream, Station
from aahat.exposure import Asset
from aahat.geo import Grid, to_wgs84
from aahat.impact import MARGIN_DISTANCE_M, MARGIN_HEIGHT_M, Impact, assess, merge_scenarios

CRS_UTM = CRS.from_epsg(32643)
VX, VY = 700_000.0, 3_600_000.0
LEVEL = 1010.0
TERRACE_M = 1014.0  # 4 m above the flood


def asset(n, kind, geom, name=None, crs=CRS_UTM):
    return Asset("node" if geom.geom_type == "Point" else "way", n, kind, kind, name, None, to_wgs84(geom, crs))


def terrace_valley(wse=lambda km: LEVEL) -> Downstream:
    """A river running +x. The bank rises 0.1 m per m for 90 m (the flood at 1010 m wets all of it),
    then steps up to a flat terrace at 1014 m. The corridor is the wetted floor, 90 m either side."""
    grid = Grid(CRS_UTM, Affine(30, 0, VX - 3000, 0, -30, VY + 3000), 400, 200)
    ys = VY + 3000 - 30 * (np.arange(200) + 0.5)
    d_river = np.abs(ys - VY)
    dem = np.where(d_river < 90, 1000 + 0.1 * d_river, TERRACE_M)[:, None] + np.zeros((1, 400))
    path = [(VX + i * 100.0, VY) for i in range(81)]  # 8 km east
    stations = [
        Station(km, 0, 0, 1000, 0.01, 5000, wse(km), 10.0, 180, km * 1000 / 8 / 60, km * 1000 / 10 / 60, False, 90, 90)
        for km in np.arange(0, 8.25, 0.25)
    ]
    wetted = to_wgs84(box(VX, VY - 90, VX + 8000, VY + 90), CRS_UTM)
    return Downstream(CRS_UTM.to_string(), path, stations, wetted, {"corridor_m": 2000}, {}, grid, dem, None)


def test_village_on_a_terrace_above_the_flood_is_on_the_margin_not_flooded():
    out = assess(
        terrace_valley(),
        [
            asset(1, "settlement", Point(VX + 3000, VY + 110), "20 m from the water"),
            # within half a DEM cell of the corridor: what used to be called flooded at +4 m
            asset(2, "settlement", Point(VX + 3000, VY + 105), "15 m from the water"),
        ],
    )
    assert [(i.status, i.height_above_flood_m) for i in out] == [("margin", 4.0), ("margin", 4.0)]
    assert all(i.lateral_m <= MARGIN_DISTANCE_M and i.height_above_flood_m <= MARGIN_HEIGHT_M for i in out)


def test_village_below_the_flood_level_inside_the_wetted_reach_is_flooded():
    (v,) = assess(terrace_valley(), [asset(1, "settlement", Point(VX + 3000, VY + 45), "on the bank")])
    assert v.status == "flooded"
    assert v.height_above_flood_m == pytest.approx(1004.5 - LEVEL)  # 5.5 m under water


def test_the_flood_level_at_an_asset_is_interpolated_between_stations():
    d = terrace_valley(wse=lambda km: 1010.0 - 8.0 * km)  # falls 2 m from one station to the next
    # 30 m past the station at 3.0 km (level 986.0 there): the level is 985.76, not the station's
    (v,) = assess(d, [asset(1, "settlement", Point(VX + 3030, VY + 300), "terrace")])
    assert v.height_above_flood_m == pytest.approx(TERRACE_M - (1010.0 - 8.0 * 3.03), abs=0.051)
    assert v.status == "outside"  # 28 m above


def test_lines_report_the_lowest_ground_the_flood_reaches_and_roads_are_never_margin():
    out = assess(
        terrace_valley(),
        [
            asset(1, "bridge", LineString([(VX + 2000, VY - 150), (VX + 2000, VY + 150)]), "bridge"),
            asset(2, "road", LineString([(VX + 1000, VY + 150), (VX + 5000, VY + 150)]), "terrace road"),
            asset(3, "bridge", LineString([(VX + 6000, VY + 120), (VX + 6000, VY + 200)]), "side bridge"),
        ],
    )
    by = {i.name: i for i in out}
    # the bridge's own nearest point to the river is at the river: ground 1001.5 m in that DEM cell
    assert by["bridge"].status == "flooded"
    assert by["bridge"].height_above_flood_m == pytest.approx(-8.5)
    assert by["bridge"].road_km_flooded == pytest.approx(0.18)
    assert (by["terrace road"].status, by["terrace road"].height_above_flood_m) == ("outside", 4.0)
    assert by["terrace road"].road_km_flooded is None
    assert (by["side bridge"].status, by["side bridge"].height_above_flood_m) == ("margin", 4.0)


def synthetic_assets(crs):
    """Points on a lattice over the plateau, the valley and the low country behind its wall, with
    roads, bridges and plants (areas) across and along the valley."""
    n = itertools.count(1)
    out = []
    for x in np.arange(-290.0, 4200.0, 97.0):
        for y in np.arange(-700.0, 900.0, 23.0):
            out.append(asset(next(n), "settlement", Point(X0 + x, Y0 + y), crs=crs))
    for x in np.arange(40.0, 4000.0, 210.0):
        for a, b in ((-300, 300), (20, 80), (60, 300), (150, 600), (450, 800)):
            out.append(asset(next(n), "bridge", LineString([(X0 + x, Y0 + a), (X0 + x + 15, Y0 + b)]), crs=crs))
    for y in np.arange(-200.0, 760.0, 35.0):
        out.append(asset(next(n), "road", LineString([(X0 + 100, Y0 + y), (X0 + 3900, Y0 + y + 40)]), crs=crs))
    for x in np.arange(200.0, 3800.0, 400.0):
        for y in (-150.0, 0.0, 40.0, 120.0, 500.0):
            out.append(asset(next(n), "hydro", box(X0 + x, Y0 + y, X0 + x + 90, Y0 + y + 70), crs=crs))
    return out


def test_status_and_height_agree_for_every_asset_in_both_scenarios(monkeypatch):
    per_scenario = {}
    for name, peak in (("expected", 2000.0), ("severe", 8000.0)):
        t = routed_plateau(monkeypatch, peak)
        d = Downstream(
            t.crs.to_string(),
            [tuple(xy) for xy in t.path],
            t.stations,
            to_wgs84(t.flooded, t.crs),
            asdict(t.p),
            {},
            t.grid,
            t.dem,
            None,
        )
        per_scenario[name] = out = assess(d, synthetic_assets(t.crs))
        assert len(out) > 2000
        for i in out:
            h = i.height_above_flood_m
            if i.status == "flooded":
                assert h is not None and h < 0, i
            else:
                assert h is None or h >= 0, i
            if i.status == "margin":
                assert h is None or h <= MARGIN_HEIGHT_M, i
                assert i.kind != "road"
        # every outcome occurs, for points and for lines and areas, or the test proves nothing
        seen = {(i.kind == "settlement", i.status) for i in out}
        assert seen == set(itertools.product((True, False), ("flooded", "margin", "outside")))
        # ground lower than the flood behind the valley wall is not reached and gets no height
        assert any(i.status == "outside" and i.height_above_flood_m is None for i in out)
        assert any(i.status == "outside" and i.height_above_flood_m is not None for i in out)
    # the merged rows carry the same pairs through unchanged
    for row in merge_scenarios(per_scenario):
        for sc in row["scenarios"].values():
            assert (sc["status"] == "flooded") == (
                sc["height_above_flood_m"] is not None and sc["height_above_flood_m"] < 0
            )
    flooded = {n: sum(i.status == "flooded" for i in out) for n, out in per_scenario.items()}
    assert flooded["severe"] > flooded["expected"] > 0


MERGED = {  # (expected, severe) -> summary status
    ("flooded", "flooded"): "in_flood_path",
    ("flooded", "margin"): "in_flood_path",  # cannot happen with a larger severe flood; expected wins
    ("flooded", "outside"): "in_flood_path",
    ("margin", "flooded"): "at_risk",  # flooded only in the severe scenario
    ("outside", "flooded"): "at_risk",
    ("margin", "margin"): "at_risk",
    ("margin", "outside"): "at_risk",
    ("outside", "margin"): "at_risk",
    ("outside", "outside"): "outside",
}


@pytest.mark.parametrize(("expected", "severe"), list(itertools.product(("flooded", "margin", "outside"), repeat=2)))
def test_merge_scenarios_maps_every_pair_of_outcomes(expected, severe):
    def imp(status):
        return Impact("v", None, "settlement", "village", "node/1", 0, 0, 5, 0, 1.0, 5.0, 10, 8, status)

    (row,) = merge_scenarios({"expected": [imp(expected)], "severe": [imp(severe)]})
    assert row["status"] == MERGED[(expected, severe)]
    assert (row["scenarios"]["expected"]["status"], row["scenarios"]["severe"]["status"]) == (expected, severe)
