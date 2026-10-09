#!/bin/bash
# Delete everything provision.sh created (instance, Elastic IP, security group, IAM role, key pair).
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
source "$AAHAT_STATE_FILE"
read -r -p "Delete the Aahat server $INSTANCE_ID ($PUBLIC_IP) and its resources? [y/N] " ok
[ "$ok" = y ] || exit 1
aws ec2 disassociate-address --association-id "$(aws ec2 describe-addresses --allocation-ids "$EIP_ALLOC" --query 'Addresses[0].AssociationId' --output text)" || true
aws ec2 release-address --allocation-id "$EIP_ALLOC" || true
aws ec2 terminate-instances --instance-ids "$INSTANCE_ID" >/dev/null && aws ec2 wait instance-terminated --instance-ids "$INSTANCE_ID"
aws ec2 delete-security-group --group-id "$SG_ID" || true
aws iam remove-role-from-instance-profile --instance-profile-name "$AAHAT_NAME-ec2" --role-name "$AAHAT_NAME-ec2" || true
aws iam delete-instance-profile --instance-profile-name "$AAHAT_NAME-ec2" || true
aws iam delete-role-policy --role-name "$AAHAT_NAME-ec2" --policy-name alerts || true
aws iam delete-role --role-name "$AAHAT_NAME-ec2" || true
aws ec2 delete-key-pair --key-name "$AAHAT_NAME" || true
rm -f "$AAHAT_STATE_FILE"
echo "deleted"
