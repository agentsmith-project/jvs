# Benchmarks

**Status:** internal benchmark guide; public UX uses save point terminology

This file documents Go benchmark packages. Package and benchmark names identify
code locations only. They are not public CLI vocabulary.

## Package Benchmarks

Run the internal benchmark set:

```bash
go test -run '^$' -bench=. -benchmem -count=5 -benchtime=1s ./internal/snapshot ./internal/restore ./internal/gc ./internal/worktree
```

Packages:

- `./internal/snapshot/` - save point creation internals
- `./internal/restore/` - restore materialization internals
- `./internal/gc/` - cleanup internals
- `./internal/worktree/` - workspace metadata/materialization internals

## Public Interpretation

When publishing benchmark summaries, translate package-specific function names
to public operation names:

- save point creation
- restore preview
- restore run
- workspace creation from a save point
- cleanup planning/run
- strict doctor/integrity checks

Do not use internal benchmark names as user-facing examples.

## Focus Areas

- small and large save point creation
- restore materialization
- path restore materialization
- workspace creation from save point
- cleanup planning and deletion
- strict doctor/integrity costs
- engine fallback costs

## Reporting Format

For each benchmark result, record:

- commit or release candidate
- platform and CPU
- filesystem and mount details
- engine class
- benchmark command
- file count and total logical bytes
- `ns/op`
- `B/op`
- `allocs/op`

## Comparison Workflow

```bash
go test -run '^$' -bench=. -benchmem -count=5 -benchtime=1s ./internal/snapshot > old.txt
go test -run '^$' -bench=. -benchmem -count=5 -benchtime=1s ./internal/snapshot > new.txt
benchstat old.txt new.txt
```

When publishing summaries, translate internal names to public save point terms
and state that package names are implementation facts only.

## Release Evidence Use

Benchmarks are regression evidence. They are not a public guarantee that any
operation is constant-time on every filesystem. Engine-scoped performance
claims must name the engine and filesystem requirement.
