# Aahat (आहट)

A satellite watch on Himalayan glacial lakes and the valleys below them, starting with Himachal Pradesh.
Built for [Environmental Hacks](https://www.wemakedevs.org/aws/env) (WeMakeDevs x AWS), Heat and Water track.

## Layout

| Path | What | Language |
|------|------|----------|
| `pipeline/` | Sentinel-2 water mapping, lake area time series, terrain, risk, downstream routing | Python |
| `api/` | HTTP API and alert dispatcher | Go |
| `web/` | Villager view (Hindi first) and officials' dashboard | TypeScript + MapLibre |
| `infra/` | AWS infrastructure | IaC |

## Pipeline quick start

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

DEM tiles are cached whole under `~/.cache/aahat` (override with `AAHAT_CACHE_DIR`, e.g. `/tmp/aahat` on Lambda).

### How lake mapping works

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

### How the hazard score works

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

**No hindsight:** the replay scores season Y using only seasons ≤ Y and the 2011–2015 DEM (`as_of_season`,
`data_until` record what was used); a unit test checks that a later jump can't change an earlier score.

It is a screening score for deciding where to look, not a probability that a lake will burst.

Spill point and lake level come from the DEM: the outline is grown over the DEM's flat water surface, and of
the 40 lowest shore cells the one whose downhill path has dropped most after 1 km is the outlet.

## Data

- Sentinel-2 L2A COGs via [Earth Search](https://earth-search.aws.element84.com/v1) (AWS Open Data, us-west-2), read in place with HTTP range requests.
- [Copernicus DEM GLO-30](https://registry.opendata.aws/copernicus-dem/) on AWS Open Data.
- Lake seed points from OpenStreetMap where named there, refined from imagery.

All downstream and risk numbers are screening estimates, not engineering hydraulics.
