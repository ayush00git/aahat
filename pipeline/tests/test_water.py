from datetime import date
from types import SimpleNamespace

import numpy as np
from affine import Affine
from pyproj import CRS

from aahat.dem import cast_shadow
from aahat.geo import Grid
from aahat.water import Composite, WaterParams, extract_lake

GRID = Grid(CRS.from_epsg(32643), Affine(10, 0, 0, 0, -10, 1000), 100, 100)


def test_cast_shadow_behind_wall():
    dem = np.zeros((50, 50), dtype="float32")
    dem[:, 25] = 500  # north-south wall, sun low in the east
    s = cast_shadow(dem, 30.0, sun_azimuth=90, sun_elevation=30)
    assert s[:, 10:24].all()  # west of the wall is shadowed (500 m / tan 30 = 866 m = ~29 px)
    assert not s[:, 26:].any()  # east of the wall is lit


def test_extract_lake_fills_icebergs_and_ignores_far_water():
    water = np.zeros(GRID.shape, bool)
    water[20:40, 20:40] = True
    water[28:31, 28:31] = False  # an iceberg
    water[80:90, 80:90] = True  # another lake, far from the seed
    comp = Composite(GRID, water, np.full(GRID.shape, 5, "int16"), water.astype("float32"))
    seed = GRID.transform @ (30, 30)
    ext = extract_lake(comp, seed, WaterParams())
    assert ext is not None
    assert ext.area_m2 == 400 * 100  # 20x20 px of 10 m, iceberg filled
    assert ext.coverage == 1.0


def test_extract_lake_none_when_seed_is_dry():
    water = np.zeros(GRID.shape, bool)
    water[80:90, 80:90] = True
    comp = Composite(GRID, water, np.full(GRID.shape, 5, "int16"), water.astype("float32"))
    assert extract_lake(comp, GRID.transform @ (10, 10), WaterParams()) is None


def test_composite_with_a_single_scene_lowers_min_obs(monkeypatch):
    import aahat.water as w

    water = np.zeros(GRID.shape, bool)
    water[20:40, 20:40] = True
    obs = w.SceneObs(date(2025, 9, 1), "x", np.ones(GRID.shape, bool), water, 1.0)
    monkeypatch.setattr(w, "read_clear", lambda s, g: np.ones(GRID.shape, bool))
    monkeypatch.setattr(w, "observe", lambda s, c, g, t, p: obs)
    terrain = SimpleNamespace(slope=np.zeros(GRID.shape, "float32"))
    scene = SimpleNamespace(day=date(2025, 9, 1))

    comp = w.composite([scene], GRID, terrain, WaterParams())
    assert comp.min_obs == 1
    ext = extract_lake(comp, GRID.transform @ (30, 30), WaterParams())
    assert ext is not None and ext.coverage == 1.0


def test_extract_lake_bridges_a_cloud_gap_but_not_dry_land():
    water = np.zeros(GRID.shape, bool)
    water[20:40, 10:30] = True
    water[20:40, 35:55] = True  # same lake beyond a 5 px cloud strip
    water[20:40, 60:80] = True  # separate lake beyond 5 px of land seen to be dry
    obs = np.full(GRID.shape, 3, "int16")
    obs[:, 30:35] = 0  # never seen: cloud over the lake in every scene
    comp = Composite(GRID, water, obs, water.astype("float32"))
    ext = extract_lake(comp, GRID.transform @ (20, 30), WaterParams())
    assert ext is not None
    # both halves plus the enclosed-by-closing part of the strip, but not the third lake
    assert 800 * 100 <= ext.area_m2 < 1200 * 100
    assert not ext.mask[:, 60:80].any()
