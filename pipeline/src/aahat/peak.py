"""Peak outburst discharge from lake volume: empirical relations from the GLOF literature.

All are regressions on past dam-breach floods with order-of-magnitude scatter. We route two scenarios:
- expected: Evans 1986, which lands closest to NRSC's physically modelled (HEC-RAS dam breach) peak
  for Gepang Gath: 9,611-9,673 m3/s for 35.1 million m3, where Evans gives ~7,200 and Huggel ~36,300.
- severe: Huggel et al. 2002's regression (fitted to moraine-dam outbursts of 0.09-19 million m3,
  so it is extrapolated for larger lakes). Huggel's own worst-case envelope, 2V/1000 s, is higher still.
NRSC reports:
https://bhuvan.nrsc.gov.in/nhpfs/pdf/NRSC_GhepangGhatGlacialLake_GLOF_Risk_Assessment_Report.pdf
https://bhuvan.nrsc.gov.in/nhpfs/pdf/NRSC_SamudraTapuGlacialLake_GLOF_Risk_Assessment_Report.pdf
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Relation:
    key: str
    formula: str
    coef: float
    exp: float
    source: str
    note: str

    def __call__(self, volume_m3: float) -> float:
        return self.coef * volume_m3**self.exp


RELATIONS: dict[str, Relation] = {
    r.key: r
    for r in (
        Relation(
            "huggel2002",
            "Qmax = 0.00077 V^1.017",
            0.00077,
            1.017,
            "Huggel et al. 2002, Can. Geotech. J. 39:316-330 (https://doi.org/10.1139/t01-099)",
            "regression on moraine-dam outbursts of 0.09-19 million m³; extrapolated above that",
        ),
        Relation(
            "popov1991",
            "Qmax = 0.0048 V^0.896",
            0.0048,
            0.896,
            "Popov 1991, as tabulated in Huggel et al. 2002 (https://doi.org/10.1139/t01-099)",
            "",
        ),
        Relation(
            "evans1986",
            "Qmax = 0.72 V^0.53",
            0.72,
            0.53,
            "Evans 1986, as tabulated in Huggel et al. 2002 (https://doi.org/10.1139/t01-099)",
            "used as the expected case: of the relations, it came closest to NRSC's HEC-RAS dam-breach peak "
            "for the benchmark lake, Gepang Gath",
        ),
    )
}
SCENARIOS = {"expected": "evans1986", "severe": "huggel2002"}


def peak_discharge(volume_m3: float) -> dict:
    """Breach peak (m3/s) per scenario with its relation, plus every relation's value for context.

    For small lakes Evans exceeds Huggel; the severe scenario is whichever of the two is larger.
    """
    expected = RELATIONS[SCENARIOS["expected"]]
    severe = max((RELATIONS[k] for k in SCENARIOS.values()), key=lambda r: r(volume_m3))

    def describe(r: Relation) -> dict:
        return {"peak_m3s": round(r(volume_m3)), "relation": r.formula, "source": r.source, "note": r.note}

    return {
        "scenarios": {"expected": describe(expected), "severe": describe(severe)},
        "all": {k: round(r(volume_m3)) for k, r in RELATIONS.items()},
    }
