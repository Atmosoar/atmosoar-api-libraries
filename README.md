# atmosoar-api-libraries

Shared Go libraries for Atmosoar services. A single Go module containing ten packages that any Atmosoar Go service can import independently.

## Packages

| Package | Purpose |
|---|---|
| `location` | Parse location query parameters (point, polyline, rectangle, bbox, polygon, WMO station, country shortcut) into typed values. |
| `time` | Parse time query parameters (single time, range with resolution, list, named shortcut) into typed values. |
| `shapefile` | Embedded Natural Earth 1:110m country polygon lookup by name or ISO code. Used internally by `location`, also importable on its own. |
| `observability` | Bootstraps Zap logger, Prometheus HTTP metrics middleware, and OTel tracing with a single `Init` call. `RedactURL`/`RedactURLWithPath`, `WithPathRedactor` and `StartSpan` (non-HTTP spans) are unreleased (develop). Ships an `fx.Module` for FX-based services and plain constructors for non-FX consumers. |
| `httputils` | Structured error response envelope and typed error-code constants. Used by every Atmosoar HTTP service to emit identical error shapes. |
| `claims` | Single source of truth for the gateway-trust identity contract: the `X-Atmosoar-User-*` / `X-Atmosoar-Identity-Version` header names the gateway stamps after JWT validation, plus the Gin middleware (`FromHeader`, `RequireAdmin`, `FromContext`) that parses them into a typed `Claims`. Replaced the per-service local `middleware/claims` mirrors. Added in v0.4.0. |
| `runtimeconfig` | Typed, bounds-checked runtime-configuration registry and `Manager` backed by a pluggable `Store` (in-memory, or Postgres via the `runtimeconfig/pgxstore` subpackage). Ships an `fx.Module`. Added in v0.4.0. |
| `admin` | Standard `/admin` REST surface (service info, feature flags, runtime-config get/set) mounted onto a Gin engine via `Register`. Builds on `runtimeconfig` and `claims`. Added in v0.4.0. |
| `observation` | Common, unit-normalised weather observation model (SI units, field names matching observation-api), unit conversions, Magnus dewpoint and plausibility bounds. Used by services that ingest station payloads. Unreleased (develop). |
| `chart` | Weather-data visualization: one `Spec` rendered three ways — JSON (for browsers), SVG and PNG — from a single shared draw list. Owns the validated colour palette, the reserved status scale and the per-parameter threshold registry, so every service draws the same weather the same way. Time series, meteograms and wind roses. Unreleased (develop). |

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
go test ./...
golangci-lint run
```

**Go version policy:** the `go` directive in `go.mod` is the single source of
truth for the Go version; CI derives from it via `actions/setup-go` with
`go-version-file: go.mod` — never hardcode a Go version in workflow files.
(This repo ships no Dockerfile, so there is nothing else to keep in sync.)

## Versioning

This repo publishes semantic Go module versions. Consumers pin to tagged versions in their `go.mod`.

## Provenance

This module was extracted from the Multi-Model API (MMA) repository as part of feature MMA-171. The package contents are ported from MMA's in-tree implementations and preserve byte-for-byte behavior parity where possible — see the MMA-171 spec for the full acceptance criteria.
