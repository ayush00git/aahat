import json
from datetime import date

import numpy as np
from affine import Affine
from pyproj import CRS
from shapely.geometry import box, mapping

from aahat import drain
from aahat.drain import DrainParams, Season, assess_scene, season_baseline
from aahat.geo import Grid, to_wgs84
from aahat.water import SceneObs

GRID = Grid(CRS.from_epsg(32643), Affine(10, 0, 0, 0, -10, 1000), 100, 100)
SEED = GRID.transform @ (30, 30)
FLAT = np.zeros(GRID.shape, "float32")
DAY = date(2026, 10, 8)


def season() -> Season:
    outline = np.zeros(GRID.shape, bool)
    outline[20:40, 20:40] = True  # 400 px = 40,000 m2, 800 m of shore -> +/- 4,000 m2
    return Season(2026, 40_000.0, 4_000.0, outline)


def scene(water_rows: slice, observed: np.ndarray | None = None) -> SceneObs:
    water = np.zeros(GRID.shape, bool)
    water[water_rows, 20:40] = True
    observed = np.ones(GRID.shape, bool) if observed is None else observed
    return SceneObs(DAY, "S2A_TEST", observed, water & observed, float(observed.mean()))


def clear_scl() -> np.ndarray:
    return np.full(GRID.shape, 5, "uint8")  # bare soil: a clear look


def test_drop_with_full_coverage_is_drained():
    look = assess_scene(season(), scene(slice(20, 30)), clear_scl(), GRID, FLAT, SEED)  # half the lake is left
    assert look.status == "ok" and look.drained
    assert look.area_m2 == 20_000 and look.drop_fraction == 0.5 and look.coverage == 1.0


def test_same_drop_with_half_the_lake_unobserved_is_not_drained():
    observed = np.ones(GRID.shape, bool)
    observed[30:40, :] = False  # a cloud over the half that "lost" its water
    look = assess_scene(season(), scene(slice(20, 30), observed), clear_scl(), GRID, FLAT, SEED)
    assert look.status == "no_clear_scene" and not look.drained
    assert look.coverage == 0.5 and look.drop_fraction is None


def test_snow_on_the_lake_is_frozen_never_drained():
    scl = clear_scl()
    scl[28:40, 20:40] = 11  # snow/ice over 60% of the outline
    observed = scl != 11
    look = assess_scene(season(), scene(slice(20, 28), observed), scl, GRID, FLAT, SEED)
    assert look.status == "frozen_or_snow" and not look.drained


def test_unchanged_lake_and_small_seasonal_shrinkage_are_not_drained():
    same = assess_scene(season(), scene(slice(20, 40)), clear_scl(), GRID, FLAT, SEED)
    assert same.status == "ok" and not same.drained and same.drop_fraction == 0.0
    smaller = assess_scene(season(), scene(slice(20, 36)), clear_scl(), GRID, FLAT, SEED)  # -20%
    assert smaller.status == "ok" and not smaller.drained


def test_threshold_follows_uncertainty_for_small_lakes():
    s = season()
    s.uncertainty_m2 = 8_000.0  # a small, ragged lake: 3 sigma is far above 30%
    look = assess_scene(s, scene(slice(20, 30)), clear_scl(), GRID, FLAT, SEED)
    assert look.threshold_fraction > 0.5 and not look.drained


def test_lake_gone_from_a_clear_scene_is_drained():
    look = assess_scene(season(), scene(slice(0, 0)), clear_scl(), GRID, FLAT, SEED)
    assert look.drained and look.area_m2 == 0.0 and look.drop_fraction == 1.0


def write_series(lake_dir, years):
    lake_dir.mkdir(parents=True, exist_ok=True)
    geom = mapping(to_wgs84(box(200, 600, 400, 800), GRID.crs))  # rows/cols 20:40
    for y in years:
        (lake_dir / f"{y['year']}.geojson").write_text(json.dumps({"type": "Feature", "geometry": geom}))
    (lake_dir / "series.json").write_text(json.dumps({"years": years}))


def test_baseline_is_latest_ok_season_up_to_as_of(tmp_path):
    rec = {"status": "ok", "area_m2": 40_000.0, "uncertainty_m2": 4_000.0}
    write_series(tmp_path, [rec | {"year": 2024}, rec | {"year": 2025}, rec | {"year": 2026, "status": "partial"}])
    s = season_baseline(tmp_path, date(2026, 10, 8), GRID)
    assert s.year == 2025 and s.outline.sum() == 400
    assert season_baseline(tmp_path, date(2024, 9, 1), GRID).year == 2024  # replay: no later seasons
    assert season_baseline(tmp_path, date(2023, 9, 1), GRID) is None


def test_latest_look_skips_cloud_but_stops_at_snow(monkeypatch):
    from types import SimpleNamespace

    scenes = [SimpleNamespace(day=date(2026, 10, d), item_id=f"s{d}") for d in (1, 4, 7)]
    cloudy, snowy = np.full(GRID.shape, 9, "uint8"), np.full(GRID.shape, 11, "uint8")
    terrain = SimpleNamespace(slope=FLAT)
    half = lambda s, scl, g, t, p: SceneObs(
        s.day, s.item_id, np.ones(GRID.shape, bool), scene(slice(20, 30)).water, 1.0
    )
    monkeypatch.setattr(drain, "observe", half)

    scl_by_day = {1: clear_scl(), 4: clear_scl(), 7: cloudy}
    monkeypatch.setattr(drain, "read_scl", lambda s, g: scl_by_day[s.day.day])
    look = drain.latest_look(scenes, season(), GRID, terrain, SEED)
    assert look.item_id == "s4" and look.drained  # newest clear scene, not the cloudy newest

    scl_by_day[7] = snowy
    look = drain.latest_look(scenes, season(), GRID, terrain, SEED)
    assert look.item_id == "s7" and look.status == "frozen_or_snow" and not look.drained

    scl_by_day.update({1: cloudy, 4: cloudy, 7: cloudy})
    assert drain.latest_look(scenes, season(), GRID, terrain, SEED).status == "no_clear_scene"
    assert drain.summarise(None, DrainParams())["status"] == "no_clear_scene"
