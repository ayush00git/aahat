"""PNG quicklooks for eyeballing the water mapping (dev tool; needs matplotlib)."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from .water import Composite, LakeExtent


def _rgb(comp: Composite) -> np.ndarray | None:
    from rasterio.enums import Resampling

    from .raster import read_to_grid

    clearest = max(comp.scenes, key=lambda o: o.clear_fraction, default=None)
    if clearest is None:
        return None
    scene = next(s for s in comp.inputs if s.item_id == clearest.item_id)
    if not all(k in scene.hrefs for k in ("red", "green", "blue")):
        return None
    bands = [
        read_to_grid(scene.hrefs[k], comp.grid, Resampling.bilinear, nodata=0) * scene.scale[k] + scene.offset[k]
        for k in ("red", "green", "blue")
    ]
    rgb = np.dstack(bands)
    return np.clip(rgb / 0.35, 0, 1) ** 0.8


def save_quicklook(comp: Composite, ext: LakeExtent | None, path: Path, title: str = "") -> None:
    import matplotlib

    matplotlib.use("Agg")
    import matplotlib.pyplot as plt

    left, bottom, right, top = comp.grid.bounds
    extent = (left, right, bottom, top)
    rgb = _rgb(comp)
    fig, axes = plt.subplots(1, 3 if rgb is not None else 2, figsize=(15 if rgb is not None else 10, 5))
    ax = axes[0]
    im = ax.imshow(comp.water_freq, extent=extent, vmin=0, vmax=1, cmap="Blues")
    fig.colorbar(im, ax=ax, fraction=0.046, label="water frequency")
    ax.set_title(f"{title}: water frequency")
    ax = axes[1]
    im = ax.imshow(comp.obs_count, extent=extent, cmap="viridis")
    fig.colorbar(im, ax=ax, fraction=0.046, label="clear observations")
    ax.set_title(f"{len(comp.scenes)} scenes")
    if rgb is not None:
        axes[2].imshow(rgb, extent=extent)
        axes[2].set_title("clearest scene")
    if ext is not None:
        for ax in axes:
            for geom in getattr(ext.polygon, "geoms", [ext.polygon]):
                x, y = geom.exterior.xy
                ax.plot(x, y, color="red", lw=1)
        fig.suptitle(f"{title}: {ext.area_m2 / 1e6:.3f} km² ± {ext.uncertainty_m2 / 1e6:.3f}, coverage {ext.coverage:.0%}")
    for ax in axes:
        ax.set_xticks([])
        ax.set_yticks([])
    fig.tight_layout()
    fig.savefig(path, dpi=110)
    plt.close(fig)
