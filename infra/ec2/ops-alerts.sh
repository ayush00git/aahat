#!/bin/bash
# Failure emails: an SNS topic with an email subscription, and on the server a systemd template unit
# (aahat-notify-failure@<unit>.service) that the other units call with OnFailure=. Safe to re-run.
#   AAHAT_OPS_EMAIL=you@example.com ./ops-alerts.sh     first run (the address is remembered in .state)
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
source "$AAHAT_STATE_FILE"
save() { grep -v "^export $1=" "$AAHAT_STATE_FILE" > "$AAHAT_STATE_FILE.tmp" || true; echo "export $1=$2" >> "$AAHAT_STATE_FILE.tmp"; mv "$AAHAT_STATE_FILE.tmp" "$AAHAT_STATE_FILE"; export "$1=$2"; }
EMAIL="${AAHAT_OPS_EMAIL:-${OPS_EMAIL:-}}"
[ -n "$EMAIL" ] || { echo "set AAHAT_OPS_EMAIL=<address that gets the failure emails>"; exit 1; }
save OPS_EMAIL "$EMAIL"

# create-topic returns the existing topic when the name is taken
save OPS_TOPIC_ARN "$(aws sns create-topic --name "$AAHAT_NAME-ops" --tags Key=project,Value=aahat --query TopicArn --output text)"
SUB=$(aws sns list-subscriptions-by-topic --topic-arn "$OPS_TOPIC_ARN" \
  --query "Subscriptions[?Protocol=='email' && Endpoint=='$EMAIL'].SubscriptionArn | [0]" --output text)
if [ "$SUB" = None ] || [ -z "$SUB" ]; then
  aws sns subscribe --topic-arn "$OPS_TOPIC_ARN" --protocol email --notification-endpoint "$EMAIL" >/dev/null
  echo "subscribed $EMAIL: open the 'AWS Notification - Subscription Confirmation' email and click Confirm"
elif [ "$SUB" = PendingConfirmation ]; then
  echo "$EMAIL has not confirmed yet: click Confirm in the email from AWS Notifications (check spam)"
fi

SSHOPT="-i $AAHAT_KEY_FILE -o StrictHostKeyChecking=accept-new"
ssh $SSHOPT "ec2-user@$PUBLIC_IP" 'mkdir -p /tmp/aahat-ops'
scp -q $SSHOPT notify-failure.sh aahat-notify-failure@.service aahat-api.service aahat-refresh.service "ec2-user@$PUBLIC_IP:/tmp/aahat-ops/"
ssh $SSHOPT "ec2-user@$PUBLIC_IP" "OPS_TOPIC_ARN=$OPS_TOPIC_ARN AWS_REGION=$AWS_REGION bash -s" <<'REMOTE'
set -euo pipefail
aws --version >/dev/null   # Amazon Linux 2023 ships AWS CLI v2; notify-failure.sh needs it
setenv() { sudo touch /srv/aahat/ops.env; { sudo grep -v "^$1=" /srv/aahat/ops.env || true; echo "$1=$2"; } > /tmp/aahat-ops/ops.env; sudo install -m 644 -o aahat -g aahat /tmp/aahat-ops/ops.env /srv/aahat/ops.env; }
setenv AWS_REGION "$AWS_REGION"
setenv OPS_TOPIC_ARN "$OPS_TOPIC_ARN"
sudo install -m 755 -o aahat -g aahat /tmp/aahat-ops/notify-failure.sh /srv/aahat/bin/notify-failure.sh
sudo install -m 644 /tmp/aahat-ops/aahat-notify-failure@.service /tmp/aahat-ops/aahat-api.service /tmp/aahat-ops/aahat-refresh.service /etc/systemd/system/
sudo systemctl daemon-reload   # OnFailure= applies from the next failure; no restart needed
REMOTE
echo "ops alerts: $OPS_TOPIC_ARN -> $EMAIL"
echo "test: ssh ... sudo systemd-run --unit=aahat-failtest -p OnFailure=aahat-notify-failure@aahat-failtest.service /bin/false"
