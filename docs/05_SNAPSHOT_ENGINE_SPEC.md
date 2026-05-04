# Save Point Engine Spec

**Status:** release-facing save point engine behavior

The repository filename is retained for manifest stability only. Public docs,
help, examples, JSON, and release notes use `save point`.

## Engine Classes

JVS chooses an engine for save, view, restore, workspace creation, and internal
maintenance materialization.

| Engine | Behavior | Public promise |
| --- | --- | --- |
| `juicefs-clone` | Uses JuiceFS clone support when available | Engine-scoped fast metadata clone; not unconditional |
| `reflink-copy` | Uses filesystem copy-on-write where supported | Tree walk with shared data blocks when the filesystem supports it |
| `copy` | Portable recursive copy | Linear in bytes and file count |

Public performance docs must scope claims to the selected engine and
filesystem support.

## Engine Transparency

User and automation output should expose:

- effective engine
- fallback or degradation reason
- relevant filesystem warnings
- metadata preservation behavior as it matures

## Save Point Publish Flow

High-level save flow:

1. Resolve workspace and managed-file boundary.
2. Check capacity before staging.
3. Materialize managed files into unpublished staging.
4. Compute content root hash.
5. Build descriptor and descriptor checksum.
6. Publish content and descriptor atomically.
7. Update workspace save point metadata last.
8. Append audit record and clean runtime state.

Storage paths and package names are implementation facts only. They do not
define user behavior, command names, selectors, or workflow concepts.

## Restore Materialization Flow

Restore is plan-bound:

1. Preview computes impact and expected target evidence.
2. Run reloads the plan and revalidates target state.
3. Source save point is protected while read/materialized.
4. A recovery plan and backup are created before files are replaced.
5. Managed files are replaced through the selected engine.
6. History remains unchanged.

## Integrity Markers

Internal publish markers and descriptor checksums let `jvs doctor --strict`
distinguish complete published save points from interrupted staging.
