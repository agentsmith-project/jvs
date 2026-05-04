# Workspace Spec

**Status:** release-facing workspace contract

The repository filename is retained for manifest stability only. Public docs,
help, examples, JSON, and release notes use `workspace`.

## Public Workspace Contract

- A workspace is a real folder.
- The default workspace is `main`.
- `jvs workspace new <folder> --from <save> [--name <name>]` creates a new
  real workspace folder from a source save point.
- `<folder>` is the explicit target path, must not already exist, and is not
  inferred from the workspace name.
- The workspace name defaults to the target folder basename; `--name <name>`
  overrides it.
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
workspace folders are user-selected real directories created through
`jvs workspace new <folder> --from <save>` and targeted by changing directories
or using `--workspace`.

Workspace folders must not contain JVS control data or runtime state. Save,
restore, view, and cleanup logic must treat that data as outside managed
workspace content.

## Removal And Cleanup Boundary

Workspace removal is not part of the public root-help workflow. Any future
removal flow must preview first, protect unsaved work, run only a reviewed
plan, remove only the selected workspace folder and workspace registry entry,
leave save point storage unchanged, and leave deletion of unprotected save
point storage to reviewed cleanup.
