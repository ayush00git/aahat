import numpy as np

from aahat.hydro import flow_path, outlet_cell, path_length_m


def valley(h=60, w=60):
    """A valley sloping down to the east (col increases), V-shaped across rows."""
    rows, cols = np.mgrid[0:h, 0:w]
    return (1000.0 - 5.0 * cols + 3.0 * np.abs(rows - h // 2)).astype("float32")


def test_flow_path_follows_valley_floor_to_the_edge():
    dem = valley()
    path = flow_path(dem, (30, 2), res=30.0)
    rows = [r for r, _ in path]
    assert path[-1][1] == dem.shape[1] - 1  # left the window on the east side
    assert max(abs(r - 30) for r in rows) <= 1  # stayed on the valley floor


def test_flow_path_escapes_a_pit():
    dem = valley()
    dem[30, 20] -= 40  # a pit on the valley floor (e.g. DEM noise)
    dem[30, 21:24] += 10  # and a small dam below it
    path = flow_path(dem, (30, 2), res=30.0)
    assert path[-1][1] == dem.shape[1] - 1
    assert len(set(path)) == len(path)  # never revisits a cell


def test_flow_path_respects_blocked_cells_and_max_length():
    dem = valley()
    blocked = np.zeros(dem.shape, bool)
    blocked[25:36, 10:15] = True  # the lake
    path = flow_path(dem, (30, 16), res=30.0, max_length_m=300, blocked=blocked)
    assert not any(blocked[r, c] for r, c in path[1:])
    assert path_length_m(path, 30.0)[-1] <= 300 + 30 * np.sqrt(2)


def test_outlet_is_lowest_ring_cell():
    dem = valley()
    lake = np.zeros(dem.shape, bool)
    lake[25:36, 10:20] = True
    dem[lake] = dem[30, 20] + 1  # flat lake surface
    r, c = outlet_cell(dem, lake)
    assert (r, c) == (30, 20)  # downstream end, on the valley axis


def test_spill_mask_grows_over_flat_surface_without_creeping_downstream():
    from aahat.hydro import spill_mask, water_level

    dem = valley()
    surface = dem[30, 25]
    old_lake = np.zeros(dem.shape, bool)
    old_lake[24:37, 10:26] = True
    dem[old_lake] = surface  # DEM's flattened lake (older, bigger extent)
    outline = np.zeros(dem.shape, bool)
    outline[27:34, 12:20] = True  # newer outline, inside the flat surface
    assert water_level(dem, outline) == surface
    grown = spill_mask(dem, outline, res=30.0)
    assert grown[old_lake].all()
    assert not grown[:, 27:].any()  # did not creep down the valley
    assert outlet_cell(dem, grown)[1] in (26, 27)  # first cell past the surface edge


def test_drains_to_excludes_the_next_valley():
    from aahat.hydro import drains_to

    rows, cols = np.mgrid[0:40, 0:40]
    dem = (100.0 + 10.0 * np.abs(cols - 20)).astype("float32")  # ridge-less V valley along rows, axis at col 20
    dem += 1.0 * rows  # sloping down toward row 0
    dem[:, 35:] = 100.0 + 10.0 * (40 - cols[:, 35:])  # past a ridge at col 35, slopes fall away east
    lake = np.zeros(dem.shape, bool)
    lake[0:3, 18:23] = True
    c = drains_to(dem, lake)
    assert c[30, 20] and c[20, 10]  # valley floor and its west flank drain to the lake
    assert not c[20, 38]  # beyond the ridge
