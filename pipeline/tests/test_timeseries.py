import json

from aahat.timeseries import build_index


def test_build_index_picks_first_and_latest_measured_years(tmp_path):
    lake_dir = tmp_path / "lakes" / "x"
    lake_dir.mkdir(parents=True)
    rec = {"status": "ok", "uncertainty_m2": 1.0, "coverage": 1.0, "scenes_clear": 3}
    series = {
        "lake": {"id": "x", "name": "X"},
        "years": [
            rec | {"year": 2017, "area_m2": None, "status": "no_data"},
            rec | {"year": 2018, "area_m2": 100.0},
            rec
            | {
                "year": 2019,
                "area_m2": 120.0,
                "scene_ids": ["S2A_1", "S2B_2"],
                "scene_days": ["2019-09-01", "2019-09-06"],
            },
            rec | {"year": 2020, "area_m2": None, "status": "not_found"},
        ],
    }
    (lake_dir / "series.json").write_text(json.dumps(series))
    for year in (2018, 2019):
        (lake_dir / f"{year}.geojson").write_text(json.dumps({"type": "Feature", "properties": {"year": year}}))
    index = build_index(tmp_path)
    lake = index["lakes"][0]
    assert lake["first"]["year"] == 2018
    assert lake["latest"]["year"] == 2019
    assert len(lake["years"]) == 4
    assert (tmp_path / "lakes" / "index.json").exists()
    outlines = json.loads((lake_dir / "outlines.geojson").read_text())
    assert [f["properties"]["year"] for f in outlines["features"]] == [2018, 2019]
    # evidence: scene ids on the yearly rows that have them, not repeated in first/latest
    assert lake["years"][2]["scene_ids"] == ["S2A_1", "S2B_2"] and lake["years"][2]["scene_days"][0] == "2019-09-01"
    assert "scene_ids" not in lake["years"][1] and "scene_ids" not in lake["latest"]
    assert lake["drain"] is None


def test_build_index_carries_the_drain_check(tmp_path):
    lake_dir = tmp_path / "lakes" / "x"
    lake_dir.mkdir(parents=True)
    (lake_dir / "series.json").write_text(json.dumps({"lake": {"id": "x"}, "years": []}))
    drain = {
        "lake_id": "x",
        "as_of": "2026-10-09",
        "drained": True,
        "drop_fraction": 0.5,
        "status": "ok",
        "latest": {"day": "2026-10-08", "item_id": "S2A_1", "area_m2": 50.0, "coverage": 1.0},
    }
    (lake_dir / "drain.json").write_text(json.dumps(drain))
    assert build_index(tmp_path)["lakes"][0]["drain"] == {
        "as_of": "2026-10-09",
        "drained": True,
        "drop_fraction": 0.5,
        "status": "ok",
        "latest_day": "2026-10-08",
    }


def test_old_series_rows_load_without_scene_lists():
    from aahat.timeseries import LakeYear

    old = LakeYear("x", 2018, "ok", 100.0, 1.0, 40.0, 1.0, 5, 3, "2018-09-01", "2018-10-01", {})
    assert old.scene_ids == [] and old.scene_days == []
