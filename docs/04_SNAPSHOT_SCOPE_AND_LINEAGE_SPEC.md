# Save Point Scope And Provenance Spec

**Status:** release-facing save point scope and provenance contract

The repository filename is retained for manifest stability only. Public docs,
help, examples, JSON, and release notes use `save point`.

## Save Point Identity

A save point ID is the public identifier for saved content. Public commands
that accept `<save>` must require a full save point ID or a unique ID prefix.

Labels, messages, and tags are not restore/view targets.

## Save Point Scope

A save point captures only managed files from one workspace folder.

Excluded from scope:

- JVS control data
- other workspace folders
- runtime state for active operations
- restore plans, recovery plans, and cleanup plans

Published save point content is implementation-owned storage. Storage paths
and field names are not public vocabulary.

## Descriptor Fields

Descriptors are immutable creation records. Public-facing fields must map to
save point terms:

- `save_point_id`
- `workspace`
- `created_at`
- `message`
- `started_from_save_point`
- restored provenance when applicable
- restored path provenance when applicable

## History And Provenance

Workspace history is linear per workspace save flow. Restore does not move or
rewrite history. `workspace new --from <save>` records the source save point
but does not inherit the source workspace history.

Provenance fields have distinct meanings:

- previous save point in the same workspace history
- source save point used to create a workspace
- whole-workspace source restored before saving
- path-level source restored before saving

Cleanup and UI must not collapse these into a branch/merge mental model.

## Integrity Responsibilities

Descriptor checksums and content root hashes protect save point integrity.
`jvs doctor --strict` is the public health command.
