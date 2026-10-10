#!/bin/bash
# Amazon Bedrock (Claude) for the API's /ask assistant and the researcher jobs: pick a Claude model the
# account can call, prove it with one tiny request, let the server's role invoke exactly that model, and
# tell the API which one to use. Safe to re-run.
#
#   ./bedrock-access.sh                      newest working Claude Sonnet/Haiku inference profile in the region
#   AAHAT_BEDROCK_MODEL=<profile or model id> [AAHAT_BEDROCK_REGION=us-west-2] ./bedrock-access.sh
#   AAHAT_BEDROCK_CLASS=haiku ./bedrock-access.sh      only consider that class (sonnet | haiku)
#
# Needs admin credentials locally (aws login). The first call to an Anthropic model in an account also
# subscribes the account to it; if that is refused the script says what to do in the console.
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
source "$AAHAT_STATE_FILE"
save() { grep -v "^export $1=" "$AAHAT_STATE_FILE" > "$AAHAT_STATE_FILE.tmp" || true; echo "export $1=$2" >> "$AAHAT_STATE_FILE.tmp"; mv "$AAHAT_STATE_FILE.tmp" "$AAHAT_STATE_FILE"; export "$1=$2"; }

REGION="${AAHAT_BEDROCK_REGION:-${BEDROCK_REGION:-$AWS_REGION}}"
MODEL="${AAHAT_BEDROCK_MODEL:-}"
CLASS="${AAHAT_BEDROCK_CLASS:-sonnet|haiku}"
ROLE="$AAHAT_NAME-ec2"
ERR=$(mktemp); trap 'rm -f "$ERR"' EXIT

aws sts get-caller-identity --query Arn --output text >/dev/null   # stops here if the login has expired

# One tiny request: prints the model's reply, fails if the model cannot be called.
try_model() {
  aws bedrock-runtime converse --region "$REGION" --model-id "$1" \
    --messages '[{"role":"user","content":[{"text":"Reply with the single word: ok"}]}]' \
    --inference-config maxTokens=10 \
    --query '[output.message.content[0].text, usage.inputTokens, usage.outputTokens]' --output text 2>"$ERR"
}

if [ -n "$MODEL" ]; then
  REPLY=$(try_model "$MODEL") || { echo "cannot call $MODEL in $REGION:"; cat "$ERR"; exit 1; }
else
  # Claude Sonnet/Haiku inference profiles in this region, newest first; take the first that answers.
  CANDIDATES=$(aws bedrock list-inference-profiles --region "$REGION" --output json \
    | jq -r --arg class "$CLASS" '[.inferenceProfileSummaries[]
        | select(.status == "ACTIVE" and (.inferenceProfileId | test("anthropic\\.claude")) and (.inferenceProfileId | test($class)))]
        | sort_by(.createdAt) | reverse | .[].inferenceProfileId')
  if [ -z "$CANDIDATES" ]; then
    echo "no Claude Sonnet/Haiku inference profile in $REGION. Try AAHAT_BEDROCK_REGION=us-west-2 (or us-east-1) ./bedrock-access.sh"; exit 1
  fi
  echo "Claude inference profiles in $REGION, newest first:"; echo "$CANDIDATES" | sed 's/^/  /'
  for candidate in $CANDIDATES; do
    if REPLY=$(try_model "$candidate"); then MODEL="$candidate"; break; fi
    echo "  $candidate: $(tr '\n' ' ' < "$ERR" | cut -c1-220)"
  done
  if [ -z "$MODEL" ]; then
    echo "None of them could be called. What is missing:"
    if ! aws bedrock get-use-case-for-model-access --region "$REGION" >/dev/null 2>&1; then
      echo "- The one-time Anthropic use-case form has not been submitted. AWS console (region $REGION) ->"
      echo "  Amazon Bedrock -> Model catalog -> any Anthropic Claude model -> \"Submit use case details\" (company,"
      echo "  website, who will use it, what for). It is approved within minutes."
    fi
    if grep -qi "being verified\|Operation not allowed" "$ERR"; then
      echo "- Bedrock refuses every model for this account (\"Operation not allowed\" / \"account is being verified\"):"
      echo "  a new AWS account is verified before it may use Bedrock, normally within 2 hours of sign-up. If it"
      echo "  lasts longer, write to aws-verification@amazon.com or open a Support case (Account and billing)."
    fi
    echo "Then run this script again; it needs nothing else."
    exit 1
  fi
fi
echo "model: $MODEL ($REGION)   test reply / input tokens / output tokens: $REPLY"

# What the role must be allowed to invoke: the inference profile itself and the foundation model behind it
# in every region the profile routes to. A plain model id needs only its own ARN.
ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
if PROFILE=$(aws bedrock get-inference-profile --region "$REGION" --inference-profile-identifier "$MODEL" --output json 2>/dev/null); then
  RESOURCES=$(jq -c '[.inferenceProfileArn] + [.models[].modelArn] | unique' <<<"$PROFILE")
else
  RESOURCES=$(jq -nc --arg m "$MODEL" --arg r "$REGION" 'if ($m | startswith("arn:")) then [$m] else ["arn:aws:bedrock:\($r)::foundation-model/\($m)"] end')
fi
# Converse is authorised by bedrock:InvokeModel (there is no separate Converse action).
POLICY=$(jq -nc --argjson res "$RESOURCES" '{Version: "2012-10-17", Statement: [
  {Sid: "InvokeChosenClaudeModel", Effect: "Allow",
   Action: ["bedrock:InvokeModel", "bedrock:InvokeModelWithResponseStream"], Resource: $res}]}')
aws iam put-role-policy --role-name "$ROLE" --policy-name bedrock --policy-document "$POLICY"
echo "role $ROLE, inline policy \"bedrock\":"; jq -r '.[]' <<<"$RESOURCES" | sed 's/^/  /'
save BEDROCK_MODEL "$MODEL"
save BEDROCK_REGION "$REGION"

# Tell the API. deploy.sh rewrites /srv/aahat/api.env, so the settings live in their own file, read
# through a drop-in for the API's unit; the API picks them up at its next restart (deploy.sh restarts it).
SSHOPT="-i $AAHAT_KEY_FILE -o StrictHostKeyChecking=accept-new"
ssh $SSHOPT "ec2-user@$PUBLIC_IP" "MODEL='$MODEL' REGION='$REGION' bash -s" <<'REMOTE'
set -euo pipefail
printf 'AAHAT_BEDROCK_MODEL=%s\nAAHAT_BEDROCK_REGION=%s\n' "$MODEL" "$REGION" | sudo tee /srv/aahat/bedrock.env >/dev/null
sudo chown aahat:aahat /srv/aahat/bedrock.env && sudo chmod 644 /srv/aahat/bedrock.env
sudo mkdir -p /etc/systemd/system/aahat-api.service.d
printf '[Service]\nEnvironmentFile=-/srv/aahat/bedrock.env\n' | sudo tee /etc/systemd/system/aahat-api.service.d/bedrock.conf >/dev/null
sudo systemctl daemon-reload
# The role's new policy can take a few seconds to apply: check from the server itself, as the API will call.
for attempt in 1 2 3 4 5 6; do
  if aws bedrock-runtime converse --region "$REGION" --model-id "$MODEL" \
       --messages '[{"role":"user","content":[{"text":"Reply with the single word: ok"}]}]' \
       --inference-config maxTokens=10 --query 'output.message.content[0].text' --output text 2>/tmp/bedrock.err; then
    echo "the server's role can call the model"; exit 0
  fi
  sleep 5
done
echo "the server's role cannot call the model yet:"; cat /tmp/bedrock.err; exit 1
REMOTE
echo "done. The API uses Bedrock after its next restart: ssh ... sudo systemctl restart aahat-api (or ./deploy.sh)"
