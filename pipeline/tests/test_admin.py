from shapely.geometry import box

from aahat.admin import AdminArea, assign

AREAS = [
    AdminArea("Himachal Pradesh", "हिमाचल प्रदेश", 4, None, box(75, 30, 79, 33)),
    AdminArea("Hamirpur", "हमीरपुर", 5, "Himachal Pradesh", box(76, 31, 77, 32)),
    AdminArea("Kangra", None, 5, "Himachal Pradesh", box(75, 31, 76, 33)),
    AdminArea("Enclave", None, 5, "Himachal Pradesh", box(75.4, 32.0, 75.6, 32.2)),  # mapped inside Kangra
    # the extract cuts Uttar Pradesh: its district is whole but the state has no polygon
    AdminArea("Hamirpur", None, 5, "Uttar Pradesh", box(79.5, 25, 80.5, 26)),
]


def test_assign_district_and_state_by_point_in_polygon():
    got = assign([(76.53, 31.68), (75.9, 32.2), (80.0, 25.5)], AREAS)
    assert got == [
        ("Hamirpur", "Himachal Pradesh"),
        ("Kangra", "Himachal Pradesh"),
        ("Hamirpur", "Uttar Pradesh"),  # state from the district when no state polygon contains it
    ]


def test_assign_outside_every_boundary_is_none_and_state_alone_is_kept():
    assert assign([(81.0, 31.5), (78.5, 30.5)], AREAS) == [(None, None), (None, "Himachal Pradesh")]


def test_assign_prefers_the_smallest_overlapping_district():
    assert assign([(75.5, 32.1)], AREAS) == [("Enclave", "Himachal Pradesh")]


def test_assign_handles_no_points_and_no_areas():
    assert assign([], AREAS) == []
    assert assign([(76.5, 31.7)], []) == [(None, None)]
