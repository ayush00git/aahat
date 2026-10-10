#!/bin/bash
# Runs ON the server as user aahat (end of refresh.sh, and aahat-s3-sync.timer daily): copy the served
# data and the API's state to the versioned data bucket (data/ is public to read, state/ is private). Credentials come from the instance role.
set -euo pipefail
export HOME=/srv/aahat
[ -f /srv/aahat/ops.env ] && . /srv/aahat/ops.env
if [ -z "${DATA_BUCKET:-}" ]; then
  echo "DATA_BUCKET is not set in /srv/aahat/ops.env (run infra/ec2/data-bucket.sh)"; exit 1
fi
export AWS_DEFAULT_REGION="${AWS_REGION:-ap-south-1}"
# --delete mirrors removals, so never sync an empty or half-seeded data directory over the bucket
if [ -f /srv/aahat/data/lakes/index.json ]; then
  aws s3 sync /srv/aahat/data "s3://$DATA_BUCKET/data" --delete --only-show-errors
else
  echo "no /srv/aahat/data/lakes/index.json: skipping data/ (restore it from the bucket instead)"
fi
# Subscriptions, the alert log and the web-push keys: the state/ prefix, which the bucket policy keeps private. Polly MP3s are regenerated.
aws s3 sync /srv/aahat/state "s3://$DATA_BUCKET/state" --only-show-errors --exclude "audio/*"
echo "synced to s3://$DATA_BUCKET $(date -u +%FT%TZ)"
