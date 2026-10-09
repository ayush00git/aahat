import math

from aahat.laketerrain import LakeTerrain
from aahat.risk import Rule, growth_as_of, huggel_volume_m3, level_for, score_as_of


def terrain(slope=5.0, avalanche_km2=0.4):
    return LakeTerrain(4000, 77.2, 32.5, 1000, 87, slope, True, avalanche_km2, [], {})


def years(areas):
    return [
        {"year": y, "status": "ok", "area_m2": a, "uncertainty_m2": 0.01 * a, "last_day": f"{y}-10-20"}
        for y, a in areas.items()
    ]


def test_huggel_volume_matches_formula():
    assert math.isclose(huggel_volume_m3(1e6), 0.104 * 1e6**1.42)


def test_rule_is_linear_and_clamped():
    r = Rule(0, 10)
    assert r.score(-1) == 0 and r.score(5) == 0.5 and r.score(20) == 1
    assert Rule(5, 8, log10=True).score(10**6.5) == 0.5
    assert r.score(None) == 0


def test_growth_needs_three_full_seasons_and_ignores_partial_ones():
    ys = years({2017: 1.0e6, 2018: 1.03e6})
    assert growth_as_of(ys, 2018).pct_per_yr is None
    ys += [{"year": 2019, "status": "partial", "area_m2": 0.5e6}, {"year": 2020, "status": "ok", "area_m2": 1.09e6}]
    g = growth_as_of(ys, 2020)
    assert 2.5 < g.pct_per_yr < 3.5  # the partial 0.5 km2 season is ignored
    assert g.significant


def test_growth_within_measurement_noise_does_not_count():
    ys = years({2017: 0.219e6, 2018: 0.226e6, 2019: 0.236e6, 2020: 0.222e6, 2021: 0.247e6, 2022: 0.226e6})
    for r in ys:
        r["uncertainty_m2"] = 0.021e6
    g = growth_as_of(ys, 2022)
    assert not g.significant
    rec = score_as_of("x", ys, lambda y: terrain(), 2022)
    assert next(f for f in rec.factors if f.key == "growth").score == 0


def test_replay_never_sees_the_future():
    ys = years({2017: 1.0e6, 2018: 1.02e6, 2019: 1.04e6, 2020: 9.0e6})  # 2020: a huge jump
    terrain_for = lambda year: terrain()
    before = score_as_of("x", ys[:3], terrain_for, 2019)
    with_future = score_as_of("x", ys, terrain_for, 2019)
    assert before.score == with_future.score
    assert with_future.area_year == 2019 and with_future.data_until == "2019-10-20"


def test_levels():
    assert level_for(10) == "low" and level_for(30) == "moderate" and level_for(50) == "high" and level_for(70) == "very_high"


def test_score_is_size_times_likelihood_and_group_weights_sum_to_one():
    from aahat.risk import FACTORS

    for group in ("size", "likelihood"):
        assert math.isclose(sum(f.weight for f in FACTORS if f.group == group), 1.0)
    rec = score_as_of("x", years({2017: 1.0e6, 2018: 1.02e6, 2019: 1.04e6}), lambda y: terrain(), 2019)
    assert math.isclose(rec.score, 100 * rec.size * rec.likelihood, abs_tol=0.2)


def test_small_lake_in_steep_terrain_scores_below_large_growing_lake():
    small = score_as_of("s", years({2017: 0.09e6, 2018: 0.09e6, 2019: 0.09e6}), lambda y: terrain(9, 0.6), 2019)
    large = score_as_of("l", years({2017: 0.86e6, 2018: 0.88e6, 2019: 0.90e6}), lambda y: terrain(4, 0.5), 2019)
    assert large.score > small.score


def test_glacier_rule_scores_contact_highest_and_none_as_zero():
    r = Rule(500.0, 0.0)
    assert r.score(0) == 1 and r.score(250) == 0.5 and r.score(800) == 0
    ys = years({2017: 1.0e6, 2018: 1.02e6, 2019: 1.04e6})
    rec = score_as_of("x", ys, lambda y: terrain(), 2019)
    g = next(f for f in rec.factors if f.key == "glacier")
    assert g.score == 0 and "no inventoried glacier" in g.note
    from aahat.glaciers import GlacierProximity

    rec = score_as_of("x", ys, lambda y: terrain(), 2019, lambda y: GlacierProximity(0.0, "RGI-x", 1.2, "2002-08-02"))
    assert next(f for f in rec.factors if f.key == "glacier").score == 1
