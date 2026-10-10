import json

from shapely.geometry import box

from aahat import places
from aahat.admin import AdminArea

AREAS = [
    AdminArea("Himachal Pradesh", None, 4, None, box(76, 31, 77, 31.9)),
    AdminArea("Hamirpur", None, 5, "Himachal Pradesh", box(76.4, 31.6, 76.6, 31.9)),
]


def test_build_places_keeps_named_settlements_sorted(tmp_path, monkeypatch):
    elements = [
        {"type": "node", "id": 2, "lat": 31.68, "lon": 76.53, "tags": {"place": "town", "name": "Hamirpur"}},
        {
            "type": "node",
            "id": 1,
            "lat": 31.70,
            "lon": 76.52,
            "tags": {"place": "village", "name": "Anu", "name:hi": "अणु"},
        },
        {"type": "node", "id": 3, "lat": 31.7, "lon": 76.5, "tags": {"place": "village"}},  # unnamed: skipped
        {"type": "node", "id": 4, "lat": 31.7, "lon": 76.5, "tags": {"highway": "trunk", "name": "NH3"}},  # not a place
        {
            "type": "way",
            "id": 5,
            "tags": {"place": "hamlet", "name": "Badi"},
            "geometry": [{"lat": 32.0, "lon": 77.0}, {"lat": 32.2, "lon": 77.2}],
        },
    ]
    monkeypatch.setattr(places, "query", lambda bounds: {"elements": elements})
    monkeypatch.setattr(places, "load_admin", lambda: AREAS)
    rows = json.loads(places.build_places(tmp_path).read_text())["places"]
    assert [r["name"] for r in rows] == ["Anu", "Badi", "Hamirpur"]
    assert rows[0]["name_hi"] == "अणु" and rows[1]["osm"] == "way/5" and rows[1]["lat"] == 32.1
    by_name = {r["name"]: r for r in rows}
    assert (by_name["Hamirpur"]["district"], by_name["Hamirpur"]["state"]) == ("Hamirpur", "Himachal Pradesh")
    assert (by_name["Badi"]["district"], by_name["Badi"]["state"]) == (None, None)  # outside every boundary
