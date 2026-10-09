#!/bin/bash
# Runs ON the server (systemd timer, weekly): pick up new Sentinel-2 scenes for the current season and
# recompute risk, downstream impacts and the index, in place in the directory the API serves.
set -euo pipefail
export HOME=/srv/aahat AAHAT_CACHE_DIR=/srv/aahat/cache
cd /srv/aahat/pipeline
YEAR=$(date -u +%Y)
OUT=/srv/aahat/data
for lake in $(/srv/aahat/.local/bin/uv run --frozen python -c "from aahat.lakes import load_lakes; print(' '.join(l.id for l in load_lakes()))"); do
  /srv/aahat/.local/bin/uv run --frozen aahat series --lake "$lake" --years "$YEAR" --max-scenes 10 --out "$OUT" || echo "series failed: $lake"
done
/srv/aahat/.local/bin/uv run --frozen aahat risk --lake all --out "$OUT"
/srv/aahat/.local/bin/uv run --frozen aahat downstream --lake all --out "$OUT"
/srv/aahat/.local/bin/uv run --frozen aahat summary --out "$OUT"
echo "refreshed $(date -u +%FT%TZ)"
