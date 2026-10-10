#!/bin/bash
# Install the researcher-job worker on the server: the script, its service and the path unit that
# starts it when the API writes a job file into /srv/aahat/state/jobs. Safe to re-run.
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
source "$AAHAT_STATE_FILE"
SSHOPT="-i $AAHAT_KEY_FILE -o StrictHostKeyChecking=accept-new"
ssh $SSHOPT "ec2-user@$PUBLIC_IP" 'mkdir -p /tmp/aahat-research'
scp -q $SSHOPT research-worker.sh research-worker.service research-jobs.path "ec2-user@$PUBLIC_IP:/tmp/aahat-research/"
ssh $SSHOPT "ec2-user@$PUBLIC_IP" 'bash -s' <<'REMOTE'
set -euo pipefail
sudo install -d -m 755 -o aahat -g aahat /srv/aahat/state/jobs /srv/aahat/research
sudo install -m 755 -o aahat -g aahat /tmp/aahat-research/research-worker.sh /srv/aahat/bin/research-worker.sh
sudo install -m 644 /tmp/aahat-research/research-worker.service /tmp/aahat-research/research-jobs.path /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable research-worker.service >/dev/null 2>&1
sudo systemctl enable --now research-jobs.path
systemctl is-active research-jobs.path
REMOTE
echo "research worker installed (jobs: /srv/aahat/state/jobs, results: /srv/aahat/research)"
