#!/bin/bash
# Runs ON the server (aahat-notify-failure@<unit>.service, started by OnFailure=): email the ops topic
# that a unit failed, with the tail of its journal. Credentials come from the instance role.
set -uo pipefail
UNIT="${1:?usage: notify-failure.sh <unit>}"
[ -f /srv/aahat/ops.env ] && . /srv/aahat/ops.env
if [ -z "${OPS_TOPIC_ARN:-}" ]; then
  echo "OPS_TOPIC_ARN is not set in /srv/aahat/ops.env (run infra/ec2/ops-alerts.sh); $UNIT failed, nobody was told"
  exit 1
fi
HOST=$(hostname)
NOW=$(date -u +%FT%TZ)
MSG=$(printf 'Unit: %s\nHost: %s\nTime: %s (UTC)\n\nLast 30 journal lines:\n%s\n' \
  "$UNIT" "$HOST" "$NOW" "$(journalctl -u "$UNIT" -n 30 --no-pager -o short-iso 2>&1)")
SUBJECT=$(printf 'Aahat: %s failed' "$UNIT" | cut -c1-100)
ID=$(aws sns publish --region "${AWS_REGION:-ap-south-1}" --topic-arn "$OPS_TOPIC_ARN" \
  --subject "$SUBJECT" --message "$MSG" --query MessageId --output text) || { echo "sns publish failed for $UNIT"; exit 1; }
echo "sns publish ok: unit=$UNIT message-id=$ID"
