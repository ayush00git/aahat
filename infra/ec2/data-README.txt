Aahat: glacial lake watch for Himachal Pradesh, published data
===============================================================

Project and methods: https://github.com/ayush00git/aahat  (see README.md, docs/methods.md, API.md)

This prefix holds the files Aahat's pipeline produces and its API serves:

  lakes/index.json            every monitored lake: position, yearly areas, risk, downstream summary
  lakes/<id>/series.json      lake area per season (15 Aug to 31 Oct), with the Sentinel-2 scenes used
  lakes/<id>/<year>.geojson   lake outline for that season
  lakes/<id>/risk.json        risk score per season and its factors
  lakes/<id>/downstream.json  flood path stations: discharge, depth, arrival time, two scenarios
  lakes/<id>/impacts.json     settlements, roads, bridges and other assets along the path
  lakes/<id>/*.geojson        flood path, flood corridors, outlet, glacier outlines
  lakes/<id>/drain.json       latest sudden-drainage check
  barrier/<id>_latest.json    latest scan for new water on the river below the lake
  places/index.json           searchable settlements with district and state

The files are refreshed every 2 days from new Sentinel-2 scenes.

Read before use
---------------
Every flood figure (discharge, depth, extent, arrival time) is a screening estimate from a 30 m
elevation model and empirical relations. It is not a hydraulic model, not a forecast, and not an
official warning. The risk score is a ranking aid, not a probability. Provided as is, without warranty.

Sources and attribution
-----------------------
- Contains modified Copernicus Sentinel data (Sentinel-2 L2A), read from the AWS Open Data registry.
- Copernicus DEM GLO-30 (c) DLR e.V. 2010-2014 and (c) Airbus Defence and Space GmbH 2014-2018,
  provided under COPERNICUS by the European Union and ESA; all rights reserved.
- Glacier outlines: Randolph Glacier Inventory 7.0 (RGI Consortium, 2023), via GLIMS.
- Places, roads, bridges and other assets: (c) OpenStreetMap contributors, Open Database License (ODbL).
  Files derived from them (places/, lakes/<id>/impacts.json) carry the same licence.

If you use this data, please credit "Aahat (github.com/ayush00git/aahat)" and the sources above.
