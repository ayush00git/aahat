# Aahat methods

How every number in Aahat is made, in full. The [README](../README.md#how-the-numbers-are-made) has the
short version; the HTTP routes that serve these results are in [API.md](../API.md).

All downstream and risk numbers are screening estimates, not engineering hydraulics.

- [Pipeline commands and outputs](#pipeline-commands-and-outputs)
- [How lake mapping works](#how-lake-mapping-works)
- [How the hazard score works](#how-the-hazard-score-works)
- [How the downstream estimate works](#how-the-downstream-estimate-works)
- [Check against NRSC's HEC-RAS modelling](#check-against-nrscs-hec-ras-modelling)
- [New barrier lakes](#new-barrier-lakes)
- [Sudden drainage](#sudden-drainage)
- [Weather trigger](#weather-trigger)
- [Data](#data)

## Pipeline commands and outputs

```bash
cd pipeline
uv sync
uv run aahat series --lake samudra-tapu --years 2017-2026 --quicklook -v   # or --lake all
uv run aahat summary                                                        # table of every lake x year
uv run aahat discover --lat 32.50 --lon 77.20 --radius 5000 --year 2025    # find water bodies around a point
uv run pytest -q
```

Outputs land in `pipeline/out/lakes/<lake-id>/`: `series.json` (yearly areas with uncertainty and data quality),
`<year>.geojson` (lake outline), and `<year>.png` quicklooks with `--quicklook`. Re-running `series` for some
years merges into the existing `series.json`. `summary` writes `out/lakes/index.json` for the API and maps.

**Evidence:** every yearly record (in `series.json`, the year's GeoJSON properties and the yearly rows of
`index.json`) lists the Sentinel-2 scenes its outline was mapped from: `scene_ids` (Earth Search item ids) and
`scene_days`. Years computed before this was recorded have none until they are re-run.

Other commands: `aahat risk`, `aahat downstream`, `aahat barrier`, `aahat drain` (sections below),
`aahat sweep` (find large high-altitude lakes in the built-in windows), and `aahat places`, which writes
`out/places/index.json`: every named settlement in the region for search, each with its `district` and
`state` so two Hamirpurs can be told apart. Those come from OpenStreetMap admin boundaries (admin_level 5
and 4) in the Geofabrik extract, assembled once into `~/.cache/aahat/osm/admin_*.json` (about 7 s and 0.7 GB
of memory, first run only) and matched by point-in-polygon; places outside every mapped district (Tibet)
get `null`.

DEM tiles are cached whole under `~/.cache/aahat` (override with `AAHAT_CACHE_DIR`, e.g. `/tmp/aahat` on Lambda).

## How lake mapping works

For each lake and each post-monsoon season (15 Aug to 31 Oct, when lakes are near their yearly maximum and
before freeze-up), the pipeline:

1. finds Sentinel-2 L2A scenes whose tile fully covers the lake's area of interest, one per day, preferring
   the newest processing baseline;
2. reads only the 20 m scene-classification band for each, and keeps the scenes that are clearest over the
   lake (up to `--max-scenes`);
3. per scene, drops pixels that are cloud, cirrus, snow/ice, saturated or nodata (SCL), or in terrain shadow
   (ray-marched over the Copernicus DEM toward that scene's sun position). SCL "cloud shadow" and "unclassified"
   pixels are treated as uncertain: Sen2Cor often labels whole dark lakes as cloud shadow and leaves thin haze
   unclassified, so they can only add evidence for water (NDWI > 0.5), never a dry look;
4. calls a clear pixel water when NDWI (green/NIR, both 10 m bands) > 0.3. Snow, ice, rock and vegetation
   sit well below that. We tried also requiring MNDWI (green/SWIR) > 0.3, but on clear dark lakes like Chandra
   Tal green is so low that MNDWI fails mid-lake, and the 20 m SWIR band blurs shorelines, so it is off;
5. marks water where a pixel was water in at least half of its clear observations, on slopes under 25 degrees;
6. takes the connected water body at the lake's seed point, closes 2-pixel gaps (brash ice at calving fronts)
   and fills holes (icebergs).

Each yearly area carries a ±half-pixel shoreline uncertainty (perimeter × 5 m) and a coverage figure: the
share of the lake and its rim that was clearly observed at least twice. Below 90%, or with fewer than two clear
scenes in the season, the year is marked `partial`.

## How the hazard score works

`uv run aahat risk --lake all` (after `series`) writes `out/lakes/<id>/risk.json` and `outlet.geojson`.

**score = 100 × size × likelihood**, both 0–1, so a small pond in steep terrain can't outrank a large lake,
and size alone can't put a quiet lake first. Every factor is a raw number with units, turned into 0–1 by a
stated linear rule; the UI shows value, rule, score, weight, note and source for each.

| Factor | Group | Measured as | 0 → 1 | Source of threshold |
|---|---|---|---|---|
| Water volume | size | Huggel et al. 2002, V = 0.104·A^1.42, from the latest full-coverage area | 10⁵ → 10⁸ m³ (log) | Kougkoulos et al. 2018 classes at 0.33 / 0.67 |
| Lake growth | likelihood (¼) | Theil–Sen trend over full-coverage seasons, ×5; zero unless the change beats 2× measurement uncertainty | 0 → 25% per 5 yr | CWC 2024 top class |
| Steepness below outlet | likelihood (¼) | mean gradient of the first 1 km of flow path below the spill point (DEM) | 0 → 10° | Fujita et al. 2013 steep-lakefront 10° |
| Ice/rock fall sources | likelihood (¼) | area of > 30° slopes in the lake's catchment whose line to the lake is > 14° | 0 → 0.5 km² | Allen et al. 2016 (Himachal) definition, Rinzin et al. 2021 high class |
| Distance to glacier ice | likelihood (¼) | nearest Randolph Glacier Inventory 7.0 outline (c. 2000; includes debris-covered tongues) | 500 m → 0 m | Rinzin et al. 2021, CWC 2024 |

Levels: low < 20 ≤ moderate < 40 ≤ high < 60 ≤ very high.

The source links, as recorded in [`pipeline/src/aahat/risk.py`](../pipeline/src/aahat/risk.py) and returned
with every factor in `risk.json`:

| Source | Link |
|---|---|
| Huggel et al. 2002 | <https://doi.org/10.1139/t01-099> |
| Kougkoulos et al. 2018 | <https://doi.org/10.1016/j.scitotenv.2017.10.083> |
| CWC 2024 | <https://cwc.gov.in/sites/default/files/final-report-risk-index-criteria1-final-book.pdf> |
| Fujita et al. 2013 | <https://doi.org/10.5194/nhess-13-1827-2013> |
| Allen et al. 2016 | <https://doi.org/10.1007/s11069-016-2511-x> |
| Rinzin et al. 2021 | <https://doi.org/10.3389/feart.2021.775195> |

**No hindsight:** the replay scores season Y using only seasons ≤ Y and the 2011–2015 DEM (`as_of_season`,
`data_until` record what was used); a unit test checks that a later jump can't change an earlier score.

It is a screening score for deciding where to look, not a probability that a lake will burst.

Spill point and lake level come from the DEM: the outline is grown over the DEM's flat water surface, and of
the 40 lowest shore cells the one whose downhill path has dropped most after 1 km is the outlet.

## How the downstream estimate works

`uv run aahat downstream --lake all` (after `risk`) writes, per lake, `downstream.json` (stations every 250 m
for each scenario), `flood_path.geojson`, `corridor_expected.geojson`, `corridor_severe.geojson` and
`impacts.json` (every settlement, bridge, road, hydro project, school and clinic near the path, nearest first).

1. **Path:** steepest descent over the Copernicus DEM from the spill point, up to 150 km (pits escaped locally).
2. **Breach peak, two scenarios:** *expected* = Evans 1986 (0.72·V^0.53), which lands closest to NRSC's
   HEC-RAS dam-breach peak for Gepang Gath; *severe* = the larger of that and Huggel et al. 2002
   (0.00077·V^1.017, a regression on lakes up to 19 million m³, so extrapolated above that). Below about
   1.26 million m³ (lakes under roughly 0.1 km²) Evans is the larger, so both scenarios are the same flood:
   `downstream.json` and the index then carry `scenarios_identical: true`. The index's downstream summary
   names the two peaks `peak_expected_m3s` and `peak_severe_m3s` (`peak_m3s` is a deprecated alias), and
   `risk.json`'s `peak_discharge_m3s` is the severe one, with `peak_discharge_relation` saying which formula.
   The pipeline cites Evans 1986 as tabulated in Huggel et al. 2002 (<https://doi.org/10.1139/t01-099>).
3. **Downstream decay:** peak × exp(−km / 42.4 km), an e-folding distance fitted to the peaks NRSC report along
   the valley for both lakes and to South Lhonak 2023 (Sattar et al. 2025).
4. **Flood level:** Manning normal depth (n = 0.06, as NRSC use below Sissu) on a DEM cross-section every 250 m
   (slope floored at 0.002). Only the span of the section connected to the channel carries water, and the lake
   itself carries none.
5. **Corridor:** DEM cells below the flood level (interpolated between stations) that lie no further from the
   path than the water reached on that side of the nearest cross-section, connected to the river. It starts at
   the spill point and never covers the lake. A flood level only means something inside the channel it was
   solved for: without that limit, every lower cell within 2 km counted, which drew fans kilometres wide
   around lakes on ridges and plateaus. Backwater up a side valley beyond a section's reach is not drawn.
6. **Arrival:** distance ÷ flood-front speed, 10 m/s (fast) to 8 m/s (expected); South Lhonak 2023 averaged
   about 8 m/s over 67.5 km.
7. **Exposure:** OpenStreetMap (Geofabrik northern-India extract, Overpass outside it). An asset is
   *in flood path* if it is flooded in the expected scenario, *at risk* if only in the severe one or within
   10 m above the flood level near the corridor. Flooded means inside the corridor: a place lower than the
   flood level but not reached by the connected flood (behind a ridge) is not counted.

## Check against NRSC's HEC-RAS modelling

**Check against NRSC's HEC-RAS modelling** (Gepang Gath, Samudra Tapu reports on Bhuvan): NRSC's peak discharge
falls between our two scenarios at every benchmark site. Its flood depth does so at three of the five: at Sissu
below Gepang Gath NRSC's 21.5 m exceeds our 17.5 m severe depth, and at Tandi below Gepang Gath NRSC's 10.4 m is
under our 11.8 m expected depth. Our depths are the median of the stations within 1 km of the site, because
normal depth on a 30 m DEM varies by several metres from one cross-section to the next.

| Site | NRSC peak, depth | Ours expected → severe |
|---|---|---|
| Gepang Gath → Sissu, 11 km | 9,378 m³/s, 21.5 m | 5,837, 7.4 m → 30,947, 17.5 m |
| Gepang Gath → Tandi, 31 km | 4,123 m³/s, 10.4 m | 3,642, 11.8 m → 19,309, 25.5 m |
| Samudra Tapu → Batal, 17.8 km | 15,692 m³/s, 17.1 m | 6,811, 14.2 m → 48,170, 32.8 m |
| Samudra Tapu → Khoksar, 67 km | 6,665 m³/s, 12.9 m | 2,132, 5.6 m → 15,077, 14.8 m |
| Samudra Tapu → Tandi, 110 km | 3,275 m³/s, 9.4 m | 773, 3.5 m → 5,469, 10.6 m |

NRSC reports: [Gepang Gath](https://bhuvan.nrsc.gov.in/nhpfs/pdf/NRSC_GhepangGhatGlacialLake_GLOF_Risk_Assessment_Report.pdf),
[Samudra Tapu](https://bhuvan.nrsc.gov.in/nhpfs/pdf/NRSC_SamudraTapuGlacialLake_GLOF_Risk_Assessment_Report.pdf).
NRSC give "Sissu in just 21 minutes" for Gepang Gath; our fast–expected arrival there is 18–23 min.

All of it is a screening estimate: normal depth on a 30 m DEM with no river bathymetry, empirical peaks with
large scatter, and only what OpenStreetMap maps.

## New barrier lakes

`uv run aahat barrier --lake <id> --as-of YYYY-MM-DD` scans that lake's downstream river in 6 km windows
(or any reach with `--reach "lon,lat;lon,lat"`) and flags **new water** on the river line: water in the clearest
scenes of the last 20 days that was (almost) never water in the year before (ending 30 days earlier), within 1 km
of the river, at least 0.02 km² of new water and 60 m wide (a river overtopping its banks is a thin sliver; a
dammed lake is a blob). Only scenes up to `--as-of` are used. Reads at 20 m from COG overviews.

- **Replay, Sedongpu (Yarlung Tsangpo), 31 Oct 2018:** three candidates on the lake backed up behind the
  debris dam that formed on 29 Oct ([SANDRP](https://sandrp.in/2018/10/19/landslide-dam-on-tsangpo-creates-flood-disaster-risk-for-siang/)).
- **False-positive check:** 0 candidates along the first 30 km below Gepang Gath as of 8 Oct 2026.
- **Reservoirs:** a reservoir refilling looks the same as a new lake (the scan below Lam Dal flagged 0.107 km²
  at the head of the Chamera reservoir on the Ravi, 8 km from the dam). Each candidate gets `near_dam_km`, the
  straight-line distance to the nearest OpenStreetMap dam, weir or hydro plant within 12 km, and a `kind`:
  `reservoir_level_change` if there is one, else `possible_barrier_lake`. Both stay in the JSON; the summary
  line counts them separately.
- Limits: lakes narrower than about 3 pixels (60 m) are missed (e.g. Kunwari, Uttarakhand, ~50 m wide), and
  cloud during the monsoon hides short-lived lakes. Area counts only the new water, not the old channel inside it.

On the server the scan runs for the first 60 km below every monitored lake at each refresh
([`infra/ec2/refresh.sh`](../infra/ec2/refresh.sh)).

## Sudden drainage

`uv run aahat drain --lake <id|all> [--as-of YYYY-MM-DD]` (after `series`) asks whether a lake has partly
emptied since its season outline was mapped, which is what a lake that burst looks like from above. It maps
water on the newest clear scene of the last 12 days, takes the lake at its seed point, and compares:
drop = 1 − latest area / season area, against the latest season up to `--as-of` whose lake was fully seen.

`drained` is true only if the drop exceeds 30% and 3× the combined shoreline uncertainty of the two outlines,
**and** that one scene observed at least 90% of the season outline's pixels. Cloud, cloud shadow and terrain
shadow are unknown, never dry, so a cloud over the lake cannot look like a drained lake (`no_clear_scene`).
Lakes smaller than 50,000 m² are not checked (`too_small`): at a few hundred pixels one scene differs from
the season outline by tens of percent (Suraj Tal read 73% lower on 2026-10-05 while every larger lake
stayed within 1%).
If more than 20% of the outline is snow or ice the lake is freezing over, not emptying: `frozen_or_snow`,
never flagged, and older scenes are not consulted either. Writes `out/lakes/<id>/drain.json` (season year and
area, the latest scene's day, item id, area and coverage, drop, threshold, status, note) and a `drain`
summary per lake in `lakes/index.json`.

On the server a lake that newly shows as drained raises a dry-run alert for officials to review; nothing is
sent to phones from that check ([`infra/ec2/drain-alert.sh`](../infra/ec2/drain-alert.sh)).

## Weather trigger

The API, not the pipeline, computes this ([`api/README.md`](../api/README.md#weather-trigger)). For each lake
it asks [Open-Meteo](https://open-meteo.com) for three past and three forecast days and keeps the answer for
3 hours.

- **high:** a forecast day with 50 mm of precipitation or more, or 100 mm over the forecast days.
- **elevated:** a forecast day with 20 mm or more, 40 mm over the forecast days, or a forecast maximum
  temperature 5 C or more above the mean of the past 3 days (fast melt).
- otherwise **normal**.

These thresholds are Aahat's own screening choice, not a published standard. The level does not change the
risk score.

## Data

- Sentinel-2 L2A COGs via [Earth Search](https://earth-search.aws.element84.com/v1) (AWS Open Data, us-west-2), read in place with HTTP range requests.
- [Copernicus DEM GLO-30](https://registry.opendata.aws/copernicus-dem/) on AWS Open Data.
- Lake seed points from OpenStreetMap where named there, refined from imagery.
- Exposure from OpenStreetMap: [Geofabrik northern-India extract](https://download.geofabrik.de/asia/india/northern-zone.html), Overpass elsewhere.
- Glacier outlines: Randolph Glacier Inventory 7.0 via the [GLIMS](https://www.glims.org) WFS.
- Weather outlook: [Open-Meteo](https://open-meteo.com).

All downstream and risk numbers are screening estimates, not engineering hydraulics.
