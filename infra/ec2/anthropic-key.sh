#!/bin/bash
# Give the API an Anthropic API key for the /ask assistant and the researcher jobs (instead of Amazon
# Bedrock). The key is read from a local file that is never in the repo, written to the server as
# /srv/aahat/anthropic.env (mode 600, user aahat), and the API is restarted.
#
#   mkdir -p ~/.config/aahat && $EDITOR ~/.config/aahat/anthropic.key    # paste the key, save
#   ./anthropic-key.sh
#   AAHAT_ANTHROPIC_MODEL=claude-haiku-5-5 ./anthropic-key.sh           # another model (default claude-sonnet-5-5)
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
source "$AAHAT_STATE_FILE"
KEY_FILE="${AAHAT_ANTHROPIC_KEY_FILE:-$HOME/.config/aahat/anthropic.key}"
[ -s "$KEY_FILE" ] || { echo "put the API key in $KEY_FILE first"; exit 1; }
MODEL="${AAHAT_ANTHROPIC_MODEL:-claude-sonnet-5-5}"
SSHOPT="-i $AAHAT_KEY_FILE -o StrictHostKeyChecking=accept-new"
# The key travels on stdin, so it never appears in a process list or in this script's output
tr -d ' \r\n' < "$KEY_FILE" | ssh $SSHOPT "ec2-user@$PUBLIC_IP" "MODEL='$MODEL' bash -c '
  set -euo pipefail
  umask 077
  { printf \"AAHAT_ANTHROPIC_API_KEY=\"; cat; printf \"\nAAHAT_ANTHROPIC_MODEL=%s\n\" \"\$MODEL\"; } | sudo tee /srv/aahat/anthropic.env >/dev/null
  sudo chown aahat:aahat /srv/aahat/anthropic.env && sudo chmod 600 /srv/aahat/anthropic.env
  sudo systemctl daemon-reload && sudo systemctl restart aahat-api
  sleep 2 && curl -fsS localhost:8080/health'"
echo
echo "the API now uses Claude ($MODEL) through the Anthropic API"
