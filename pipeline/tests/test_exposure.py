import json

from aahat import exposure
from aahat.exposure import assets_geojson, classify, fetch_assets, overpass_query, parse_overpass, query_key


def pts(*lonlats):
    return [{"lon": lon, "lat": lat} for lon, lat in lonlats]


SQUARE = pts((77.10, 32.40), (77.11, 32.40), (77.11, 32.41), (77.10, 32.41), (77.10, 32.40))


def fake_overpass():
    """A small Overpass `out geom` response with one of each thing we care about, plus noise."""
    return {
        "elements": [
            {
                "type": "node",
                "id": 1,
                "lat": 32.48,
                "lon": 77.12,
                "tags": {"place": "village", "name": "Sissu", "name:hi": "सिस्सू", "population": "500", "foo": "x"},
            },
            {"type": "way", "id": 2, "geometry": SQUARE, "tags": {"place": "hamlet", "name": "Khoksar"}},
            {
                "type": "way",
                "id": 3,
                "geometry": pts((77.10, 32.45), (77.20, 32.47)),
                "tags": {"highway": "primary", "ref": "NH3"},
            },
            {
                "type": "way",
                "id": 4,
                "geometry": pts((77.15, 32.46), (77.151, 32.461)),
                "tags": {"highway": "primary", "bridge": "yes", "name": "Chandra bridge"},
            },
            {
                "type": "way",
                "id": 5,
                "geometry": pts((77.16, 32.46), (77.161, 32.461)),
                "tags": {"highway": "footway", "bridge": "yes"},
            },
            {
                "type": "way",
                "id": 6,
                "geometry": SQUARE,
                "tags": {"power": "plant", "plant:source": "hydro", "name": "Some Plant"},
            },
            {"type": "node", "id": 7, "lat": 32.5, "lon": 77.2, "tags": {"power": "plant", "name": "Seli HEP"}},
            {"type": "way", "id": 8, "geometry": SQUARE, "tags": {"waterway": "dam", "name": "Barrage"}},
            {"type": "node", "id": 9, "lat": 32.49, "lon": 77.13, "tags": {"amenity": "school", "name": "GPS Sissu"}},
            {
                "type": "relation",
                "id": 10,
                "tags": {"place": "town", "name": "Keylong"},
                "members": [
                    {"type": "way", "ref": 100, "role": "outer", "geometry": SQUARE[:3]},
                    {"type": "way", "ref": 101, "role": "outer", "geometry": SQUARE[2:]},
                    {"type": "node", "ref": 102, "role": "label", "lat": 32.405, "lon": 77.105},
                ],
            },
            {"type": "way", "id": 11, "tags": {"place": "village", "name": "Nowhere"}},  # no geometry
            {"type": "node", "id": 12, "tags": {"amenity": "clinic"}},  # no coordinates
            {"type": "node", "id": 13, "lat": 32.4, "lon": 77.1, "tags": {"shop": "bakery"}},  # not tracked
            {"type": "node", "id": 1, "lat": 32.48, "lon": 77.12, "tags": {"place": "village", "name": "Sissu"}},
        ]
    }


def by_osm():
    return {a.osm: a for a in parse_overpass(fake_overpass())}


def test_classification_and_geometry_types():
    a = by_osm()
    expect = {
        "node/1": ("settlement", "village", "Point"),
        "way/2": ("settlement", "hamlet", "Polygon"),
        "way/3": ("road", "primary", "LineString"),
        "way/4": ("bridge", "road_bridge", "LineString"),
        "way/5": ("bridge", "footbridge", "LineString"),
        "way/6": ("hydro", "hydro_plant", "Polygon"),
        "node/7": ("hydro", "hydro_plant", "Point"),
        "way/8": ("hydro", "dam", "Polygon"),
        "node/9": ("school", "school", "Point"),
        "relation/10": ("settlement", "town", "Polygon"),
    }
    assert set(a) == set(expect)
    for osm, (kind, subkind, geom) in expect.items():
        assert (a[osm].kind, a[osm].subkind, a[osm].geometry.geom_type) == (kind, subkind, geom), osm


def test_names_and_tag_subset():
    v = by_osm()["node/1"]
    assert v.name == "Sissu" and v.name_hi == "सिस्सू"
    assert v.tags == {"place": "village", "name": "Sissu", "name:hi": "सिस्सू", "population": "500"}


def test_dedup_and_bridge_not_also_a_road():
    assets = parse_overpass(fake_overpass())
    assert len(assets) == len({a.osm for a in assets})
    roads = [a for a in assets if a.kind == "road"]
    assert [a.osm for a in roads] == ["way/3"]


def test_missing_geometry_is_skipped():
    a = by_osm()
    assert "way/11" not in a and "node/12" not in a and "node/13" not in a


def test_relation_rings_assembled_from_split_outer_ways():
    rel = by_osm()["relation/10"].geometry
    assert abs(rel.area - 0.01 * 0.01) < 1e-9


def test_hydro_name_needs_whole_word():
    assert classify({"power": "plant", "name": "Malana-II H.E.P"}) == ("hydro", "hydro_plant")
    assert classify({"power": "plant", "name": "Allain Duhangan Hydro Project"}) == ("hydro", "hydro_plant")
    assert classify({"power": "plant", "name": "Shepherd Solar"}) is None
    assert classify({"highway": "primary", "bridge": "no"}) == ("road", "primary")


def test_geojson():
    fc = assets_geojson(parse_overpass(fake_overpass()))
    assert fc["type"] == "FeatureCollection" and len(fc["features"]) == 10
    f = next(f for f in fc["features"] if f["properties"]["osm"] == "node/1")
    assert f["geometry"]["type"] == "Point"
    assert f["properties"] == {
        "kind": "settlement",
        "subkind": "village",
        "name": "Sissu",
        "name_hi": "सिस्सू",
        "osm": "node/1",
    }
    json.dumps(fc)  # serialisable as-is


def test_fetch_reads_disk_cache_without_network(tmp_path, monkeypatch):
    bounds = (77.10, 32.40, 77.30, 32.55)
    monkeypatch.setattr(exposure, "cache_dir", lambda: tmp_path)

    def no_network(*args, **kwargs):
        raise AssertionError("network used")

    monkeypatch.setattr(exposure, "_post_overpass", no_network)
    monkeypatch.setattr(exposure.urllib.request, "urlopen", no_network)
    path = tmp_path / "osm" / f"{query_key(overpass_query(bounds))}.json"
    path.parent.mkdir(parents=True)
    path.write_text(json.dumps(fake_overpass()))
    assets = fetch_assets(bounds)
    assert len(assets) == 10
