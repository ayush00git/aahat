"""Every settlement people might search for, so the app can answer "not covered" honestly.

The flood analysis only knows the places near monitored lakes' flood paths. Search should still find
any village in Himachal (and the neighbouring valleys its rivers run into) and say clearly when no
monitored lake threatens it. Built from the same OpenStreetMap extract as the exposure data.
"""

from __future__ import annotations

import json
from pathlib import Path

from .admin import assign, load_admin
from .exposure import PLACES
from .osm_extract import query

# Himachal Pradesh and the valleys around it (Chenab into J&K, Sutlej from Tibet, Ravi to Punjab).
REGION = (75.4, 30.2, 79.2, 33.6)


def build_places(out_dir: Path, bounds: tuple[float, float, float, float] = REGION) -> Path:
    """Write places/index.json: [{osm, name, name_hi, place, lon, lat, district, state}] for named
    settlements, by name. District and state are null outside the mapped boundaries (e.g. Tibet)."""
    # Boundaries first: building them from the raw extract (first run only) needs about 0.7 GB, which is
    # released again before the filtered extract (another 1.3 GB) is loaded. The server has 2 GB.
    areas = load_admin()
    rows = []
    seen = set()
    for el in query(bounds)["elements"]:
        tags = el.get("tags", {})
        if tags.get("place") not in PLACES or not tags.get("name"):
            continue
        if el["type"] == "node":
            lon, lat = el["lon"], el["lat"]
        else:  # an area mapped as a way: use the mean of its outline
            pts = el["geometry"]
            lon, lat = sum(p["lon"] for p in pts) / len(pts), sum(p["lat"] for p in pts) / len(pts)
        osm = f"{el['type']}/{el['id']}"
        if osm in seen:
            continue
        seen.add(osm)
        rows.append(
            {
                "osm": osm,
                "name": tags["name"],
                "name_hi": tags.get("name:hi"),
                "place": tags["place"],
                "lon": round(lon, 5),
                "lat": round(lat, 5),
            }
        )
    # Many names repeat (Hamirpur is a town in Himachal and a district in Uttar Pradesh): say where each is.
    for row, (district, state) in zip(rows, assign([(r["lon"], r["lat"]) for r in rows], areas)):
        row |= {"district": district, "state": state}
    rows.sort(key=lambda r: (r["name"].lower(), r["osm"]))
    path = out_dir / "places" / "index.json"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps({"bounds": bounds, "places": rows}, ensure_ascii=False, separators=(",", ":")))
    return path
