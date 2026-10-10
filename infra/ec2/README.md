# Aahat on one EC2 instance

Everything is created and updated with the AWS CLI; no console needed.

```bash
cd infra/ec2
./provision.sh                 # key pair, security group, IAM role (Polly + SNS), t4g.small, Elastic IP
./deploy.sh                    # build + ship API, web apps and pipeline results; restart services
AAHAT_SMS=sns ./deploy.sh      # same, with SMS through Amazon SNS turned on
./deploy-pipeline.sh           # ship the pipeline + refresh timer (runs on the server every 2 days)
AAHAT_OPS_EMAIL=you@example.com ./ops-alerts.sh   # failure emails (SNS topic + OnFailure= hook)
./data-bucket.sh               # private S3 bucket for data + state, synced after each refresh and daily
./teardown.sh                  # delete it all (asks separately before deleting the data bucket)
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

## Ops alerts

`ops-alerts.sh` creates the SNS topic `aahat-ops`, subscribes the address in `AAHAT_OPS_EMAIL` (remembered in
`.state`; AWS sends a confirmation email whose link must be clicked once) and installs
`aahat-notify-failure@.service` + `/srv/aahat/bin/notify-failure.sh` on the server. `aahat-api`,
`aahat-refresh` and `aahat-s3-sync` carry `OnFailure=aahat-notify-failure@%n.service`, so a failed unit
emails its name, the host, the UTC time and its last 30 journal lines. The server publishes with its
instance role (AWS CLI v2 ships with Amazon Linux 2023); no mail password anywhere.

- `aahat-api` has `Restart=on-failure`: one crash restarts silently, the email comes when it crash-loops
  into the `failed` state.
- Test: `sudo systemd-run --unit=aahat-failtest -p OnFailure=aahat-notify-failure@aahat-failtest.service /bin/false`
  then `journalctl -u 'aahat-notify-failure@*' -n 5` (expect `sns publish ok`).

## Data bucket

`data-bucket.sh` creates `aahat-data-<account id>` (private, public access blocked, SSE-S3, versioned, old
versions expire after 30 days), lets the server's role read and write it (inline policy `data-bucket`), and
installs `/srv/aahat/bin/s3-sync.sh` with a daily timer (`aahat-s3-sync.timer`, 03:30 UTC). `refresh.sh`
also runs the sync when it finishes. Names of the bucket and topic live in `/srv/aahat/ops.env` on the server.

| Prefix   | Mirrors            | Contents |
|----------|--------------------|----------|
| `data/`  | `/srv/aahat/data`  | pipeline output the API serves: `lakes/`, `places/`, `barrier/`, `inventory/` (deletions are mirrored) |
| `state/` | `/srv/aahat/state` | `store.json` (subscriptions, alert log) and `vapid.json` (web-push keys); never public. `audio/` is skipped: Polly regenerates it |

- Run one now: `sudo systemctl start aahat-s3-sync`, logs with `journalctl -u aahat-s3-sync`.
  Check: `aws s3 ls s3://$DATA_BUCKET --recursive --summarize | tail -2`.
- Restore onto a (new) server, with the API stopped so it does not overwrite the store:
  ```bash
  sudo systemctl stop aahat-api
  sudo -u aahat env HOME=/srv/aahat aws s3 sync s3://$DATA_BUCKET/data  /srv/aahat/data
  sudo -u aahat env HOME=/srv/aahat aws s3 sync s3://$DATA_BUCKET/state /srv/aahat/state
  sudo chmod 600 /srv/aahat/state/*.json && sudo systemctl start aahat-api
  ```
  An older copy of a file: `aws s3api list-object-versions --bucket $DATA_BUCKET --prefix state/store.json`,
  then `aws s3api get-object --version-id ...`.
- The Go API can read the data straight from S3 instead of the local directory: set
  `AAHAT_DATA_BUCKET=<bucket>` and `AAHAT_DATA_PREFIX=data` (the role already has read access). That is also
  the path to open data later: publish only the `data/` prefix (a second public bucket or a CloudFront
  origin limited to `data/*`), never `state/`.
