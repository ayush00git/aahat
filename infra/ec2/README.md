# Aahat on one EC2 instance

Everything is created and updated with the AWS CLI; no console needed.

```bash
cd infra/ec2
./provision.sh                 # key pair, security group, IAM role (Polly + SNS), t4g.small, Elastic IP
./deploy.sh                    # build + ship API, web apps and pipeline results; restart services
AAHAT_SMS=sns ./deploy.sh      # same, with SMS through Amazon SNS turned on
./deploy-pipeline.sh           # ship the pipeline + refresh timer (runs on the server every 2 days)
./teardown.sh                  # delete it all
```

- Region `ap-south-1` (Mumbai), instance `t4g.small` (~$0.02/h). Override in `config.sh` or the environment.
- The site is `https://<ip-with-dashes>.sslip.io`: villager app at `/`, officials' dashboard at `/officials/`,
  API at `/api/`. Caddy gets the certificate automatically.
- Logs: `ssh -i ~/.ssh/aahat-ec2.pem ec2-user@<ip>` then `journalctl -u aahat-api -f` or
  `sudo tail -f /var/log/caddy-access.log`.
- The API runs as user `aahat` with a JSON-file store in `/srv/aahat/state`, data in `/srv/aahat/data`,
  Polly MP3s in `/srv/aahat/state/audio`, and web push keys in `/srv/aahat/state/vapid.json`.
- The sensor webhook secret is generated on the server at `/srv/aahat/webhook.secret`, and the officials'
  token (needed for trigger, subscriber list and alert log; the dashboard asks for it) at `/srv/aahat/official.token`.
- SMS via SNS starts in the sandbox: only verified numbers receive messages
  (`aws sns create-sms-sandbox-phone-number --phone-number +91...`, then `verify-sms-sandbox-phone-number`
  with the OTP), with a $1/month default spend limit.

- The pipeline runs on the server: `systemctl list-timers aahat-refresh.timer`, logs with
  `journalctl -u aahat-refresh`, start one now with `sudo systemctl start aahat-refresh`. It writes to
  `/srv/aahat/data` (lakes, places, barrier scans), which the API serves.
