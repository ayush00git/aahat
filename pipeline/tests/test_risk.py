import math

from aahat.laketerrain import LakeTerrain
from aahat.risk import Rule, growth_as_of, huggel_volume_m3, level_for, score_as_of


def terrain(slope=5.0, share=0.4):
    return LakeTerrain(4000, 77.2, 32.5, 1000, 87, slope, True, 1.0, share, [], {})


def years(areas):
    return [{"year": y, "status": "ok", "area_m2": a, "last_day": f"{y}-10-20"} for y, a in areas.items()]


def test_huggel_volume_matches_formula():
    assert math.isclose(huggel_volume_m3(1e6), 0.104 * 1e6**1.42)


def test_rule_is_linear_and_clamped():
    r = Rule(0, 10)
    assert r.score(-1) == 0 and r.score(5) == 0.5 and r.score(20) == 1
    assert Rule(5, 8, log10=True).score(10**6.5) == 0.5
    assert r.score(None) == 0


def test_growth_needs_three_full_seasons_and_ignores_partial_ones():
    ys = years({2017: 1.0e6, 2018: 1.03e6})
    assert growth_as_of(ys, 2018)[0] is None
    ys += [{"year": 2019, "status": "partial", "area_m2": 0.5e6}, {"year": 2020, "status": "ok", "area_m2": 1.09e6}]
    pct, _ = growth_as_of(ys, 2020)
    assert 2.5 < pct < 3.5  # the partial 0.5 km2 season is ignored


def test_replay_never_sees_the_future():
    ys = years({2017: 1.0e6, 2018: 1.02e6, 2019: 1.04e6, 2020: 9.0e6})  # 2020: a huge jump
    terrain_for = lambda year: terrain()
    before = score_as_of("x", ys[:3], terrain_for, 2019)
    with_future = score_as_of("x", ys, terrain_for, 2019)
    assert before.score == with_future.score
    assert with_future.area_year == 2019 and with_future.data_until == "2019-10-20"


def test_levels():
    assert level_for(10) == "low" and level_for(30) == "moderate" and level_for(60) == "high" and level_for(80) == "very_high"


def test_contributions_add_up_and_weights_sum_to_one():
    from aahat.risk import FACTORS

    assert math.isclose(sum(f.weight for f in FACTORS), 1.0)
    rec = score_as_of("x", years({2017: 1.0e6, 2018: 1.02e6, 2019: 1.04e6}), lambda y: terrain(), 2019)
    assert math.isclose(rec.score, sum(f.contribution for f in rec.factors), abs_tol=0.2)
