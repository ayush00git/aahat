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


def _lake_with_downstream(tmp_path, discharge, risk_latest):
    lake_dir = tmp_path / "lakes" / "x"
    lake_dir.mkdir(parents=True)
    (lake_dir / "series.json").write_text(json.dumps({"lake": {"id": "x"}, "years": []}))
    (lake_dir / "downstream.json").write_text(json.dumps({"path_km": 12.0, "discharge": discharge}))
    (lake_dir / "impacts.json").write_text("[]")
    (lake_dir / "risk.json").write_text(json.dumps({"latest": risk_latest}))
    return build_index(tmp_path)["lakes"][0]


RISK_LATEST = {"as_of_season": 2026, "data_until": "2026-10-01", "score": 0.0, "level": "low", "volume_m3": 24100}


def test_index_names_both_scenario_peaks_and_flags_identical_scenarios(tmp_path):
    from aahat.peak import peak_discharge

    risk = RISK_LATEST | {
        "peak_discharge_m3s": 151,
        "peak_discharge_relation": "evans1986: Qmax = 0.72 V^0.53",
        "peak_expected_m3s": 151,
        "peak_severe_m3s": 151,
    }
    lake = _lake_with_downstream(tmp_path, peak_discharge(24_100), risk)
    d = lake["downstream"]
    assert d["peak_expected_m3s"] == d["peak_severe_m3s"] == 151
    assert d["scenarios_identical"] is True
    assert d["peak_m3s"] == {"expected": 151, "severe": 151}  # deprecated alias, same numbers
    assert d["peak_relations"]["severe"] == "Qmax = 0.72 V^0.53"
    # one lake, one severe peak: the risk summary and the downstream summary agree and say which formula
    assert lake["risk"]["peak_discharge_m3s"] == d["peak_severe_m3s"]
    assert lake["risk"]["peak_discharge_relation"].startswith("evans1986")


def test_index_reads_downstream_and_risk_files_written_before_the_rename(tmp_path):
    old = {"scenarios": {"expected": {"peak_m3s": 7566}, "severe": {"peak_m3s": 40113}}}
    lake = _lake_with_downstream(tmp_path, old, RISK_LATEST | {"peak_discharge_m3s": 40113})
    d = lake["downstream"]
    assert (d["peak_expected_m3s"], d["peak_severe_m3s"], d["scenarios_identical"]) == (7566, 40113, False)
    assert lake["risk"]["peak_discharge_m3s"] == 40113 and lake["risk"]["peak_discharge_relation"] is None
