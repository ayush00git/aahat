#!/bin/bash
# Runs ON the server (systemd timer, every 2 days): pick up new Sentinel-2 scenes for the current season,
# recompute risk, downstream impacts, the index and the place list, and scan for new barrier lakes, in
# place in the directory the API serves.
set -euo pipefail
export HOME=/srv/aahat AAHAT_CACHE_DIR=/srv/aahat/cache
cd /srv/aahat/pipeline
YEAR=$(date -u +%Y)
OUT=/srv/aahat/data
LAKES=$(/srv/aahat/.local/bin/uv run --frozen python -c "from aahat.lakes import load_lakes; print(' '.join(l.id for l in load_lakes()))")
for lake in $LAKES; do
  /srv/aahat/.local/bin/uv run --frozen aahat series --lake "$lake" --years "$YEAR" --max-scenes 10 --out "$OUT" || echo "series failed: $lake"
done
/srv/aahat/.local/bin/uv run --frozen aahat risk --lake all --out "$OUT"
/srv/aahat/.local/bin/uv run --frozen aahat downstream --lake all --out "$OUT"
/srv/aahat/.local/bin/uv run --frozen aahat summary --out "$OUT"
/srv/aahat/.local/bin/uv run --frozen aahat places --out "$OUT"
# New barrier (landslide-dammed) lakes on the first 60 km below each monitored lake
for lake in $LAKES; do
  /srv/aahat/.local/bin/uv run --frozen aahat barrier --lake "$lake" --max-km 60 --out "$OUT" || echo "barrier scan failed: $lake"
done
# Sudden drainage: compare the newest clear scene with the season outline
for lake in $LAKES; do
  /srv/aahat/.local/bin/uv run --frozen aahat drain --lake "$lake" --out "$OUT" || echo "drain check failed: $lake"
done
# A newly drained lake raises a dry-run alert for officials to review; nothing is sent from here
/srv/aahat/bin/drain-alert.sh "$OUT" || echo "drain alert failed"
# Back up data and state to the data bucket; a failed backup must not fail the refresh
/srv/aahat/bin/s3-sync.sh || echo "s3 sync failed"
echo "refreshed $(date -u +%FT%TZ)"
