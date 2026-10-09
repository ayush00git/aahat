"""Flow routing on a DEM: outlet finding and a steepest-descent path that escapes pits.

The Copernicus DEM has noise, bridges and flattened lake surfaces, so a pure D8 walk stalls in
small depressions. When the walk reaches a cell with no lower unvisited neighbour, a local
priority flood (lowest frontier cell first) searches outward until it finds a cell lower than
the pit, and the path continues through the cells that flood passed. This needs no global
depression filling, so it stays cheap on large DEMs.
"""

from __future__ import annotations

import heapq
import math

import numpy as np
from scipy import ndimage

# 8-neighbour offsets (row, col) and their step lengths in pixels.
NEIGHBOURS = [(-1, -1), (-1, 0), (-1, 1), (0, -1), (0, 1), (1, -1), (1, 0), (1, 1)]
STEP = [math.hypot(dr, dc) for dr, dc in NEIGHBOURS]


def outlet_cell(dem: np.ndarray, lake: np.ndarray) -> tuple[int, int]:
    """The lowest cell on the ring just outside the lake: where it spills."""
    ring = ndimage.binary_dilation(lake, structure=np.ones((3, 3))) & ~lake
    z = np.where(ring & np.isfinite(dem), dem, np.inf)
    r, c = np.unravel_index(int(np.argmin(z)), z.shape)
    return int(r), int(c)


def water_level(dem: np.ndarray, lake: np.ndarray) -> float:
    """The DEM's flattened water surface: the most common 1 m elevation bin inside the outline.

    Robust to an outline that also covers newer water over old glacier surface (higher) or
    spills past the DEM lake edge (lower).
    """
    z = dem[lake & np.isfinite(dem)]
    if z.size == 0:
        return float("nan")
    bins = np.floor(z).astype(int)
    values, counts = np.unique(bins, return_counts=True)
    mode = values[np.argmax(counts)]
    return float(np.median(z[bins == mode]))


def spill_mask(dem: np.ndarray, lake: np.ndarray, res: float, tol_m: float = 1.5, max_slope_deg: float = 2.0) -> np.ndarray:
    """Grow a lake mask over the DEM's flat water surface that touches it.

    DEMs flatten lakes, and an outline from a newer image need not line up with that surface.
    Cells connected to the lake, within `tol_m` of its water level and flat (so not a contour line
    along a hillside) are part of the surface; the true spill point lies on the ring around it.
    """
    level = water_level(dem, lake)
    at_level = np.isfinite(dem) & (np.abs(dem - level) <= tol_m)
    dzdy, dzdx = np.gradient(dem, res)
    core = at_level & (np.degrees(np.arctan(np.hypot(dzdx, dzdy))) < max_slope_deg)
    # the surface's edge cells have a steep central gradient (it mixes in the bank): add them back
    flat = core | (ndimage.binary_dilation(core, structure=np.ones((3, 3))) & at_level)
    labels, _ = ndimage.label(lake | flat, structure=np.ones((3, 3)))
    return np.isin(labels, np.unique(labels[lake]))


def _escape_pit(dem, start, visited, max_cells):
    """Priority flood from `start` until a cell lower than dem[start]; return the cells to walk."""
    h, w = dem.shape
    level = dem[start]
    parent = {start: None}
    heap = [(level, start)]
    seen = {start}
    while heap and len(seen) < max_cells:
        _, cell = heapq.heappop(heap)
        r, c = cell
        for dr, dc in NEIGHBOURS:
            nr, nc = r + dr, c + dc
            if not (0 <= nr < h and 0 <= nc < w) or (nr, nc) in seen or visited[nr, nc]:
                continue
            z = dem[nr, nc]
            if not np.isfinite(z):
                continue
            seen.add((nr, nc))
            parent[(nr, nc)] = cell
            on_edge = nr in (0, h - 1) or nc in (0, w - 1)
            if z < level or on_edge:  # found the way out (lower ground, or off the window): rebuild the route
                route = [(nr, nc)]
                while parent[route[-1]] != start:
                    route.append(parent[route[-1]])
                return route[::-1]
            heapq.heappush(heap, (z, (nr, nc)))
    return None


def flow_path(
    dem: np.ndarray,
    start: tuple[int, int],
    res: float,
    max_length_m: float = float("inf"),
    blocked: np.ndarray | None = None,
    max_pit_cells: int = 50_000,
) -> list[tuple[int, int]]:
    """Cells from `start` downhill until the grid edge, `max_length_m`, or an inescapable pit.

    `blocked` cells (e.g. the lake itself) are never entered.
    """
    h, w = dem.shape
    visited = np.zeros(dem.shape, bool) if blocked is None else blocked.copy()
    path = [start]
    visited[start] = True
    length = 0.0
    while length < max_length_m:
        r, c = path[-1]
        if not (0 < r < h - 1 and 0 < c < w - 1):
            return path  # reached the edge of the DEM window
        best, best_gradient = None, 0.0
        for (dr, dc), step in zip(NEIGHBOURS, STEP):
            nr, nc = r + dr, c + dc
            z = dem[nr, nc]
            if visited[nr, nc] or not np.isfinite(z):
                continue
            gradient = (dem[r, c] - z) / step  # steepest descent: largest drop per unit distance
            if gradient > best_gradient:
                best, best_gradient = (nr, nc), gradient
        route = [best] if best is not None else _escape_pit(dem, (r, c), visited, max_pit_cells)
        if not route:
            return path
        for cell in route:
            prev = path[-1]
            length += res * math.hypot(cell[0] - prev[0], cell[1] - prev[1])
            visited[cell] = True
            path.append(cell)
    return path


def path_length_m(path: list[tuple[int, int]], res: float) -> np.ndarray:
    """Cumulative along-path distance (m) at each cell."""
    if len(path) < 2:
        return np.zeros(len(path))
    p = np.array(path, dtype=float)
    seg = np.hypot(*(np.diff(p, axis=0).T)) * res
    return np.concatenate([[0.0], np.cumsum(seg)])
