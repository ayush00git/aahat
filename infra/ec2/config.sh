# Shared settings for the EC2 scripts. Override any of these in the environment.
export AWS_REGION="${AWS_REGION:-ap-south-1}"          # Mumbai: closest to users in Himachal
export AAHAT_NAME="${AAHAT_NAME:-aahat}"
export AAHAT_INSTANCE_TYPE="${AAHAT_INSTANCE_TYPE:-t4g.small}"   # 2 vCPU ARM, 2 GB: ~$0.02/h
export AAHAT_KEY_FILE="${AAHAT_KEY_FILE:-$HOME/.ssh/aahat-ec2.pem}"
export AAHAT_STATE_FILE="${AAHAT_STATE_FILE:-$(dirname "${BASH_SOURCE[0]}")/.state}"   # ids of what provision.sh created
