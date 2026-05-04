# Performance Guide

**Status:** release-facing save point performance guidance

JVS performance depends on managed-file size, file count, filesystem behavior,
and selected materialization engine. Public docs must not make unconditional
constant-time claims.

## Public Operations

| Operation | Expected shape | Main cost |
| --- | --- | --- |
| `jvs save -m <message>` | O(files + bytes) unless engine/storage optimizes materialization | scanning, copying/cloning, hashing, descriptor publish |
| `jvs history` | O(save points listed) | descriptor reads |
| `jvs view <save> [path]` | engine-dependent materialization | source read and temporary view setup |
| restore preview | O(source + target metadata) | impact calculation and expected evidence |
| restore run | engine-dependent write path | revalidation, backup, materialization |
| `jvs workspace new <folder> --from <save>` | engine-dependent materialization | source read and new workspace creation |
| `jvs doctor --strict` | O(repository state + integrity checks selected) | layout checks, descriptor reads, audit, content verification when strict requires it |

## Engine Classes

| Engine | Performance class | Boundary |
| --- | --- | --- |
| `juicefs-clone` | Fast metadata clone when supported | Requires JuiceFS clone support; no unconditional guarantee |
| `reflink-copy` | Tree walk with copy-on-write data sharing | Requires filesystem reflink support |
| `copy` | Portable recursive copy | Linear in bytes and file count |

## Practical Guidance

- Generated output, dependency caches, and build artifacts inside the
  workspace are saved like other user files; keep high-churn outputs outside
  the workspace when they are not useful save point content.
- Prefer path restore for accidental single-file or single-directory recovery.
- Use `jvs history --path <path>` to find candidates before restore.
- Expect first save of a large folder to be the most expensive operation.
- Re-run restore preview if another agent or process modifies the folder
  before run.
- Resolve active recovery plans before benchmarking restore or cleanup.

## Benchmarking Public Flows

Example local measurements:

```bash
time jvs save -m "benchmark"
time jvs history
time jvs restore <save>
time jvs restore --run <plan-id>
time jvs workspace new ../bench --from <save>
time jvs doctor --strict
```

Restore preview and restore run are separate measurements. Preview should not
be mixed with run because preview changes no files while run performs
revalidation, backup, and materialization.

## Interpreting Results

Report:

- filesystem and mount type
- engine class and fallback reason
- file count and total logical bytes
- platform and storage location
- whether restore timing includes preview, run, or both
- whether recovery plans or cleanup preview plans existed

When comparing runs, use the same engine, filesystem, and data shape. A result
from an internal package benchmark is implementation evidence, not public
vocabulary.

## Scaling Notes

- Save and restore scale with managed-file count and bytes unless the selected
  engine/storage provides metadata clone behavior.
- History and cleanup planning scale with descriptor count.
- Strict doctor can be I/O intensive when it includes full content integrity.
- Cleanup deletion cost depends on candidate save point content size and filesystem
  deletion behavior.

## Boundaries

The public contract does not include:

- explicit engine selection flags
- public partial-save performance contracts
- public compression performance contracts
- complex retention policy tuning
- remote push/pull performance guarantees
