# Deploying

Two ways, both on Linux. cgo is required, so the binary cannot be
cross compiled from Windows or macOS for a Linux server; build it on the target
or in the container.

- **[Docker](#docker)** is the shorter path and the one CI exercises on every
  push, so the image is known to build and to answer correctly.
- **[systemd](#systemd)** is the leaner path: one process, no runtime, and the
  logs land in the journal alongside everything else on the machine.

Either way the service listens on the loopback and nginx faces the world. See
[nginx/swisseph-api.conf](nginx/swisseph-api.conf).

## Docker

```sh
git clone https://github.com/casbek/api-swiss-ephemeris.git
cd api-swiss-ephemeris

# A key, so that whatever ends up in front of the service cannot be called by
# anything that merely reaches it.
printf 'API_KEYS=%s\n' "$(openssl rand -hex 32)" > .env
printf 'VERSION=%s\n' "$(git describe --tags --always)" >> .env

docker compose up --build -d
curl -s localhost:8080/health | jq
```

The image is built on `scratch`. It holds the binary and the ephemeris files
and nothing else: no shell, no package manager, no libc. That is worth knowing
for two reasons. There is almost nothing in it to patch, and there is no way to
open a shell inside it to look around, so anything you need to see has to come
out through the logs or the API.

The health check is the binary probing itself, since there is no `curl` in
there to do it. It runs a real calculation, so a healthy container is one whose
ephemeris files are readable, not merely one whose process started.

Upgrading:

```sh
git pull
VERSION=$(git describe --tags --always) docker compose up --build -d
```

Compose stops the old container before starting the new one, so there is a
gap of a second or two. If that matters, run two and let nginx move between
them.

## systemd

For the full recipe, including the hardening the unit file applies and how to
cap the journal so logs cannot fill the disk, see
[systemd/README.md](systemd/README.md).

The short version:

```sh
CGO_ENABLED=1 go build -trimpath \
  -ldflags "-s -w -X github.com/casbek/api-swiss-ephemeris/internal/version.Version=$(git describe --tags --always)" \
  -o bin/api ./cmd/api

sudo useradd --system --no-create-home --shell /usr/sbin/nologin swisseph
sudo mkdir -p /opt/swisseph-api/bin
sudo cp bin/api /opt/swisseph-api/bin/
sudo cp -r ephe /opt/swisseph-api/

sudo install -d -m 0750 /etc/swisseph-api
printf 'API_KEYS=%s\n' "$(openssl rand -hex 32)" \
  | sudo tee /etc/swisseph-api/secrets.env >/dev/null
sudo chmod 0600 /etc/swisseph-api/secrets.env

sudo cp deploy/systemd/swisseph-api.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now swisseph-api
```

## Checking it from the outside

Whichever way it is running, these should all answer.

```sh
BASE=http://localhost:8080
KEY=...   # what you put in API_KEYS

# No key needed: the licence has to be reachable to anyone using the service,
# and a probe should not need credentials.
curl -s $BASE/health | jq '.status, .ephemeris'
curl -s $BASE/v1/license | jq '.license, .source'

# Everything else does.
curl -s -H "X-API-Key: $KEY" $BASE/v1 | jq '.endpoints | length'

curl -s -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"datetime":{"date":"1990-06-15","time":"17:30:00","timezone":"Europe/Istanbul"},
       "location":{"latitude":41.0082,"longitude":28.9784}}' \
  $BASE/v1/natal | jq '.bodies[0].formatted, .meta.timezone'
```

The Sun in that chart is at 84.22904 degrees and the 1990 offset for Istanbul
is `+03:00`. Both are asserted in the test suite and in CI, so if either comes
out differently on your server, something about the deployment is wrong rather
than something about the request.

## What to watch

`GET /health` is the one to point a monitor at. It answers `200` when the
service can calculate, and `503` both when the ephemeris files have become
unreadable and while the process is draining during a restart. A load balancer
reading it will take an instance out of rotation before it stops accepting
connections.

Logs carry the method, the path, the status, the duration and a request id, and
nothing else. Request bodies are never logged: a birth date, time and
coordinate together identify a person, and a log file is the wrong place to
keep that. When a client reports a problem, ask them for the `X-Request-Id`
from the response and search on it.

```sh
docker compose logs -f api                       # Docker
journalctl -u swisseph-api -f                    # systemd
journalctl -u swisseph-api | grep 3f2a91c4e8b7   # one request
```
