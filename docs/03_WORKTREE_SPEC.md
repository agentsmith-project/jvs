# Workspace Spec

**Status:** release-facing workspace contract

The repository filename is retained for manifest stability only. Public docs,
help, examples, JSON, and release notes use `workspace`.

## Public Workspace Contract

- A workspace is a real folder.
- The default workspace is `main`.
- `jvs workspace new <name> --from <save>` creates a new real workspace from a
  source save point.
- The source workspace is unchanged.
- The new workspace starts with no save point history:
  `newest_save_point` and `history_head` are null until its first save.
- The new workspace records `started_from_save_point`.
- The first save in the new workspace starts a new history and records the
  source save point as provenance, not as an inherited history parent.

## Public Metadata

Public JSON facades should prefer:

- `workspace`
- `folder`
- `content_source`
- `newest_save_point`
- `history_head`
- `started_from_save_point`
- `path_sources`

Implementation metadata is private storage. Its directory names and field names
do not define user behavior or public vocabulary.

## Workspace Folders

The adopted workspace may be the repository root folder itself. Additional
workspace folders are implementation-owned real directories selected through
`jvs workspace new` and `--workspace`.

Workspace payload folders must not contain JVS control data. Save, restore,
view, and cleanup logic must treat JVS control data as ignored/unmanaged.

## Removal And Cleanup Boundary

Workspace removal is not part of the public root-help workflow. Any future
removal flow must preview first, protect unsaved work, and leave save point
deletion to reviewed cleanup.
