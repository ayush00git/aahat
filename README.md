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
uv run aahat series --lake samudra-tapu --years 2019,2025 --quicklook -v
uv run aahat discover --lat 32.50 --lon 77.20 --radius 5000 --year 2025
```

Outputs land in `pipeline/out/lakes/<lake-id>/`: `series.json` (yearly areas with uncertainty and data quality),
`<year>.geojson` (lake outline), and `<year>.png` quicklooks with `--quicklook`.

## Data

- Sentinel-2 L2A COGs via [Earth Search](https://earth-search.aws.element84.com/v1) (AWS Open Data, us-west-2), read in place with HTTP range requests.
- [Copernicus DEM GLO-30](https://registry.opendata.aws/copernicus-dem/) on AWS Open Data.
- Lake seed points from OpenStreetMap where named there, refined from imagery.

All downstream and risk numbers are screening estimates, not engineering hydraulics.
