"""The catalogue of monitored lakes."""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from importlib import resources


@dataclass(frozen=True)
class Lake:
    id: str
    name: str
    name_hi: str
    lat: float
    lon: float
    aoi_radius_m: float
    district: str
    basin: str
    kind: str
    notes: str = ""
    sources: list[str] = field(default_factory=list)


def load_lakes() -> list[Lake]:
    raw = json.loads(resources.files("aahat.data").joinpath("lakes.json").read_text())
    return [Lake(**r) for r in raw]


def get_lake(lake_id: str) -> Lake:
    for lake in load_lakes():
        if lake.id == lake_id:
            return lake
    raise KeyError(f"unknown lake {lake_id!r}; known: {[l.id for l in load_lakes()]}")
