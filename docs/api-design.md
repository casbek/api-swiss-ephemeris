# API design

This document is the contract the implementation is written against. It
describes the shapes, names and defaults of the HTTP API. Endpoints are added
over time; the conventions here apply to all of them.

## Scope

The API performs calculation only. It returns positions, houses, aspects and
timing, and it never returns interpretation text. Interpretation is content
rather than computation, it belongs to the application that consumes this API,
and keeping it out means it is not covered by this project's AGPL licence.

## Conventions

- Field names are `snake_case`. Enumerated values are lowercase strings, never
  numeric codes: `"placidus"`, not `"P"`.
- Angles are degrees as JSON numbers. Longitudes are normalised to `[0, 360)`.
- Times are ISO 8601. Any time carrying a `Z` or an offset is authoritative;
  a bare local time must be accompanied by a zone or offset.
- Endpoints that take a chart definition use `POST`, because the input is a
  structured object. Endpoints that read a table or a range use `GET`.
- Every path is prefixed with `/v1`. Breaking changes get a new prefix; new
  optional fields do not.

## Common request objects

### `datetime`

Three forms are accepted. The first is preferred, because only a named zone
carries the historical daylight saving rules that apply to a past date.

```json
{ "date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul" }
{ "date": "1990-06-15", "time": "17:30:00", "utc_offset": "+03:00" }
{ "date": "1990-06-15", "time": "14:30:00Z" }
```

A fixed `utc_offset` is taken at face value and no daylight saving rule is
applied to it. This matters: a birth certificate that records a local clock
time is best expressed with `timezone`, while an offset should only be used
when the offset itself is known to be correct.

Naming the zone more than once is a contradiction, not something to resolve by
precedence, so a request that combines `timezone` with `utc_offset`, or either
of them with an offset inside `time`, is rejected.

### Unknown birth time

`time` may be omitted. Midday local time is then used, `flags.time_known` is
false and a note says so. Midday is the conventional choice because it
minimises the error in the Moon's position across the day. Houses, angles and
anything derived from them are not reported for such a chart: they depend
entirely on the time and would be pure invention.

### Readings that are not instants

Twice a year a local reading does not name one instant, and both cases are
reported in `flags.anomaly` rather than resolved quietly.

`nonexistent` means the clocks went forward over the reading, so it never
occurred. The instant returned is the one the clock showed after the change.
Turkey moved from 03:00 to 04:00 on 27 March 2016, so 03:30 that day is such a
reading.

`ambiguous` means the clocks went back over the reading, so it occurred twice,
and `flags.candidates` lists both instants. Turkey moved from 04:00 back to
03:00 on 8 November 2015, so 03:30 that morning is such a reading, and the two
candidates are an hour apart. The later one is used; a client that knows which
is meant should send `utc_offset` instead.

### Supported years

The ephemeris files shipped with the service cover **1800 to 2399**. A date
outside that range is rejected with a `422`. This is deliberate: Swiss
Ephemeris does not fail outside the range, it falls back to the lower precision
Moshier ephemeris and keeps answering, so accepting the request would mean
quietly returning a worse chart.

### `location`

```json
{ "latitude": 41.0082, "longitude": 28.9784, "altitude_m": 40 }
```

Latitude is positive north, longitude positive east. `altitude_m` defaults to 0
and only affects topocentric positions and rise and set times.

### `settings`

Every field is optional and falls back to the default shown.

```json
{
  "zodiac": "tropical",
  "ayanamsha": "lahiri",
  "house_system": "placidus",
  "bodies": ["sun", "moon", "mercury", "venus", "mars", "jupiter", "saturn",
             "uranus", "neptune", "pluto", "true_node", "chiron", "lilith"],
  "topocentric": false,
  "aspects": {
    "types": ["conjunction", "opposition", "trine", "square", "sextile"],
    "orbs": { "sun": 10, "moon": 10 }
  }
}
```

`ayanamsha` is ignored unless `zodiac` is `"sidereal"`. `orbs` overrides
individual entries of the default table; omitted bodies keep their defaults.

## The position object

Every body in every response uses this shape. Both the raw longitude and its
decomposition into sign and degree are present, so a client never has to
divide the circle itself.

```json
{
  "body": "sun",
  "longitude": 84.2290447,
  "sign": "gemini",
  "sign_index": 2,
  "degree_in_sign": 24.2290447,
  "formatted": "24°13'45\" Gemini",
  "latitude": -0.0000015,
  "distance_au": 1.0158,
  "speed": 0.9550925,
  "is_retrograde": false,
  "declination": 23.2841,
  "house": 8,
  "house_position": 8.53
}
```

`sign_index` is zero based from Aries, so Gemini is 2. `house_position` is the
fractional position within the house, so 8.53 is 53 percent of the way through
the eighth house. `house` and `house_position` are omitted by endpoints that do
not compute houses.

A body's house is decided by which pair of cusps its longitude falls between.
That is what practitioners mean by the house a body is in, and it gives the
same answer in both zodiacs and in every house system. It follows that ecliptic
latitude plays no part, so for a body well off the ecliptic a more refined
method can disagree within a fraction of a degree of a cusp.

## The meta object

Every response carries a `meta` block describing how the result was produced.

```json
{
  "julian_day_ut": 2448058.104166667,
  "utc": "1990-06-15T14:30:00Z",
  "timezone": { "name": "Europe/Istanbul", "offset": "+03:00", "is_dst": true },
  "delta_t_seconds": 57.165747,
  "ephemeris": "Swiss Ephemeris 2.10.03",
  "zodiac": "tropical",
  "house_system": "placidus",
  "source": "https://github.com/casbek/api-swiss-ephemeris"
}
```

This exists for a practical reason. When a client reports that a result differs
from some other site, the answer is almost always a different time zone
resolution, house system or zodiac, and `meta` settles it without a support
conversation. The `source` field also satisfies the AGPL requirement to offer
the source code to anyone interacting with the service over a network.

## Errors

Errors use RFC 9457, served as `application/problem+json`.

```json
{
  "type": "https://github.com/casbek/api-swiss-ephemeris/blob/main/docs/errors.md#unknown-timezone",
  "title": "Unknown time zone",
  "status": 422,
  "detail": "\"Europe/Constantinople\" is not an IANA time zone name.",
  "instance": "/v1/natal",
  "errors": [{ "field": "datetime.timezone", "message": "unknown zone" }]
}
```

Validation failures are `422`. A date outside the range covered by the
ephemeris files is `422` rather than `500`, since it is a property of the
request.

## Endpoints

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/natal` | POST | Full birth chart: bodies, houses, aspects, dignities |
| `/v1/positions` | POST | Body positions only, no houses or aspects |
| `/v1/houses` | POST | House cusps and angles only |
| `/v1/transits` | POST | Transiting positions and their aspects to a natal chart |
| `/v1/transits/search` | POST | Scan a date range for exact transit contacts |
| `/v1/synastry` | POST | Cross aspects between two charts |
| `/v1/composite` | POST | Midpoint composite chart |
| `/v1/progressions` | POST | Secondary progressions and solar arc directions |
| `/v1/returns` | POST | Solar and lunar returns |
| `/v1/ephemeris` | GET | Daily positions across a date range |
| `/v1/moon/phases` | GET | New, first quarter, full and last quarter moments |
| `/v1/retrogrades` | GET | Retrograde stations and periods |
| `/v1/eclipses` | GET | Solar and lunar eclipses |
| `/v1/rise-set` | POST | Rise, set and meridian transit times |
| `/v1/time` | POST | Resolve a local reading into an instant and a Julian Day |
| `/v1/reference/{topic}` | GET | Supported bodies, house systems, ayanamshas, aspects |
| `/v1/license` | GET | Licence terms and a link to the source |
| `/health` | GET | Liveness and readiness |

`/v1/reference/*` lets a client discover what the service supports instead of
asking, which keeps the documentation burden down as options are added.

`/v1/time` exists because time zone handling is where results most often
disagree between services. It exposes the conversion on its own, with the zone,
the offset, the daylight saving state and both Julian Days visible, so a client
can settle a discrepancy without opening a support ticket.

## Default bodies

The default set is the one modern Western practice expects.

| Name | Notes |
| --- | --- |
| `sun` `moon` | Luminaries |
| `mercury` `venus` `mars` `jupiter` `saturn` | Classical planets |
| `uranus` `neptune` `pluto` | Modern planets |
| `true_node` | True lunar node; `mean_node` is also available |
| `chiron` | |
| `lilith` | Mean lunar apogee; `true_lilith` is the osculating apogee |

Also available on request: `ceres`, `pallas`, `juno`, `vesta`, `pholus`,
`earth`, `south_node`, and any numbered asteroid.

## Aspects

Default aspect set and the orb factor applied to each.

| Aspect | Angle | Factor | In default set |
| --- | --- | --- | --- |
| `conjunction` | 0° | 1.0 | yes |
| `opposition` | 180° | 1.0 | yes |
| `trine` | 120° | 1.0 | yes |
| `square` | 90° | 1.0 | yes |
| `sextile` | 60° | 0.75 | yes |
| `quincunx` | 150° | 0.4 | no |
| `semisextile` | 30° | 0.4 | no |
| `semisquare` | 45° | 0.4 | no |
| `sesquiquadrate` | 135° | 0.4 | no |
| `quintile` | 72° | 0.3 | no |
| `biquintile` | 144° | 0.3 | no |

### Orbs

Orbs are a property of the bodies involved, not only of the aspect. Each body
has a base orb; the orb allowed for a pair is the larger of the two, multiplied
by the aspect factor above.

| Body | Base orb |
| --- | --- |
| `sun`, `moon` | 10° |
| `ascendant`, `midheaven` | 8° |
| `mercury`, `venus`, `mars`, `jupiter`, `saturn` | 7° |
| `uranus`, `neptune`, `pluto` | 6° |
| `chiron`, nodes, `lilith`, asteroids | 4° |

Taking the larger of the two means the luminaries keep their wide orbs against
slower bodies, which is what practitioners expect. Any entry can be overridden
per request through `settings.aspects.orbs`.

Each aspect reports whether it is applying or separating, derived from the
relative speed of the two bodies. The moment an aspect perfects is not reported
on a natal chart; finding it needs a search over the ephemeris and belongs with
the transit endpoints, where the question is actually asked.

Where a pair satisfies more than one aspect, which can happen once orbs are
widened, only the tightest is reported. Points that are opposite one another by
construction are skipped entirely: the descendant is defined as the degree
opposite the ascendant, the imum coeli as the degree opposite the midheaven and
the south node as the point opposite the north, so an exact opposition between
any of them holds in every chart ever cast and would head every aspect list
while saying nothing about the chart.

## Essential dignity

Every classical planet carries a `dignity` object giving its domicile,
exaltation, detriment, fall, triplicity by sect, Egyptian bound and Chaldean
face, with the Ptolemaic weights summed into a score. The score is a summary
rather than a verdict; practitioners weigh these differently.

Nothing is reported for the outer planets, the nodes or the asteroids. The
scheme predates their discovery, and the rulerships assigned to them since are
modern additions that practitioners disagree about, so the field is absent
rather than taking a side.

A chart is a day chart when the Sun is above the horizon, which is to say in
houses seven to twelve. That decides which triplicity ruler applies, so the
sect is reported alongside.

## House systems

Every system Swiss Ephemeris implements is exposed. The letter column is the
internal code and is not part of the API.

| API name | Letter | | API name | Letter |
| --- | --- | --- | --- | --- |
| `placidus` | P | | `porphyry` | O |
| `koch` | K | | `alcabitius` | B |
| `whole_sign` | W | | `morinus` | M |
| `equal` | A | | `polich_page` | T |
| `equal_mc` | D | | `krusinski` | U |
| `equal_aries` | N | | `sripati` | S |
| `vehlow` | V | | `sunshine` | I |
| `regiomontanus` | R | | `pullen_sd` | L |
| `campanus` | C | | `pullen_sr` | Q |
| `horizon` | H | | `carter` | F |
| `meridian` | X | | `savard` | J |
| `apc` | Y | | `gauquelin` | G |

Placidus and Koch are undefined above roughly 66 degrees of latitude. A request
for one of them at such a location returns a `422` naming the problem, rather
than silently returning degenerate cusps.

## Ayanamshas

Used when `zodiac` is `"sidereal"`. `lahiri` is the default and by far the most
common in Vedic practice. Also exposed: `fagan_bradley`, `raman`,
`krishnamurti`, `yukteshwar`, `jn_bhasin`, `true_citra`, `true_revati`,
`true_pushya`, `lahiri_icrc`, `deluce`.

## Caching

Results are deterministic: the same input always produces the same output.
Responses therefore carry an `ETag` and a long `Cache-Control` lifetime, and
the service keeps an in-memory cache keyed by a hash of the normalised request.
