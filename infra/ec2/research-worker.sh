#!/bin/bash
# Runs ON the server as user aahat (research-worker.service, started by research-jobs.path whenever the
# jobs directory changes): take each queued researcher job (a JSON file the API wrote), run the pipeline on
# that lake and write the outcome back into the job file. One job at a time: the server has 2 GB of RAM.
#
#   <jobs>/<id>.json   the job; this script edits status, started_at, finished_at, error, results
#   <jobs>/<id>.log    what the pipeline printed (the API shows its tail)
#   <out>/<id>/        series.json, <year>.geojson, risk.json, outlet.geojson, discover.json
set -uo pipefail
ROOT=/srv/aahat
JOBS="${AAHAT_JOBS_DIR:-$ROOT/state/jobs}"
OUT="${AAHAT_RESEARCH_DIR:-$ROOT/research}"
UV="$ROOT/.local/bin/uv"
export HOME="$ROOT" AAHAT_CACHE_DIR="$ROOT/cache"
[ -f "$ROOT/ops.env" ] && . "$ROOT/ops.env"      # DATA_BUCKET, AWS_REGION (infra/ec2/data-bucket.sh)
BUCKET="${AAHAT_DATA_BUCKET:-${DATA_BUCKET:-}}"
if [ -z "$BUCKET" ] && [ -r "$ROOT/api.env" ]; then
  BUCKET=$(sed -n 's/^AAHAT_DATA_BUCKET=//p' "$ROOT/api.env" | tail -1)
fi
export AWS_DEFAULT_REGION="${AWS_REGION:-ap-south-1}"
SNAP_RADIUS_M=1500     # how far from the given point the lake may be
MIN_AREA_M2=2000       # smallest water body taken for a lake (20 Sentinel-2 pixels)
MIN_FREE_MB=350        # do not start the pipeline with less memory available than this

mkdir -p "$JOBS" "$OUT"
cd "$ROOT/pipeline" || exit 1
exec 9>"$JOBS/.lock"
flock -n 9 || { echo "another research worker holds the lock"; exit 0; }

now() { date -u +%FT%TZ; }

# edit <job file> <jq filter> [jq args...]: rewrite the job file atomically
edit() {
  local f="$1" filter="$2"; shift 2
  local tmp; tmp=$(mktemp "$JOBS/.edit-XXXXXX.tmp") || return 1
  if jq "$@" "$filter" "$f" > "$tmp"; then chmod 600 "$tmp"; mv "$tmp" "$f"; else rm -f "$tmp"; return 1; fi
}

fail() {  # fail <job file> <message>
  echo "FAILED: $2"
  edit "$1" '.status="failed" | .error=$e | .finished_at=$t' --arg e "$2" --arg t "$(now)"
}

# A job left "running" belongs to a worker that died (reboot, out of memory): close it.
for f in "$JOBS"/r-*.json; do
  [ -f "$f" ] || continue
  if [ "$(jq -r '.status // ""' "$f" 2>/dev/null)" = "running" ]; then
    fail "$f" "the worker was interrupted while running this job (server restart or out of memory); queue it again" >/dev/null
  fi
done

# A oneshot service is "activating" for as long as it runs.
refresh_running() {
  case "$(systemctl is-active aahat-refresh.service 2>/dev/null)" in
    active|activating|deactivating|reloading) return 0 ;;
  esac
  return 1
}

next_job() {  # oldest queued job file, by name (ids start with the date)
  local f
  for f in "$JOBS"/r-*.json; do
    [ -f "$f" ] || continue
    if [ "$(jq -r '.status // ""' "$f" 2>/dev/null)" = "queued" ]; then echo "$f"; return 0; fi
  done
  return 1
}

# results <dir> <s3 prefix or ""> : the results object for the job file, from the files in <dir>
results() {
  local dir="$1" s3="$2" series='[]' risk='null' snapped='null' files
  if [ -f "$dir/series.json" ]; then
    series=$(jq -c '[.years[] | {year, status, area_m2, uncertainty_m2, coverage, scenes_clear, last_day}]' "$dir/series.json")
  fi
  if [ -f "$dir/risk.json" ]; then
    risk=$(jq -c '.latest | if . == null then null else
      {score, level, as_of_season, data_until, area_m2, area_year, volume_m3, peak_discharge_m3s,
       factors: [.factors[] | {key, label, value, unit, score, note}]} end' "$dir/risk.json")
  fi
  [ -f "$dir/snapped.json" ] && snapped=$(jq -c . "$dir/snapped.json")
  files=$(cd "$dir" && ls -1 | jq -R . | jq -sc .)
  jq -nc --arg dir "$dir" --arg s3 "$s3" --argjson files "$files" --argjson series "$series" \
     --argjson risk "$risk" --argjson snapped "$snapped" \
     '{dir: $dir, files: $files, snapped: $snapped, series: $series, risk: $risk}
      + (if $s3 == "" then {} else {s3_prefix: $s3} end)'
}

run_job() {
  local f="$1" id name lat lon y0 y1 dir this_year dyear snapped slat slon area radius s3=""
  id=$(basename "$f" .json)
  if ! [[ "$id" =~ ^r-[0-9]{8}-[0-9a-f]{8}$ ]]; then fail "$f" "bad job id"; return; fi
  name=$(jq -r '.lake.name // .name // "lake"' "$f")
  lat=$(jq -r '.lake.lat | numbers' "$f"); lon=$(jq -r '.lake.lon | numbers' "$f")
  y0=$(jq -r '.year_from | numbers | floor' "$f"); y1=$(jq -r '.year_to | numbers | floor' "$f")
  dir="$OUT/$id"
  edit "$f" '.status="running" | .started_at=$t | del(.error, .finished_at, .results)' --arg t "$(now)" || return
  echo "== $(now) job $id: $name ($lat, $lon) seasons $y0-$y1"

  if [ -z "$lat" ] || [ -z "$lon" ]; then fail "$f" "the job has no coordinates (lake.lat, lake.lon)"; return; fi
  this_year=$(date -u +%Y)
  if [ -z "$y0" ] || [ -z "$y1" ] || [ "$y0" -gt "$y1" ] || [ "$y0" -lt 2016 ] || [ "$y1" -gt "$this_year" ] || [ $((y1 - y0)) -ge 10 ]; then
    fail "$f" "year_from/year_to must be 2016-$this_year, at most 10 seasons"; return
  fi

  # Never alongside the scheduled refresh, and never when memory is already short.
  local waited=0
  while refresh_running; do
    [ "$waited" -ge 240 ] && { fail "$f" "the scheduled data refresh was still running after 4 hours; queue the job again"; return; }
    [ $((waited % 30)) -eq 0 ] && echo "waiting for aahat-refresh.service to finish"
    sleep 60; waited=$((waited + 1))
  done
  local free; free=$(awk '/MemAvailable/ {print int($2/1024)}' /proc/meminfo)
  echo "memory available: ${free} MB"
  if [ "$free" -lt "$MIN_FREE_MB" ]; then fail "$f" "the server is short of memory (${free} MB available); queue the job again later"; return; fi

  rm -rf "$dir"; mkdir -p "$dir"

  # 1. Snap to the water body: the water nearest the given point, in the latest complete season.
  dyear=$y1; [ "$dyear" -ge "$this_year" ] && dyear=$((this_year - 1))
  echo "-- discover: water within $SNAP_RADIUS_M m, season $dyear"
  if ! timeout 30m "$UV" run --frozen aahat discover --lat "$lat" --lon "$lon" --radius "$SNAP_RADIUS_M" \
        --year "$dyear" --max-scenes 8 --min-area "$MIN_AREA_M2" > "$dir/discover.out"; then
    cat "$dir/discover.out"
    fail "$f" "could not read satellite scenes around this point (see log_tail)"; return
  fi
  cat "$dir/discover.out"
  if ! jq -e 'type == "array" and length > 0' "$dir/discover.out" >/dev/null 2>&1; then
    rm -f "$dir/discover.out"
    fail "$f" "no water body of at least $MIN_AREA_M2 m2 found within $SNAP_RADIUS_M m of ($lat, $lon) in $dyear; send more exact coordinates"; return
  fi
  mv "$dir/discover.out" "$dir/discover.json"
  jq -c --argjson lat "$lat" --argjson lon "$lon" '
      def rad: . * 3.141592653589793 / 180;
      map(. + {distance_m: ((((.lat - $lat) * 111320) as $dy | ((.lon - $lon) * 111320 * ($lat | rad | cos)) as $dx
                             | ($dx * $dx + $dy * $dy) | sqrt | round))})
      | sort_by(.distance_m) | .[0]' "$dir/discover.json" > "$dir/snapped.json"
  snapped=$(cat "$dir/snapped.json")
  slat=$(jq -r .lat <<<"$snapped"); slon=$(jq -r .lon <<<"$snapped"); area=$(jq -r .area_km2 <<<"$snapped")
  # Window around the lake: about four lake radii plus a margin, 1 to 5 km (the catalogue's lakes use the same range).
  radius=$(jq -n --argjson a "$area" '(4 * (($a * 1e6 / 3.141592653589793) | sqrt) + 700) / 100 | round * 100 | if . < 1000 then 1000 elif . > 5000 then 5000 else . end')
  echo "snapped to $slat, $slon (${area} km2), window radius $radius m"

  # 2. Yearly areas, then the hazard score. The pipeline's own functions, on a lake that is not in its catalogue.
  echo "-- series $y0-$y1 and risk"
  timeout 150m "$UV" run --frozen python - "$id" "$name" "$slat" "$slon" "$radius" "$y0" "$y1" "$dir" <<'PY'
import logging
import shutil
import sys
from pathlib import Path

from aahat.lakes import Lake
from aahat.risk import run_risk
from aahat.timeseries import run_lake

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(message)s", stream=sys.stdout)
job_id, name, lat, lon, radius, y0, y1, out = sys.argv[1:]
out = Path(out)
lake = Lake(id=job_id, name=name, name_hi=name, lat=float(lat), lon=float(lon), aoi_radius_m=float(radius),
            district="", basin="", kind="research", notes="researcher job, not in the catalogue")
run_lake(lake, list(range(int(y0), int(y1) + 1)), out, max_scenes=8)
lake_dir = out / "lakes" / job_id          # run_lake writes <out>/lakes/<id>/; keep the job directory flat
for path in lake_dir.iterdir():
    shutil.move(str(path), str(out / path.name))
lake_dir.rmdir()
(out / "lakes").rmdir()
sys.stdout.flush()
try:
    run_risk(out)
except Exception as e:  # the series alone is still a result
    logging.exception("risk scoring failed")
    print(f"risk scoring failed: {e}")
PY
  local rc=$?
  if [ ! -f "$dir/series.json" ]; then
    [ "$rc" -eq 124 ] && { fail "$f" "the analysis took longer than 150 minutes and was stopped"; return; }
    fail "$f" "the pipeline did not produce a series (exit $rc; see log_tail)"; return
  fi

  # 3. Copy to the data bucket if there is one. Best effort: the results stay on the server either way.
  if [ -n "$BUCKET" ]; then
    if aws s3 sync "$dir" "s3://$BUCKET/research/$id/" --only-show-errors; then
      s3="s3://$BUCKET/research/$id/"; echo "uploaded to $s3"
    else
      echo "upload to s3://$BUCKET failed (results are kept in $dir)"
    fi
  fi

  local res; res=$(results "$dir" "$s3") || { fail "$f" "could not read the results in $dir"; return; }
  if jq -e '[.series[] | select(.area_m2 != null)] | length == 0' <<<"$res" >/dev/null; then
    edit "$f" '.results=$r' --argjson r "$res"
    fail "$f" "no lake outline could be measured at this point in $y0-$y1 (cloud, ice cover, or no lake here)"; return
  fi
  edit "$f" '.status="done" | .finished_at=$t | .results=$r' --arg t "$(now)" --argjson r "$res"
  echo "== $(now) job $id done"
}

while f=$(next_job); do
  id=$(basename "$f" .json)
  run_job "$f" >> "$JOBS/$id.log" 2>&1
  status=$(jq -r '.status // ""' "$f" 2>/dev/null)
  echo "job $id: $status"
  # A job that could not even be marked running would be picked again forever: stop instead.
  [ "$status" = "queued" ] && { echo "job $id is still queued (job file not writable?)"; exit 1; }
done
exit 0
