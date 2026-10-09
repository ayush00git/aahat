import numpy as np

from aahat.barrier import BarrierParams, new_water_blobs

RES = 20.0


def setup():
    shape = (100, 100)
    corridor = np.zeros(shape, bool)
    corridor[:, 30:70] = True  # river corridor
    base_freq = np.zeros(shape, "float32")
    base_freq[:, 48:52] = 1.0  # the river itself was always water
    base_obs = np.full(shape, 5, "int16")
    now = base_freq > 0.5
    return now, base_freq, base_obs, corridor


def test_dammed_lake_blob_is_found():
    now, base_freq, base_obs, corridor = setup()
    now = now.copy()
    now[40:60, 38:62] = True  # a lake backed up behind a dam: 20 x 24 px = 0.19 km2, 480 m wide
    blobs = new_water_blobs(now, base_freq, base_obs, corridor, RES, BarrierParams())
    assert len(blobs) == 1
    _, area, width = blobs[0]
    assert area >= 0.15e6 and width >= 100


def test_river_widening_sliver_is_ignored():
    now, base_freq, base_obs, corridor = setup()
    now = now.copy()
    now[:, 52:54] = True  # banks overtopped along the river: 40 m wide, 2 km long
    assert new_water_blobs(now, base_freq, base_obs, corridor, RES, BarrierParams()) == []


def test_water_outside_corridor_or_unseen_before_is_ignored():
    now, base_freq, base_obs, corridor = setup()
    now = now.copy()
    now[40:60, 5:25] = True  # a pond far from the river
    now[70:90, 38:46] = True
    base_obs[70:90, 38:46] = 0  # never clearly seen in the baseline: can't call it new
    assert new_water_blobs(now, base_freq, base_obs, corridor, RES, BarrierParams()) == []
