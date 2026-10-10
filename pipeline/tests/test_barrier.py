import numpy as np
from shapely.geometry import LineString, Point

from aahat import barrier
from aahat.barrier import BarrierParams, Candidate, new_water_blobs, tag_reservoirs
from aahat.exposure import Asset

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


def candidate(lat: float, lon: float) -> Candidate:
    return Candidate(lon, lat, 107_000, 120, 60.0, 0.02, ["S2A_TEST"], "2026-10-09")


def test_candidate_near_a_dam_is_a_reservoir_level_change(monkeypatch):
    # Regression: the Lam Dal scan flagged the head of the Chamera reservoir (Ravi) refilling. Chamera
    # Dam is mapped as a power plant, so it arrives as subkind hydro_plant, 8.4 km away.
    chamera = Asset("way", 1, "hydro", "hydro_plant", "Chamera Dam", None, Point(75.9856, 32.5970))
    school = Asset("node", 2, "school", "school", "GSSS", None, Point(76.07, 32.62))  # near, but not a dam
    monkeypatch.setattr(barrier, "fetch_assets", lambda bounds: [chamera, school])
    cands = [candidate(32.61817, 76.0715), candidate(32.80, 76.40)]
    tag_reservoirs(cands)
    assert cands[0].kind == "reservoir_level_change" and 8.2 < cands[0].near_dam_km < 8.6
    assert cands[1].kind == "possible_barrier_lake" and cands[1].near_dam_km is None  # 40 km upstream


def test_dam_distance_is_to_its_nearest_point_and_no_dam_means_barrier_lake(monkeypatch):
    weir = Asset("way", 3, "hydro", "weir", None, None, LineString([(76.0, 32.0), (76.3, 32.0)]))  # centre 14 km off
    monkeypatch.setattr(barrier, "fetch_assets", lambda bounds: [weir])
    cands = [candidate(32.0, 76.31)]
    tag_reservoirs(cands)
    assert cands[0].kind == "reservoir_level_change" and cands[0].near_dam_km < 1.5

    monkeypatch.setattr(barrier, "fetch_assets", lambda bounds: [])
    cands = [candidate(32.0, 76.31)]
    tag_reservoirs(cands)
    assert cands[0].kind == "possible_barrier_lake" and cands[0].near_dam_km is None


def test_failed_dam_lookup_leaves_the_candidate_a_possible_barrier_lake(monkeypatch):
    def down(bounds):
        raise RuntimeError("all Overpass endpoints failed")

    monkeypatch.setattr(barrier, "fetch_assets", down)
    cands = [candidate(29.75, 94.93)]  # Sedongpu: outside the local extract
    tag_reservoirs(cands)
    assert cands[0].kind == "possible_barrier_lake" and cands[0].near_dam_km is None
