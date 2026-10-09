#!/bin/bash
# Ship the Python pipeline to the server with uv and its refresh timer (every 2 days).
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
source "$AAHAT_STATE_FILE"
ROOT=$(cd ../.. && pwd)
SSHOPT="-i $AAHAT_KEY_FILE -o StrictHostKeyChecking=accept-new"
ssh $SSHOPT "ec2-user@$PUBLIC_IP" 'sudo mkdir -p /tmp/aahat-pipeline && sudo chown -R ec2-user /tmp/aahat-pipeline'
rsync -az --delete -e "ssh $SSHOPT" --exclude .venv --exclude out --exclude '__pycache__' --exclude .pytest_cache --exclude .ruff_cache \
  "$ROOT/pipeline/" "ec2-user@$PUBLIC_IP:/tmp/aahat-pipeline/"
scp -q $SSHOPT refresh.sh aahat-refresh.service aahat-refresh.timer "ec2-user@$PUBLIC_IP:/tmp/aahat-pipeline/"
ssh $SSHOPT "ec2-user@$PUBLIC_IP" bash -s <<'REMOTE'
set -euo pipefail
sudo rsync -a --delete --exclude .venv /tmp/aahat-pipeline/ /srv/aahat/pipeline/  # keep the venv: running jobs use it
sudo mkdir -p /srv/aahat/cache   # DEM tiles and the OSM extract download here on the first run
# 2 GB swap: filtering the OSM extract can outgrow a t4g.small's 2 GB of RAM
if ! swapon --show | grep -q swapfile; then
  sudo fallocate -l 2G /swapfile && sudo chmod 600 /swapfile && sudo mkswap /swapfile >/dev/null && sudo swapon /swapfile
  echo '/swapfile none swap defaults 0 0' | sudo tee -a /etc/fstab >/dev/null
fi
sudo install -m 755 /tmp/aahat-pipeline/refresh.sh /srv/aahat/bin/refresh.sh
sudo install -m 644 /tmp/aahat-pipeline/aahat-refresh.service /tmp/aahat-pipeline/aahat-refresh.timer /etc/systemd/system/
sudo chown -R aahat:aahat /srv/aahat
if [ ! -x /srv/aahat/.local/bin/uv ]; then
  sudo -u aahat bash -c 'curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/srv/aahat/.local/bin INSTALLER_NO_MODIFY_PATH=1 sh' >/dev/null
fi
cd /srv/aahat/pipeline && sudo -u aahat env HOME=/srv/aahat /srv/aahat/.local/bin/uv sync --frozen --no-dev 2>&1 | tail -1
sudo -u aahat env HOME=/srv/aahat /srv/aahat/.local/bin/uv run --frozen aahat --help | head -3
sudo systemctl daemon-reload && sudo systemctl enable --now aahat-refresh.timer
systemctl list-timers aahat-refresh.timer --no-pager | head -3
REMOTE
