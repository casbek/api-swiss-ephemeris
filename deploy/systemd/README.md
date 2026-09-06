# Deploying with systemd

## Install

```sh
# Build on the target machine, or on a machine with the same libc.
# cgo is required, so this cannot be cross compiled from Windows.
CGO_ENABLED=1 go build -trimpath \
  -ldflags "-s -w -X github.com/casbek/api-swiss-ephemeris/internal/version.Version=$(git describe --tags --always)" \
  -o bin/api ./cmd/api

sudo useradd --system --no-create-home --shell /usr/sbin/nologin swisseph
sudo mkdir -p /opt/swisseph-api/bin
sudo cp bin/api /opt/swisseph-api/bin/
sudo cp -r ephe /opt/swisseph-api/
sudo chown -R root:root /opt/swisseph-api

sudo install -d -m 0750 -o root -g root /etc/swisseph-api
printf 'API_KEYS=%s\n' "$(openssl rand -hex 32)" | sudo tee /etc/swisseph-api/secrets.env >/dev/null
sudo chmod 0600 /etc/swisseph-api/secrets.env

sudo cp deploy/systemd/swisseph-api.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now swisseph-api
```

Check it came up:

```sh
curl -s localhost:8080/health | jq
systemctl status swisseph-api
```

The health endpoint runs a real calculation, so a 200 from it means the
ephemeris files are readable, not merely that the process started.

## Logs

The service writes to stdout and never to a file. journald collects the output
and, unlike an application writing its own log, enforces a hard ceiling on how
much disk it can use.

Set the caps in `/etc/systemd/journald.conf`:

```ini
[Journal]
Storage=persistent
Compress=yes
SystemMaxUse=500M
SystemMaxFileSize=50M
MaxRetentionSec=2week
```

Then `sudo systemctl restart systemd-journald`. Journald will never exceed
`SystemMaxUse`; it discards the oldest records instead. There is no logrotate
configuration to forget and no way for the service to fill the disk.

Reading the logs:

```sh
journalctl -u swisseph-api -f              # follow
journalctl -u swisseph-api --since "1 hour ago"
journalctl -u swisseph-api -p err          # errors only
journalctl -u swisseph-api -o json | jq    # structured, in production
```

Request bodies are never logged. They carry birth dates, times and coordinates,
which together identify a person, so a log is the wrong place for them. Use the
request id instead: every response carries `X-Request-Id`, and every log line
for that request repeats it.

```sh
journalctl -u swisseph-api | grep 'request_id=3f2a91c4e8b7d605'
```

## Behind nginx

The service listens on loopback and has no TLS. Terminate TLS in nginx and pass
requests through. See `deploy/nginx/swisseph-api.conf`.

## Upgrading

```sh
sudo systemctl stop swisseph-api
sudo cp bin/api /opt/swisseph-api/bin/
sudo systemctl start swisseph-api
```

The server drains before it exits: on SIGTERM it reports itself unready,
finishes the requests already in flight and then stops, within
`SHUTDOWN_TIMEOUT`.

Deploy a tagged build. The AGPL requires that the source of the running version
be available, and the version endpoint reports the commit it was built from:

```sh
curl -s localhost:8080/v1/license | jq .version
```
