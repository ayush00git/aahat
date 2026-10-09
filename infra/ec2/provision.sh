#!/bin/bash
# Create the single Aahat server: key pair, security group, IAM role (Polly + SNS), t4g.small with
# Amazon Linux 2023 arm64, and an Elastic IP. Safe to re-run: existing pieces are reused.
set -euo pipefail
cd "$(dirname "$0")"
source ./config.sh
touch "$AAHAT_STATE_FILE"; source "$AAHAT_STATE_FILE"
save() { grep -v "^export $1=" "$AAHAT_STATE_FILE" > "$AAHAT_STATE_FILE.tmp" || true; echo "export $1=$2" >> "$AAHAT_STATE_FILE.tmp"; mv "$AAHAT_STATE_FILE.tmp" "$AAHAT_STATE_FILE"; export "$1=$2"; }
tag="ResourceType=%s,Tags=[{Key=Name,Value=$AAHAT_NAME},{Key=project,Value=aahat}]"

# SSH key pair
if [ ! -f "$AAHAT_KEY_FILE" ]; then
  aws ec2 create-key-pair --key-name "$AAHAT_NAME" --key-type ed25519 --query KeyMaterial --output text > "$AAHAT_KEY_FILE"
  chmod 600 "$AAHAT_KEY_FILE"
fi

# Security group: HTTP/HTTPS from anywhere, SSH only from this machine's current IP
if [ -z "${SG_ID:-}" ]; then
  VPC=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)
  save SG_ID "$(aws ec2 create-security-group --group-name "$AAHAT_NAME" --description "Aahat web + SSH" --vpc-id "$VPC" \
    --tag-specifications "$(printf "$tag" security-group)" --query GroupId --output text)"
  for port in 80 443; do
    aws ec2 authorize-security-group-ingress --group-id "$SG_ID" --protocol tcp --port $port --cidr 0.0.0.0/0 >/dev/null
  done
fi
# Mobile carriers (carrier-grade NAT) can send SSH from a different public IP than the one an HTTPS
# check reports, so allow a range: AAHAT_SSH_CIDR, or this machine's IP as a /32 by default.
SSH_CIDR="${AAHAT_SSH_CIDR:-$(curl -fsS https://checkip.amazonaws.com)/32}"
aws ec2 authorize-security-group-ingress --group-id "$SG_ID" --protocol tcp --port 22 --cidr "$SSH_CIDR" >/dev/null 2>&1 || true

# IAM role: the server may speak (Polly) and text (SNS) without stored keys
if ! aws iam get-role --role-name "$AAHAT_NAME-ec2" >/dev/null 2>&1; then
  aws iam create-role --role-name "$AAHAT_NAME-ec2" --assume-role-policy-document '{
    "Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}' >/dev/null
  aws iam put-role-policy --role-name "$AAHAT_NAME-ec2" --policy-name alerts --policy-document '{
    "Version":"2012-10-17","Statement":[
      {"Effect":"Allow","Action":["polly:SynthesizeSpeech"],"Resource":"*"},
      {"Effect":"Allow","Action":["sns:Publish"],"Resource":"*"}]}'
  aws iam create-instance-profile --instance-profile-name "$AAHAT_NAME-ec2" >/dev/null
  aws iam add-role-to-instance-profile --instance-profile-name "$AAHAT_NAME-ec2" --role-name "$AAHAT_NAME-ec2"
  sleep 10  # instance profiles take a moment to become usable
fi

# Instance
if [ -z "${INSTANCE_ID:-}" ]; then
  AMI=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64 --query Parameter.Value --output text)
  save INSTANCE_ID "$(aws ec2 run-instances --image-id "$AMI" --instance-type "$AAHAT_INSTANCE_TYPE" \
    --key-name "$AAHAT_NAME" --security-group-ids "$SG_ID" --iam-instance-profile Name="$AAHAT_NAME-ec2" \
    --block-device-mappings 'DeviceName=/dev/xvda,Ebs={VolumeSize=20,VolumeType=gp3}' \
    --metadata-options HttpTokens=required \
    --user-data file://user-data.sh \
    --tag-specifications "$(printf "$tag" instance)" --query 'Instances[0].InstanceId' --output text)"
  aws ec2 wait instance-running --instance-ids "$INSTANCE_ID"
fi

# Fixed public IP
if [ -z "${EIP_ALLOC:-}" ]; then
  save EIP_ALLOC "$(aws ec2 allocate-address --domain vpc --tag-specifications "$(printf "$tag" elastic-ip)" --query AllocationId --output text)"
  aws ec2 associate-address --instance-id "$INSTANCE_ID" --allocation-id "$EIP_ALLOC" >/dev/null
fi
save PUBLIC_IP "$(aws ec2 describe-addresses --allocation-ids "$EIP_ALLOC" --query 'Addresses[0].PublicIp' --output text)"
save SITE_HOST "$(echo "$PUBLIC_IP" | tr . -).sslip.io"   # a hostname for the IP, so Caddy can get a certificate

echo "server: $PUBLIC_IP  site: https://$SITE_HOST  ssh: ssh -i $AAHAT_KEY_FILE ec2-user@$PUBLIC_IP"
