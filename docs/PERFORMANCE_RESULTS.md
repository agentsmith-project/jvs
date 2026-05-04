# Performance Results

**Status:** release evidence summary with internal benchmark package names

This document summarizes performance evidence. Internal package names such as
`internal/snapshot`, `internal/worktree`, and `internal/gc` identify Go
benchmark locations only. Public UX remains save point, workspace, restore,
doctor, and cleanup.

## GA Result Boundaries (v0)

Performance evidence is regression evidence, not a user-facing service-level
objective. Claims must name the selected engine and filesystem support.

Required engine coverage:

- `juicefs-clone`: qualifies only when JuiceFS clone support is available.
- `reflink-copy`: qualifies only when filesystem copy-on-write is available.
- `copy`: portable fallback, linear in bytes and file count.

Required benchmark package commands:

```bash
go test -bench=. -benchmem ./internal/snapshot/
go test -bench=. -benchmem ./internal/restore/
go test -bench=. -benchmem ./internal/gc/
go test -bench=. -benchmem ./internal/worktree/
```

## Reporting Fields

Report benchmark results with:

- command
- platform
- filesystem
- engine
- file count and total logical bytes
- `ns/op`, `B/op`, and `allocs/op`
- public operation represented by the benchmark

## Public Interpretation

Use public operation names when summarizing results:

- save point creation
- restore preview
- restore run
- workspace creation from save point
- cleanup planning/run
- strict doctor/integrity check

Internal benchmark function names may contain package-specific storage terms.
When they appear, they identify Go functions only and must not be used as
product terminology.

## Known Limits

- Results from one filesystem or mount do not imply the same performance on
  another.
- Copy fallback is linear in content bytes and file count.
- Strict integrity checks can be I/O intensive.
- Cleanup deletion can be dominated by filesystem unlink behavior.
