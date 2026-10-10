#!/bin/bash
# Runs ON the server as user aahat (end of the drain loop in refresh.sh): for every lake the drain check
# flags as drained, raise a DRY-RUN alert so officials see it in the dashboard's alert log and decide
# whether to send it. Never sends anything itself. Each scene raises once: the scene last alerted is
# remembered per lake in the state directory.
#   drain-alert.sh [data dir]      default /srv/aahat/data
# Env (for testing by hand): AAHAT_API_URL, AAHAT_TOKEN_FILE, AAHAT_DRAIN_STATE.
set -uo pipefail
DATA=${1:-/srv/aahat/data}
API=${AAHAT_API_URL:-http://127.0.0.1:8080}
TOKEN_FILE=${AAHAT_TOKEN_FILE:-/srv/aahat/official.token}
STATE=${AAHAT_DRAIN_STATE:-/srv/aahat/state/drain-alerted}

command -v jq >/dev/null || { echo "drain alert: jq is not installed"; exit 1; }
TOKEN=$(cat "$TOKEN_FILE" 2>/dev/null) || { echo "drain alert: cannot read $TOKEN_FILE"; exit 1; }
mkdir -p "$STATE" || exit 1

status=0
for f in "$DATA"/lakes/*/drain.json; do
  [ -f "$f" ] || continue
  [ "$(jq -r '.drained == true' "$f" 2>/dev/null)" = true ] || continue
  lake=$(basename "$(dirname "$f")")
  scene=$(jq -r '.latest.item_id // .latest.day // empty' "$f")
  [ -n "$scene" ] || { echo "drain alert: $lake is flagged but names no scene"; status=1; continue; }
  [ "$(cat "$STATE/$lake" 2>/dev/null)" = "$scene" ] && continue

  body=$(jq -c --arg lake "$lake" '{
    lake_id: $lake,
    source: "satellite_drain_check",
    dry_run: true,
    note: ("Area fell \((.drop_fraction // 0) * 100 | round)% between the \(.season_year // "current") season outline and the scene of \(.latest.day // "unknown date")"
           + (if .latest.item_id then " (\(.latest.item_id))" else "" end))[:300]
  }' "$f") || { echo "drain alert: cannot read $f"; status=1; continue; }

  code=$(curl -sS -m 60 -o /dev/null -w '%{http_code}' -X POST "$API/trigger" \
    -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d "$body") || code=000
  if [ "$code" = 201 ]; then
    echo "$scene" > "$STATE/$lake"
    echo "drain alert: raised a dry run for $lake ($scene)"
  else
    echo "drain alert: trigger for $lake failed (HTTP $code); will retry on the next refresh"
    status=1
  fi
done
exit $status
