"""Command line entry point: `aahat <command>`."""

from __future__ import annotations

import argparse
import json
import logging
from pathlib import Path

import numpy as np
from scipy import ndimage
from shapely.geometry import Point

from .dem import Terrain
from .geo import grid_around, to_wgs84
from .lakes import get_lake, load_lakes
from .timeseries import FIRST_YEAR, build_index, run_lake, season
from .water import WaterParams, composite


def parse_years(spec: str) -> list[int]:
    if "-" in spec:
        a, b = spec.split("-")
        return list(range(int(a), int(b) + 1))
    return [int(y) for y in spec.split(",")]


def cmd_series(args) -> None:
    lakes = load_lakes() if args.lake == "all" else [get_lake(args.lake)]
    for lake in lakes:
        recs = run_lake(
            lake, parse_years(args.years), Path(args.out), max_scenes=args.max_scenes, quicklook=args.quicklook
        )
        print(f"\n{lake.name} ({lake.id})")
        print(f"{'year':>6} {'status':>10} {'area km2':>10} {'+/-':>8} {'cover':>6} {'scenes':>7}")
        for r in recs:
            area = f"{r.area_m2 / 1e6:.4f}" if r.area_m2 else "-"
            unc = f"{r.uncertainty_m2 / 1e6:.4f}" if r.uncertainty_m2 else "-"
            cov = f"{r.coverage:.0%}" if r.coverage is not None else "-"
            print(f"{r.year:>6} {r.status:>10} {area:>10} {unc:>8} {cov:>6} {r.scenes_clear:>3}/{r.scenes_found:<3}")


def cmd_risk(args) -> None:
    from .risk import run_risk

    lakes = load_lakes() if args.lake == "all" else [get_lake(args.lake)]
    for lake in lakes:
        out = run_risk(Path(args.out) / "lakes" / lake.id)
        print(f"\n{lake.name}: replay (season: score = 100 x size x likelihood, level)")
        for r in out["replay"]:
            parts = ", ".join(f"{f['key']} {f['score']:.2f}" for f in r["factors"])
            print(
                f"  {r['as_of_season']}  {r['score']:5.1f} = {r['size']:.2f} x {r['likelihood']:.2f}  {r['level']:<9}  [{parts}]"
            )
    build_index(Path(args.out))


def cmd_downstream(args) -> None:
    from .impact import run_downstream

    lakes = load_lakes() if args.lake == "all" else [get_lake(args.lake)]
    for lake in lakes:
        out = run_downstream(Path(args.out) / "lakes" / lake.id)
        sc = out["discharge"]["scenarios"]
        print(
            f"\n{lake.name}: breach peak {sc['expected']['peak_m3s']:,} (expected) / {sc['severe']['peak_m3s']:,} m3/s"
            f" (severe), path {out['path_km']:.0f} km"
        )
        for r in out["impacts"]:
            if r["status"] != "outside" and r["kind"] != "road":
                name = r["name"] or f"unnamed {r['subkind']}"
                print(
                    f"  {r['km']:6.1f} km  {r['arrival_min_fast']:4.0f}-{r['arrival_min_expected']:4.0f} min"
                    f"  {r['status']:13} {r['kind']:<10} {name}"
                )
    build_index(Path(args.out))


def cmd_barrier(args) -> None:
    from datetime import UTC, date, datetime

    from shapely.geometry import LineString, shape

    from .barrier import candidates_json, scan_reach

    as_of = date.fromisoformat(args.as_of) if args.as_of else datetime.now(UTC).date()
    if args.reach:
        reach = LineString([tuple(map(float, pt.split(","))) for pt in args.reach.split(";")])
        name = args.name or "reach"
    else:
        path = Path(args.out) / "lakes" / args.lake / "flood_path.geojson"
        reach = shape(json.loads(path.read_text())["geometry"])
        name = args.name or args.lake
    cands = scan_reach(reach, as_of, max_km=args.max_km)
    out_dir = Path(args.out) / "barrier"
    out_dir.mkdir(parents=True, exist_ok=True)
    result = {"reach": name, "as_of": as_of.isoformat(), "candidates": candidates_json(cands)}
    (out_dir / f"{name}_{as_of.isoformat()}.json").write_text(json.dumps(result, indent=1))
    print(f"{name} as of {as_of}: {len(cands)} new-water candidate(s)")
    for c in cands:
        print(f"  km {c.km_along_reach:6.1f}  {c.lat:.4f},{c.lon:.4f}  {c.area_m2 / 1e6:.3f} km2  width {c.width_m} m")


def cmd_summary(args) -> None:
    index = build_index(Path(args.out))
    years = sorted({r["year"] for lake in index["lakes"] for r in lake["years"]})
    print(f"{'lake':<16}" + "".join(f"{y:>8}" for y in years) + "   (km2; * partial, - not found/no data)")
    for lake in index["lakes"]:
        by_year = {r["year"]: r for r in lake["years"]}
        cells = []
        for y in years:
            r = by_year.get(y)
            if r is None or r["area_m2"] is None:
                cells.append(f"{'-':>8}")
            else:
                cells.append(f"{r['area_m2'] / 1e6:>7.3f}" + ("*" if r["status"] == "partial" else " "))
        print(f"{lake['id']:<16}" + "".join(cells))


def cmd_discover(args) -> None:
    """List water bodies around a point, to place catalogue seed points from imagery."""
    from .stac import find_scenes

    grid = grid_around(args.lon, args.lat, args.radius)
    terrain = Terrain(grid)
    start, end = season(args.year)
    scenes = find_scenes(to_wgs84(grid.polygon(), grid.crs), start, end)
    comp = composite(scenes, grid, terrain, WaterParams(), max_scenes=args.max_scenes)
    labels, n = ndimage.label(comp.water, structure=np.ones((3, 3)))
    if n == 0:
        print("no water found")
        return
    idx = np.arange(1, n + 1)
    sizes = ndimage.sum_labels(np.ones_like(labels), labels, idx) * grid.pixel_area_m2()
    centers = ndimage.center_of_mass(comp.water, labels, idx)
    rows = []
    for i, area, (r, c) in zip(idx, sizes, centers):
        if area < args.min_area:
            continue
        x, y = grid.transform @ (c + 0.5, r + 0.5)
        pt = to_wgs84(Point(x, y), grid.crs)
        elev = float(np.nanmedian(terrain.dem[labels == i]))
        rows.append(
            {"lat": round(pt.y, 5), "lon": round(pt.x, 5), "area_km2": round(area / 1e6, 4), "elev_m": round(elev)}
        )
    rows.sort(key=lambda r: -r["area_km2"])
    print(json.dumps(rows[: args.top], indent=1))
    if args.quicklook:
        from .quicklook import save_quicklook

        save_quicklook(comp, None, Path(args.quicklook), title=f"discover {args.lat},{args.lon} {args.year}")


def main(argv: list[str] | None = None) -> None:
    ap = argparse.ArgumentParser(prog="aahat")
    ap.add_argument("-v", "--verbose", action="store_true")
    sub = ap.add_subparsers(dest="cmd", required=True)

    s = sub.add_parser("series", help="yearly post-monsoon area series for a lake")
    s.add_argument("--lake", required=True, help="lake id from the catalogue, or 'all'")
    s.add_argument("--years", default=f"{FIRST_YEAR}-2026")
    s.add_argument("--out", default="out")
    s.add_argument("--max-scenes", type=int, default=12)
    s.add_argument("--quicklook", action="store_true")
    s.set_defaults(func=cmd_series)

    k = sub.add_parser("risk", help="hazard score replay for a lake (needs its series first)")
    k.add_argument("--lake", required=True, help="lake id from the catalogue, or 'all'")
    k.add_argument("--out", default="out")
    k.set_defaults(func=cmd_risk)

    w = sub.add_parser("downstream", help="flood path, corridor, arrival times and exposed assets (needs risk first)")
    w.add_argument("--lake", required=True, help="lake id from the catalogue, or 'all'")
    w.add_argument("--out", default="out")
    w.set_defaults(func=cmd_downstream)

    b = sub.add_parser("barrier", help="scan a river reach for new barrier (landslide-dammed) lakes")
    b.add_argument("--lake", help="scan this lake's downstream flood path")
    b.add_argument("--reach", help="or a reach as 'lon,lat;lon,lat;...'")
    b.add_argument("--name", help="name for the output file")
    b.add_argument("--as-of", help="YYYY-MM-DD (default today); only scenes up to this date are used")
    b.add_argument("--max-km", type=float, default=None)
    b.add_argument("--out", default="out")
    b.set_defaults(func=cmd_barrier)

    m = sub.add_parser("summary", help="table of all lake series; writes lakes/index.json")
    m.add_argument("--out", default="out")
    m.set_defaults(func=cmd_summary)

    d = sub.add_parser("discover", help="list water bodies around a point")
    d.add_argument("--lat", type=float, required=True)
    d.add_argument("--lon", type=float, required=True)
    d.add_argument("--radius", type=float, default=5000)
    d.add_argument("--year", type=int, default=2025)
    d.add_argument("--max-scenes", type=int, default=8)
    d.add_argument("--min-area", type=float, default=10000, help="m2")
    d.add_argument("--top", type=int, default=15)
    d.add_argument("--quicklook", help="write a PNG here")
    d.set_defaults(func=cmd_discover)

    args = ap.parse_args(argv)
    logging.basicConfig(
        level=logging.INFO if args.verbose else logging.WARNING, format="%(asctime)s %(name)s %(message)s"
    )
    args.func(args)


if __name__ == "__main__":
    main()
