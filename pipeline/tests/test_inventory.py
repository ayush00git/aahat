from aahat.inventory import FoundLake, dedupe


def test_dedupe_keeps_one_per_lake_and_the_larger_outline():
    a = FoundLake(32.5, 77.2, 1.0, 4000, "w1")
    b = FoundLake(32.5005, 77.2005, 0.9, 4000, "w2")  # same lake seen from the next window
    c = FoundLake(32.6, 77.3, 0.2, 4500, "w2")
    kept = dedupe([b, c, a])
    assert [k.area_km2 for k in kept] == [1.0, 0.2]
