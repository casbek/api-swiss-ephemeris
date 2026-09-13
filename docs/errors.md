# Errors

Every error this service returns is an [RFC 9457][rfc] problem document, served
as `application/problem+json`. The `type` field of each one links to a section
of this page.

[rfc]: https://www.rfc-editor.org/rfc/rfc9457

```json
{
  "type": "https://github.com/casbek/api-swiss-ephemeris/blob/main/docs/errors.md#validation-failed",
  "title": "Validation failed",
  "status": 422,
  "detail": "The request could not be turned into a moment in time.",
  "instance": "/v1/natal",
  "errors": [
    { "field": "location.latitude", "message": "999 is outside the range -90 to 90" }
  ],
  "request_id": "c9ea0bc6800676ad"
}
```

`title` is a fixed phrase for the kind of error and is safe to match on.
`detail` describes the particular failure and may change between versions, so
show it to a person rather than branching on it. `errors` appears only on a
validation failure and names the fields that were wrong. `instance` is the path
that was called.

`request_id` is worth keeping. Request bodies are never logged — a birth date,
time and coordinate together identify a person, and a log file is the wrong
place to keep that — so when something goes wrong on the server side, the id is
the only handle on the call. It is also returned on success, in the
`X-Request-Id` header. Send your own `X-Request-Id` and it will be echoed back
and used in the logs, which lets you follow one request across your application
and this service.

## Whether to retry

| Status | Retry? |
| --- | --- |
| 400 Bad request | No. The body will be just as malformed next time. |
| 401 Unauthorized | No. Fix the key. |
| 404 Not found | No. |
| 413 Payload too large | No. Send less. |
| 422 Validation failed | No. The same input fails the same way. |
| 429 Too many requests | Yes, after a pause. |
| 500 Internal error | Once, then give up and report the `request_id`. |

Connection failures and timeouts are the ones worth retrying freely; a request
that reached the service and was answered is not.

## Bad request

**400.** The body could not be read as a JSON object at all. Three things cause
it:

- The `Content-Type` header is not `application/json`.
- The body is not valid JSON. `detail` carries the parser's own message,
  including where it gave up: `invalid character 'o' looking for beginning of
  object key string`.
- The body holds something other than exactly one JSON object — nothing at all,
  or a second document after the first.

This is a fault in the client, not in the input. Nothing here depends on
astrology; the request never got as far as being understood as a chart.

## Unauthorized

**401.** Returned when `API_KEYS` is configured and the key presented was
missing or unknown. The response carries `WWW-Authenticate: Bearer realm="api"`.

Send the key either as `X-API-Key: <key>` or as `Authorization: Bearer <key>`.
Keys are compared in constant time, so a wrong key takes as long to reject as a
right one takes to accept; that is deliberate and does not indicate a slow
service.

Two endpoints never require a key. `/health` is exempt because a monitor should
not need credentials to ask whether the service is alive, and `/v1/license` is
exempt because this service is AGPL and section 13 of that licence requires the
offer of source to reach anyone using it over a network, including someone who
has no key.

If no keys are configured at all, authentication is off and every request is
allowed through. That is fine on a loopback address during development and
wrong on anything reachable from outside.

## Not found

**404.** No endpoint matches. `detail` names the method and path that were
tried, and `GET /v1` lists everything the service offers.

It also covers `/v1/reference/{topic}` asked for a topic that does not exist.

**A wrong method lands here too, as 404 rather than 405.** `GET /v1/natal`
answers `No endpoint matches GET /v1/natal.`, because the routes are registered
against a method and nothing else claims that path. Every calculating endpoint
is `POST`: the birth moment, the place and the settings are a structured object,
not a handful of query parameters. If you are getting a 404 on a path you can
see in the index, check the method first.

## Validation failed

**422.** The body parsed, but it does not describe something that can be
calculated. This is the error worth handling properly, because it is the one a
user's input can cause.

`errors` names each field that failed, in dotted path form, alongside a message
meant to be readable:

```json
"errors": [
  { "field": "location.latitude", "message": "999 is outside the range -90 to 90" }
]
```

What lands here, in practice:

- A coordinate outside its range, or a date the ephemeris does not cover.
- A time zone that does not name a zone, or a date and time with no zone at
  all. A date with no time and no zone names no instant. If the birth time is
  genuinely unknown, leave `time` out but send `timezone` anyway; the service
  assumes midday, marks `time_known` false and withholds the houses, which
  depend entirely on the time of day.
- A local time that never happened, because the clocks jumped forward over it,
  or one that happened twice, because they went back. Both are named as such
  rather than being silently resolved one way.
- An unknown body, house system, ayanamsha or aspect. The `/v1/reference/*`
  endpoints list what is accepted.
- A span longer than an endpoint allows. Most searches take fifty years;
  `/v1/retrogrades` and `/v1/transits/search` take ten, because both have to
  sample every body every day and a longer span would hold a calculation thread
  for seconds while other requests waited. Ask for a longer stretch a decade at
  a time.

Do not retry. Show the field and the message to whoever typed the input.

## Payload too large

**413.** The body is over `MAX_BODY_BYTES`, which defaults to one mebibyte.

Nothing this API takes should come anywhere near that. A body that large is
almost always a client sending the wrong thing — a file, or an array where an
object belongs.

## Internal error

**500.** Something failed on this side. `detail` is deliberately vague:

> The request could not be completed. Quote the request id when reporting this.

The specifics go to the log, not into a response that may reach a caller who
should not see them. This includes a panic in a handler, which the middleware
catches and turns into this rather than dropping the connection.

Retry once. If it happens again, quote the `request_id` — that is what makes the
call findable in the log:

```sh
journalctl -u swisseph-api | grep c9ea0bc6800676ad   # systemd
docker compose logs api | grep c9ea0bc6800676ad      # Docker
```

## Too many requests

**429.** This one does not come from the service. The nginx configuration in
`deploy/nginx/swisseph-api.conf` limits each client address to twenty requests a
second with a burst of forty, and answers 429 beyond that.

**It is therefore not a problem document.** nginx sends its own error page, so a
client that assumes every error body is JSON will fail to parse this one. Check
the status before parsing, or check the content type.

Back off and try again. Unlike every other error on this page, a 429 says the
request was fine and the timing was not.

## Errors that are not HTTP errors

Two things worth knowing, because neither shows up as a failed request.

`/health` answers `503` with its own body, not a problem document, in two
cases: the ephemeris files have become unreadable, or the process is draining
during a restart. The `status` field says which — `degraded` or `draining`.

It can also answer `503` simply because the service is busy. The readiness probe
runs a real calculation on the same threads that serve requests, and gives up
after two seconds, so a long enough queue reads as unhealthy. That is the
intended behaviour for a load balancer, but if you see it under load rather than
under fault, the answer is more calculation threads — see
[deploy/README.md](../deploy/README.md#how-much-work-it-can-take-at-once) — not
a longer timeout.

And a successful response can still carry a `warning`. The most important one
reports that a position was computed without the Swiss ephemeris files, using
the built-in Moshier approximation instead. The library does not treat missing
files as an error; it quietly falls back, and the results are close enough to
look right and wrong enough to matter. A `200` with a warning is not a success
to ignore.
