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
3. per scene, drops pixels that are cloud, cloud shadow, cirrus, snow/ice, saturated or nodata (SCL), or in
   terrain shadow (ray-marched over the Copernicus DEM toward that scene's sun position);
4. calls a clear pixel water when NDWI (green/NIR, both 10 m bands) > 0.3. Snow, ice, rock and vegetation
   sit well below that. We tried also requiring MNDWI (green/SWIR) > 0.3, but on clear dark lakes like Chandra
   Tal green is so low that MNDWI fails mid-lake, and the 20 m SWIR band blurs shorelines, so it is off;
5. marks water where a pixel was water in at least half of its clear observations (at least 2; a season
   with a single clear scene uses that one and is flagged `partial`), on slopes
   under 25 degrees;
6. takes the connected water body at the lake's seed point, closes 2-pixel gaps (brash ice at calving fronts)
   and fills holes (icebergs).

Each yearly area carries a ±half-pixel shoreline uncertainty (perimeter × 5 m) and a coverage figure: the
share of the lake and its rim that was clearly observed at least twice. Below 90% the year is marked `partial`.

## Data

- Sentinel-2 L2A COGs via [Earth Search](https://earth-search.aws.element84.com/v1) (AWS Open Data, us-west-2), read in place with HTTP range requests.
- [Copernicus DEM GLO-30](https://registry.opendata.aws/copernicus-dem/) on AWS Open Data.
- Lake seed points from OpenStreetMap where named there, refined from imagery.

All downstream and risk numbers are screening estimates, not engineering hydraulics.
