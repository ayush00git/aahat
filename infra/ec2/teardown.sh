#!/bin/bash
# Delete everything provision.sh created (instance, Elastic IP, security group, IAM role, key pair) and the
# ops-alerts topic. The data bucket (data-bucket.sh) is kept unless you confirm its name separately.
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
source "$AAHAT_STATE_FILE"
read -r -p "Delete the Aahat server $INSTANCE_ID ($PUBLIC_IP) and its resources? [y/N] " ok
[ "$ok" = y ] || exit 1
# The data bucket holds the only copy of subscriptions and the lake history once the server is gone
DEL_BUCKET=""
if [ -n "${DATA_BUCKET:-}" ]; then
  read -r -p "ALSO delete the data bucket s3://$DATA_BUCKET and every version in it? Type the bucket name to confirm, or Enter to keep it: " ok2
  [ "$ok2" = "$DATA_BUCKET" ] && DEL_BUCKET=1
fi
aws ec2 disassociate-address --association-id "$(aws ec2 describe-addresses --allocation-ids "$EIP_ALLOC" --query 'Addresses[0].AssociationId' --output text)" || true
aws ec2 release-address --allocation-id "$EIP_ALLOC" || true
aws ec2 terminate-instances --instance-ids "$INSTANCE_ID" >/dev/null && aws ec2 wait instance-terminated --instance-ids "$INSTANCE_ID"
aws ec2 delete-security-group --group-id "$SG_ID" || true
aws iam remove-role-from-instance-profile --instance-profile-name "$AAHAT_NAME-ec2" --role-name "$AAHAT_NAME-ec2" || true
aws iam delete-instance-profile --instance-profile-name "$AAHAT_NAME-ec2" || true
aws iam delete-role-policy --role-name "$AAHAT_NAME-ec2" --policy-name alerts || true
aws iam delete-role-policy --role-name "$AAHAT_NAME-ec2" --policy-name data-bucket 2>/dev/null || true
aws iam delete-role --role-name "$AAHAT_NAME-ec2" || true
aws ec2 delete-key-pair --key-name "$AAHAT_NAME" || true
# Ops alerts: deleting the topic removes its email subscription too
[ -n "${OPS_TOPIC_ARN:-}" ] && { aws sns delete-topic --topic-arn "$OPS_TOPIC_ARN" || true; }
if [ -n "$DEL_BUCKET" ]; then
  # A versioned bucket is only deletable once every version and delete marker is gone
  for kind in Versions DeleteMarkers; do
    while :; do
      batch=$(aws s3api list-object-versions --bucket "$DATA_BUCKET" --max-items 1000 \
        --query "{Objects: ${kind}[].{Key:Key,VersionId:VersionId}, Quiet: \`true\`}" --output json)
      echo "$batch" | grep -q '"Key"' || break
      aws s3api delete-objects --bucket "$DATA_BUCKET" --delete "$batch" >/dev/null
    done
  done
  aws s3api delete-bucket --bucket "$DATA_BUCKET" && echo "deleted s3://$DATA_BUCKET"
elif [ -n "${DATA_BUCKET:-}" ]; then
  echo "kept the data bucket s3://$DATA_BUCKET (delete it by hand, or re-run data-bucket.sh with DATA_BUCKET=$DATA_BUCKET to reuse it)"
fi
rm -f "$AAHAT_STATE_FILE"
echo "deleted"
