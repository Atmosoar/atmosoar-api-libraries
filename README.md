# atmosoar-api-libraries

Shared Go libraries for Atmosoar services. A single Go module containing ten packages that any Atmosoar Go service can import independently.

## Packages

| Package | Purpose |
|---|---|
| `location` | Parse location query parameters (point, polyline, rectangle, bbox, polygon, WMO station, country shortcut) into typed values. |
| `time` | Parse time query parameters (single time, range with resolution, list, named shortcut) into typed values. |
| `shapefile` | Embedded Natural Earth 1:110m country polygon lookup by name or ISO code. Used internally by `location`, also importable on its own. |
| `observability` | Zap logger, Prometheus HTTP metrics middleware and OTel tracing. FX services import `observability.Module`; non-FX consumers call `ConfigFromEnv`, `NewLogger`, `InitTracing`, `RegisterDefaultCollectors`, `HTTPMetricsMiddleware`, `TracingMiddleware` and `MetricsHandler` directly. `RedactURL`/`RedactURLWithPath`, `WithPathRedactor` and `StartSpan` (non-HTTP spans) were added in v0.5.0. |
| `httputils` | Structured error response envelope and typed error-code constants. Used by every Atmosoar HTTP service to emit identical error shapes. |
| `claims` | Single source of truth for the gateway-trust identity contract: the `X-Atmosoar-User-*` / `X-Atmosoar-Identity-Version` header names the gateway stamps after JWT validation, plus the Gin middleware (`FromHeader`, `RequireAdmin`, `FromContext`) that parses them into a typed `Claims`. Replaced the per-service local `middleware/claims` mirrors. Added in v0.4.0. |
| `runtimeconfig` | Typed, bounds-checked runtime-configuration registry and `Manager` backed by a pluggable `Store` (in-memory, or Postgres via the `runtimeconfig/pgxstore` subpackage). Ships an `fx.Module`. Added in v0.4.0. |
| `admin` | Standard `/admin` REST surface (service info, feature flags, runtime-config get/set) mounted onto a Gin engine via `Register`. Builds on `runtimeconfig` and `claims`. Added in v0.4.0. |
| `observation` | Common, unit-normalised weather observation model (SI units, field names matching observation-api), unit conversions, Magnus dewpoint and plausibility bounds. Used by services that ingest station payloads. Added in v0.5.0. |
| `chart` | Weather-data visualization: one `Spec` rendered three ways — JSON (for browsers), SVG and PNG — from a single shared draw list. Owns the validated colour palette, the reserved status scale and the per-parameter threshold registry, so every service draws the same weather the same way. Time series, meteograms and wind roses. Added in v0.5.0. |

## Install

```bash
go get atmosoar.io/atmosoar-api-libraries@latest
```

Then import the package you need:

```go
import (
    "atmosoar.io/atmosoar-api-libraries/location"
    "atmosoar.io/atmosoar-api-libraries/time"
    "atmosoar.io/atmosoar-api-libraries/shapefile"
    "atmosoar.io/atmosoar-api-libraries/observability"
    "atmosoar.io/atmosoar-api-libraries/httputils"
    "atmosoar.io/atmosoar-api-libraries/claims"
    "atmosoar.io/atmosoar-api-libraries/runtimeconfig"
    "atmosoar.io/atmosoar-api-libraries/admin"
    "atmosoar.io/atmosoar-api-libraries/observation"
    "atmosoar.io/atmosoar-api-libraries/chart"
)
```

## Quick start — observability in an FX service

```go
package main

import (
    "go.uber.org/fx"
    "atmosoar.io/atmosoar-api-libraries/observability"
)

func main() {
    fx.New(
        observability.Module,
        // ... your other modules
    ).Run()
}
```

## Quick start — a chart endpoint

Every service serves the same three encodings from one spec, so a chart is the
same picture whether it is a PNG on a page, an SVG in a doc, or JSON drawn by
recharts in the browser:

```go
spec := chart.FromSamples(chart.KindMeteogram, "Innsbruck", samples,
    []string{"temperature_2m", "wind_speed", "precipitation"})
spec.Mode = chart.ModeDark          // both modes are selected, not derived
spec.Source = "Atmosoar MMA · ICON-D2"

format, ok := chart.ParseFormat(c.Query("format"))   // chart_png | chart_svg | chartspec
if !ok {
    // ... not a chart request
}
data, contentType, err := chart.Encode(spec, format)
```

Colours, units, threshold bands, tick labels and the missing-data rule (a gap,
never a zero) come from the package — a service never picks a hex.

## Development

```bash
go build ./...
go test -race -covermode=atomic -coverprofile=coverage.out ./...
bash .github/workflows/scripts/print_coverage.sh   # fails below MIN_COVERAGE (default 80%)
golangci-lint run ./...                            # v2.15 or newer (built with Go 1.27)
govulncheck ./...
go generate ./shapefile/                           # after replacing the Natural Earth GeoJSON
```

CI (`build-pipeline.yml`, `golangci-lint.yml`) runs on a push to any branch and
on PRs into `master` or `develop`. Total coverage below 80% fails the build.
On `master`/`main` any govulncheck or osv-scanner finding fails the build. On
other branches a high or critical osv-scanner finding (CVSS >= 7) fails and the
rest is reported as a note. An osv-scanner run that errors or produces an
unreadable report fails on every branch. Accepted advisories live in
`osv-scanner.toml`, each with an `ignoreUntil` date.

The embedded `shapefile/ne_110m_admin_0_countries.geojson` is generated: it keeps
only the `NAME` and ISO code properties the lookup reads. Do not edit it by hand;
pass a fresh upstream file to the generator from the `shapefile/` directory
(`go run ./internal/gen -in <download>.geojson`) and commit the result. A test
fails if the embedded file is not in generated form.

**Go version policy:** the `go` directive in `go.mod` is the single source of
truth for the Go version; CI derives from it via `actions/setup-go` with
`go-version-file: go.mod` — never hardcode a Go version in workflow files.
(This repo ships no Dockerfile, so there is nothing else to keep in sync.)

## Versioning

This repo publishes semantic Go module versions. Consumers pin to tagged versions in their `go.mod`.

### Unreleased

- **`location` validates coordinates.** Every parser and the structured JSON
  payload path reject NaN, ±Inf, latitudes outside [-90, 90] and longitudes
  outside [-180, 180]. A `rectangle:-Inf,-Inf,Inf,Inf` used to loop in
  `ExpandLocations` until the process died with a fatal out-of-memory error.
- **`location` caps bbox grids.** New `BboxGridDims` and
  `ExpandBboxToGridChecked` reject a density outside [1, `MaxPointsPerDegree`
  = 100] or a grid above `MaxBboxGridPoints` (1,000,000) *before* allocating.
  `ExpandBboxToGrid` is deprecated; it now clamps the density to 100 and
  halves it until the grid fits instead of allocating without bound (the
  rows/cols it returns describe the grid actually produced). A bbox JSON payload with a
  `density` outside [0, 100] is rejected. Consumers (MMA) should call
  `BboxGridDims` with their own cell budget before expanding.
- **Behaviour fix: polygon expansion covers the whole country.**
  `ExpandLocations` on a polygon used to return the vertices of `Rings[0]`,
  which for a flattened MultiPolygon is whichever part Natural Earth lists
  first: France resolved to French Guiana, Norway to Svalbard, the United
  Kingdom to Northern Ireland. It now returns up to 250 grid points inside the
  polygon (even-odd rule across all rings, holes excluded), falling back to
  the ring vertices when the polygon is smaller than the grid. Responses for
  country shortcuts change content, so release this as a minor version.
- **Behaviour change: `claims.RequireAdmin` no longer fails open on the trust
  switch alone.** With `ATMOSOAR_TRUST_GATEWAY_HEADERS=false` (or a legacy
  `*_TRUST_GATEWAY_HEADERS=false`), `/admin/<svc>/*` still requires the
  gateway's `X-Atmosoar-Admin` header unless `ATMOSOAR_INSECURE_ADMIN=true` is
  set as well. Local docker-compose setups that call admin routes without the
  gateway need both variables. `FromHeader` and `RequireAdmin` log a WARN at
  startup naming the env var that disabled enforcement. The legacy per-service
  trust variables are deprecated; move manifests to the canonical name.
- **Behaviour change: `admin` HTTP contract.** `POST /admin/<svc>/flags/:flag`
  now answers 400 `INVALID_REQUEST` when the body has no boolean `value`
  (`{}` used to switch the flag off with a 200). `PUT /admin/<svc>/config/:key`
  answers 500 `INTERNAL_ERROR` when the store write fails (it was a 400
  `INVALID_VALUE` echoing the database error); validation failures stay 400
  and no longer echo the submitted value for `Secret` keys. Validation errors
  from `runtimeconfig.Manager.Apply` wrap the new `runtimeconfig.ErrInvalidValue`.
- **`runtimeconfig/pgxstore` can target another table.** `NewWithTable`,
  `CreateTableSQLFor` and `EnsureSchemaFor` give an `admin.FlagSet` its own
  table (e.g. `feature_flags`); `New` keeps `runtime_config`. A service that
  wires both FlagSet and Manager over `pgxstore.New` should move the flags.
- **`observability` middlewares survive handler panics.** With `gin.Recovery`
  registered outside them, `HTTPMetricsMiddleware` and `TracingMiddleware` now
  release the in-flight gauge, count the request as a 500 and end the span
  (status Error) before re-panicking. `NewLogger` returns the zap build error
  instead of panicking.
- **pgx stays on v5.10.x.** pgx v5.11.0 added `TypeMap()` to the `pgx.Rows`
  interface, and pgxmock v4.9.0 (latest) does not implement it, so a consumer
  that tests with pgxmock fails `go vet`/`go test` once this module forces
  v5.11. Move to v5.11 only after pgxmock ships a release with `TypeMap`.
- **Go 1.27.2 required.** The `go` directive moved from 1.25.13 to 1.27.2, so
  consumers need a Go 1.27.2 toolchain and a golangci-lint release built with
  Go 1.27 (CI here uses v2.15) to build and lint against this version.
- **`location.ExpandBboxToGrid` is deprecated.** Consumers that lint with
  staticcheck (SA1019) will see a warning until they move to
  `BboxGridDims` / `ExpandBboxToGridChecked`.
- **Polygon sampling is bounded.** Grid sampling of a structured polygon
  payload also fits an edge-test budget (16M point-in-ring tests per pass), so
  a 5,000-vertex self-intersecting payload costs milliseconds instead of about
  300 ms of CPU. Embedded countries keep the full grid.
- **`runtimeconfig.Manager` and `admin.FlagSet` serialise writes.** Two
  concurrent `Apply`/`Clear`/`Set` calls on different keys used to lose one
  change from the live overlay while the store kept both rows. Reads stay
  lock-free.
- **`shapefile` binary size.** The embedded GeoJSON keeps only `NAME` and the
  ISO code properties (839 KB -> 263 KB per consumer binary). Lookups and
  geometry are unchanged.

### v0.6.0

- **Behaviour change (ATMO-311): `time` reads the range resolution as hours.**
  In `start,end,N` the third element used to split the span into N segments; it
  is now an N-hour step, the same meaning as the slash form `start/end/PT<N>H`,
  with the end boundary always included. `2024-01-01T00:00:00Z,2024-01-01T12:00:00Z,3`
  now yields 5 points (00/03/06/09/12) instead of 4. The structured JSON
  `resolution` follows the same rule.
- **Range shortcuts expand hourly.** `BuildRangeTimeShortcuts` builds ranges with
  `Resolution: 1`, so a `range_offset` shortcut such as `3day` expands to 73
  hourly timestamps instead of its two boundaries. A consumer that moves its pin
  from an earlier tag fans out more upstream requests per shortcut and must
  check its own limits.
- **Caps move to the expansion.** More than `MaxTimestamps` (10,000) expanded
  points is a parse error; a step above `MaxResolutionHours` (100 years) is
  rejected before the duration arithmetic can overflow; `ExpandTimes` coarsens a
  hand-built range instead of panicking.

### v0.5.3

- **`observability` latency buckets reach 200 s.** `http_request_duration_seconds`
  keeps the Prometheus default buckets and adds 30, 60, 120 and 200 s (the
  gateway's upstream timeout). Dashboards and alerts see new `le` labels;
  requests slower than 10 s no longer all land in `+Inf`.
- `go.opentelemetry.io/otel` 1.45.0 (and the matching sdk/trace/otlptracegrpc)
  for GO-2026-6505.

### v0.5.2

- `chart` knows `evapotranspiration` (reference ET0, FAO-56, mm/h, bars from
  zero) with the aliases `et0` and `et0_fao_evapotranspiration`, and charts
  `hourly_precipitation` as `precipitation_rate` (mm/h).

### v0.5.0

- New `chart` package: one weather-chart definition rendered as JSON, SVG or PNG,
  with the validated palette, the reserved status scale and the per-parameter
  threshold registry the whole platform now draws from.
- New `observation` package; `observability` gains exported URL redaction, a
  path redactor and non-HTTP spans.
- **`location` caps expansion.** `MaxRadiusKm` is 200 and
  `MaxExpandedRadiusPoints` is 7,000, so `lat,lon|R` with R above 200 km is now
  rejected with an invalid-location error instead of pre-allocating megabytes.
  A service that documented a larger radius has to correct that text. The two
  constants move together: the expansion loop *stops* on the point ceiling
  rather than failing, so a radius whose closed form exceeds it would silently
  return a smaller circle than asked for.

## Provenance

This module was extracted from the Multi-Model API (MMA) repository as part of feature MMA-171. The package contents are ported from MMA's in-tree implementations and preserve byte-for-byte behavior parity where possible — see the MMA-171 spec for the full acceptance criteria.
