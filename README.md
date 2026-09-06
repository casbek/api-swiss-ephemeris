# api-swiss-ephemeris

A high-precision astrology calculation API built on the
[Swiss Ephemeris](https://www.astro.com/swisseph/) 2.10.03 library, written in Go.

> **Status: early development.** The calculation core is complete and verified.
> The HTTP API layer is in progress and endpoints are not stable yet.

## Why

Swiss Ephemeris is the reference implementation for astronomical calculations in
astrology, but it is a C library with global state and no network interface.
This project wraps it in a single self-contained Go binary that speaks JSON over
HTTP, so applications in any language can use it without linking C code or
managing ephemeris files themselves.

## Accuracy

Results are verified against `swetest`, the reference command line tool shipped
with Swiss Ephemeris. The test suite compares planetary longitudes and Placidus
house cusps for a fixed moment and requires agreement to within 1e-6 degrees.

The tests also assert that the `.se1` ephemeris files are actually being read.
Swiss Ephemeris silently falls back to the lower precision Moshier ephemeris
when its data files cannot be found, which is easy to miss and quietly degrades
every result.

## Requirements

- Go 1.27 or newer
- A C compiler (cgo is required; the Swiss Ephemeris sources are vendored and
  compiled as part of the build)
  - Linux: `gcc` and `libc` development headers
  - Windows: MinGW-w64
  - macOS: Xcode command line tools

No external Go modules are used. The build depends only on the standard library
and the vendored C sources.

## Build and test

```sh
go build ./...
go test ./... -race
go test ./internal/swe -run='^$' -bench=. -benchtime=2s
```

A full natal chart (10 bodies plus 12 house cusps) takes roughly 35 µs on a
Ryzen 5 7600X, so calculation cost is negligible compared to HTTP overhead.

## Layout

| Path | Contents |
| --- | --- |
| `internal/libswe` | Vendored Swiss Ephemeris C sources and the raw cgo bindings |
| `internal/swe` | Concurrency-safe wrapper: worker pool and the session API |
| `ephe` | Ephemeris data files covering 1800–2400 CE |

## Thread safety

Swiss Ephemeris keeps global state. On Linux with GCC that state is
thread-local (see the `TLS` definition in `sweodef.h`), which means every OS
thread needs its own `swe_set_ephe_path` call and its own file cache. On
Windows the thread-local storage is disabled and the state is shared across all
threads, so concurrent calls race.

Go's scheduler moves goroutines between OS threads freely, so neither situation
is safe to call into directly. All calculations therefore run on worker
goroutines pinned with `runtime.LockOSThread` and initialised once per thread.
Callers submit work through `Calculator.Do`, which runs an entire chart on a
single thread in one pass.

Per-request settings such as the ayanamsa and the observer location are written
before *every* calculation rather than once at startup, because worker threads
are reused across requests and would otherwise leak one request's settings into
the next.

## License

This project is licensed under the **GNU Affero General Public License v3.0**.
See [LICENSE](LICENSE).

Swiss Ephemeris is made available by its authors under a dual licensing system:
the AGPL, or a separately purchased Swiss Ephemeris Professional License. This
project chooses the AGPL. If you build on this code, that choice applies to
your work as well; if it does not suit you, obtain a Professional License from
Astrodienst before making any public service available.

The original Swiss Ephemeris copyright notice is preserved at
[`internal/libswe/LICENSE`](internal/libswe/LICENSE) and in the vendored source
files, as its terms require.

### Source availability

Section 13 of the AGPL requires that users interacting with this software over
a network be offered its source code. That obligation is met by this
repository. If you operate a modified version of this service, you must publish
your modifications and make them available to your users in the same way.
