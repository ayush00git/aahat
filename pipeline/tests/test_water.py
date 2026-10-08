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
