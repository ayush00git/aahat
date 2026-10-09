#!/bin/bash
# Build and ship the API, both web apps and the pipeline's results to the server, then restart.
#   AAHAT_SMS=sns ./deploy.sh      also turn on SMS through SNS (sandbox: verified numbers only)
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
source "$AAHAT_STATE_FILE"
ROOT=$(cd ../.. && pwd)
SSH="ssh -i $AAHAT_KEY_FILE -o StrictHostKeyChecking=accept-new ec2-user@$PUBLIC_IP"
RSYNC="rsync -az --delete -e \"ssh -i $AAHAT_KEY_FILE -o StrictHostKeyChecking=accept-new\""

echo "== build"
(cd "$ROOT/api" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o "$ROOT/api/bin/aahat-server" ./cmd/server)
for app in villager dashboard; do
  if [ -z "${AAHAT_SKIP_WEB:-}" ] && [ -f "$ROOT/web/$app/package.json" ]; then
    (cd "$ROOT/web/$app" && npm ci --silent && VITE_API_BASE=/api npm run build --silent)
  fi
done

echo "== ship"
$SSH 'sudo mkdir -p /tmp/aahat/data/lakes && sudo chown -R ec2-user /tmp/aahat'
eval $RSYNC "$ROOT/api/bin/aahat-server" "ec2-user@$PUBLIC_IP:/tmp/aahat/"
eval $RSYNC --include='*/' --include='*.json' --include='*.geojson' --exclude='*' "$ROOT/pipeline/out/lakes/" "ec2-user@$PUBLIC_IP:/tmp/aahat/data/lakes/"
for app in villager dashboard; do
  [ -z "${AAHAT_SKIP_WEB:-}" ] && [ -d "$ROOT/web/$app/dist" ] && eval $RSYNC "$ROOT/web/$app/dist/" "ec2-user@$PUBLIC_IP:/tmp/aahat/web-$app/"
done
sed "s/{\$SITE_HOST}/$SITE_HOST/" Caddyfile.tmpl > /tmp/aahat-Caddyfile
scp -q -i "$AAHAT_KEY_FILE" /tmp/aahat-Caddyfile "ec2-user@$PUBLIC_IP:/tmp/aahat/Caddyfile"
scp -q -i "$AAHAT_KEY_FILE" aahat-api.service "ec2-user@$PUBLIC_IP:/tmp/aahat/"

echo "== install"
$SSH "AWS_REGION=$AWS_REGION AAHAT_SMS=${AAHAT_SMS:-} bash -s" <<'REMOTE'
set -euo pipefail
sudo install -m 755 /tmp/aahat/aahat-server /srv/aahat/bin/aahat-server
sudo rsync -a --delete /tmp/aahat/data/ /srv/aahat/data/
for app in villager dashboard; do
  [ -d /tmp/aahat/web-$app ] && sudo rsync -a --delete /tmp/aahat/web-$app/ /srv/aahat/web/$app/
done
printf 'AWS_REGION=%s\nAAHAT_POLLY=1\nAAHAT_SMS=%s\n' "$AWS_REGION" "$AAHAT_SMS" | sudo tee /srv/aahat/api.env >/dev/null
[ -f /srv/aahat/webhook.secret ] || openssl rand -hex 32 | sudo tee /srv/aahat/webhook.secret >/dev/null
echo "AAHAT_WEBHOOK_SECRET=$(sudo cat /srv/aahat/webhook.secret)" | sudo tee -a /srv/aahat/api.env >/dev/null
sudo chown -R aahat:aahat /srv/aahat && sudo chmod 600 /srv/aahat/api.env /srv/aahat/webhook.secret
sudo install -m 644 /tmp/aahat/Caddyfile /etc/caddy/Caddyfile
sudo install -m 644 /tmp/aahat/aahat-api.service /etc/systemd/system/aahat-api.service
sudo systemctl daemon-reload
sudo systemctl enable --now caddy aahat-api
sudo systemctl restart aahat-api && sudo systemctl reload caddy || sudo systemctl restart caddy
sleep 1; curl -fsS 127.0.0.1:8080/health
REMOTE
echo "== live at https://$SITE_HOST  (officials: /officials, API: /api)"
