# ADR-001: Registry accessors alongside deprecated package-level vars

**Status:** Accepted
**Date:** 2026-04-19
**Deciders:** Solution Architect, Library maintainers
**Feature:** BUG-001 (docs/BUGS.md)

## Context

The `location` and `time` packages — extracted from MMA in ATMO-219 — expose mutable package-level registries as exported `var`s: `LocationShortcutsMap`, `StationMap`, `ActiveShapeProvider`, `TimeShortcutsMap`, `TimeRangeShortcutsMap`. Internal lookup paths (`ParseLocation`, `tryPolygonFromShortcut`, `ParseTime`, `parseTimeShortcut`) read these vars without synchronisation. Consumers that mutate a registry after request serving has begun will race with internal reads — `go test -race` would flag this immediately. The library is already published and consumers are pinning tagged versions, so we cannot remove the exported vars without a major-version bump.

## Decision

Add thread-safe `Register*` / `Lookup*` accessors (and `SetShapeProvider` / `ShapeProvider` for the singleton) to both packages, each guarded by a package-level `sync.RWMutex` shared with the existing internal reads. Keep the original exported vars for backwards compatibility, marked `Deprecated:` in godoc, and remove them in v2.

## Alternatives Considered

### Alternative 1: Replace the exported vars outright (breaking change now)
- **Pros:** Single clean API; no deprecation debt; no double-bookkeeping between var and accessor.
- **Cons:** Breaks every existing consumer at the next release; forces a v2 immediately when the library was just extracted.
- **Why not:** ATMO-219 only just shipped the v1 surface. A breaking change before the first round of consumers has even adopted the module destroys the "drop-in consumption" goal in PRD.md.

### Alternative 2: Wrap the existing vars in a `sync.Map` and keep the var name
- **Pros:** Type compatible with most read patterns; no new accessor names.
- **Cons:** `sync.Map` semantics differ from a plain `map` (no `len`, no range without callback); changes the type of the exported var, which is itself a breaking change.
- **Why not:** Breaks consumers that range over the map or call `len()` on it — same blast radius as Alternative 1, with worse ergonomics.

### Alternative 3: Document the vars as "init-time only, do not mutate after serving starts"
- **Pros:** Zero code change; matches how most consumers actually use the vars today.
- **Cons:** Race conditions remain latent; `go test -race` will trip the moment any consumer writes a test that mutates concurrently; no compiler help.
- **Why not:** Unenforceable. The bug report (BUG-001) is High severity precisely because the contract is implicit and easy to violate.

## Consequences

### Positive
- Concurrent registration becomes safe — closes BUG-001.
- Consumers can migrate to accessors at their own pace; no immediate churn.
- v2 has a clear, pre-announced removal list (the five deprecated vars), making the upgrade story crisp.

### Negative
- Two ways to do the same thing for the lifetime of v1 — surface area roughly doubles for these symbols.
- Direct writes to the vars remain race-prone (they take no lock); we accept this for backwards compatibility but call it out in the godoc `Deprecated:` line.
- Existing tests that write directly to the vars stay on the unsafe path. We do not fix them in this round (BUG-001 verification step 2 requires they keep passing).

### Follow-up work
- BUG-001 implementation in `/dev`: add accessors, route internal reads through them, add `-race` test, add `Deprecated:` godoc.
- Track v2 cut: when v2 ships, delete the five exported vars and any tests that wrote to them directly. Note in the v2 release migration guide.
- If a sixth registry is added in the future, it ships with accessors only — no exported var.

## References

- `docs/BUGS.md` — BUG-001
- `location/location.go:107, 112, 115` — exported registry vars
- `time/time.go:69, 72` — exported registry vars
- `docs/PRD.md` — versioning policy (semantic Go module versions)
