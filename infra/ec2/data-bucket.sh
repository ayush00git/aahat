#!/bin/bash
# The data bucket: private, versioned, encrypted, aahat-data-<account id>. Gives the server's role access
# to it and installs the sync script and its daily timer on the server. Safe to re-run.
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
source "$AAHAT_STATE_FILE"
save() { grep -v "^export $1=" "$AAHAT_STATE_FILE" > "$AAHAT_STATE_FILE.tmp" || true; echo "export $1=$2" >> "$AAHAT_STATE_FILE.tmp"; mv "$AAHAT_STATE_FILE.tmp" "$AAHAT_STATE_FILE"; export "$1=$2"; }

BUCKET="${DATA_BUCKET:-$AAHAT_NAME-data-$(aws sts get-caller-identity --query Account --output text)}"
if ! aws s3api head-bucket --bucket "$BUCKET" >/dev/null 2>&1; then
  aws s3api create-bucket --bucket "$BUCKET" --region "$AWS_REGION" \
    --create-bucket-configuration "LocationConstraint=$AWS_REGION" >/dev/null
fi
save DATA_BUCKET "$BUCKET"
aws s3api put-public-access-block --bucket "$BUCKET" --public-access-block-configuration \
  BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
aws s3api put-bucket-encryption --bucket "$BUCKET" --server-side-encryption-configuration \
  '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'
aws s3api put-bucket-versioning --bucket "$BUCKET" --versioning-configuration Status=Enabled
aws s3api put-bucket-lifecycle-configuration --bucket "$BUCKET" --lifecycle-configuration '{"Rules":[
  {"ID":"expire-noncurrent","Status":"Enabled","Filter":{},
   "NoncurrentVersionExpiration":{"NoncurrentDays":30},
   "Expiration":{"ExpiredObjectDeleteMarker":true},
   "AbortIncompleteMultipartUpload":{"DaysAfterInitiation":7}}]}'  >/dev/null
aws s3api put-bucket-tagging --bucket "$BUCKET" --tagging 'TagSet=[{Key=project,Value=aahat}]'

# The server's role: list the bucket, read/write/delete its objects (nothing else in S3)
aws iam put-role-policy --role-name "$AAHAT_NAME-ec2" --policy-name data-bucket --policy-document "{
  \"Version\":\"2012-10-17\",\"Statement\":[
    {\"Effect\":\"Allow\",\"Action\":[\"s3:ListBucket\"],\"Resource\":\"arn:aws:s3:::$BUCKET\"},
    {\"Effect\":\"Allow\",\"Action\":[\"s3:GetObject\",\"s3:PutObject\",\"s3:DeleteObject\"],\"Resource\":\"arn:aws:s3:::$BUCKET/*\"}]}"

SSHOPT="-i $AAHAT_KEY_FILE -o StrictHostKeyChecking=accept-new"
ssh $SSHOPT "ec2-user@$PUBLIC_IP" 'mkdir -p /tmp/aahat-ops'
scp -q $SSHOPT s3-sync.sh aahat-s3-sync.service aahat-s3-sync.timer "ec2-user@$PUBLIC_IP:/tmp/aahat-ops/"
ssh $SSHOPT "ec2-user@$PUBLIC_IP" "DATA_BUCKET=$BUCKET AWS_REGION=$AWS_REGION bash -s" <<'REMOTE'
set -euo pipefail
setenv() { sudo touch /srv/aahat/ops.env; { sudo grep -v "^$1=" /srv/aahat/ops.env || true; echo "$1=$2"; } > /tmp/aahat-ops/ops.env; sudo install -m 644 -o aahat -g aahat /tmp/aahat-ops/ops.env /srv/aahat/ops.env; }
setenv AWS_REGION "$AWS_REGION"
setenv DATA_BUCKET "$DATA_BUCKET"
sudo install -m 755 -o aahat -g aahat /tmp/aahat-ops/s3-sync.sh /srv/aahat/bin/s3-sync.sh
sudo install -m 644 /tmp/aahat-ops/aahat-s3-sync.service /tmp/aahat-ops/aahat-s3-sync.timer /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now aahat-s3-sync.timer
systemctl list-timers aahat-s3-sync.timer --no-pager | head -3
REMOTE
echo "data bucket: s3://$BUCKET  (first sync: ssh ... sudo systemctl start aahat-s3-sync)"
