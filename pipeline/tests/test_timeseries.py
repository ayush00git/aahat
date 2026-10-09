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
            rec | {"year": 2019, "area_m2": 120.0},
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
