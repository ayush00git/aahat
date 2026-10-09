#!/bin/bash
# First-boot setup on Amazon Linux 2023 (arm64): Caddy for HTTPS, an unprivileged service user,
# and the directory layout deploy.sh fills in.
set -euxo pipefail
dnf -y install tar rsync
CADDY=2.8.4
curl -fsSL "https://github.com/caddyserver/caddy/releases/download/v${CADDY}/caddy_${CADDY}_linux_arm64.tar.gz" | tar -xz -C /usr/local/bin caddy
useradd --system --home /srv/aahat --shell /sbin/nologin aahat || true
mkdir -p /srv/aahat/{bin,data,state,web/villager,web/dashboard} /etc/caddy
chown -R aahat:aahat /srv/aahat
cat > /etc/systemd/system/caddy.service <<'UNIT'
[Unit]
Description=Caddy
After=network-online.target
[Service]
ExecStart=/usr/local/bin/caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
ExecReload=/usr/local/bin/caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
Restart=on-failure
AmbientCapabilities=CAP_NET_BIND_SERVICE
[Install]
WantedBy=multi-user.target
UNIT
