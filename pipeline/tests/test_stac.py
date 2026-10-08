from datetime import datetime

from pystac import Asset, Item
from shapely.geometry import box, mapping

from aahat.stac import Scene, one_per_day


def make_item(item_id, day, baseline, applied, cloud=10.0, footprint=(77, 32, 78, 33)):
    item = Item(
        id=item_id,
        geometry=mapping(box(*footprint)),
        bbox=list(footprint),
        datetime=datetime.fromisoformat(f"{day}T05:40:00+00:00"),
        properties={
            "eo:cloud_cover": cloud,
            "s2:processing_baseline": baseline,
            "earthsearch:boa_offset_applied": applied,
        },
    )
    for key in ("green", "nir", "swir16", "scl"):
        bands = [{"scale": 0.0001, "offset": -0.1}] if key != "scl" else [{}]
        item.add_asset(key, Asset(href=f"https://example/{item_id}/{key}.tif", extra_fields={"raster:bands": bands}))
    return item


def test_offset_not_applied_twice_when_earth_search_already_applied_it():
    s = Scene.from_item(make_item("a", "2025-09-20", "05.11", applied=True))
    assert s.offset["green"] == 0.0
    assert s.scale["green"] == 0.0001


def test_offset_kept_when_not_yet_applied():
    s = Scene.from_item(make_item("b", "2025-09-20", "05.11", applied=False))
    assert s.offset["green"] == -0.1


def test_one_per_day_prefers_newest_baseline_and_full_coverage():
    aoi = box(77.2, 32.4, 77.3, 32.5)
    items = [
        make_item("old", "2019-09-17", "02.13", applied=False, cloud=5),
        make_item("new", "2019-09-17", "05.00", applied=True, cloud=9),
        make_item("partial", "2019-09-22", "05.00", applied=True, footprint=(77.25, 32, 78, 33)),
    ]
    scenes = one_per_day(items, aoi)
    assert [s.item_id for s in scenes] == ["new"]
