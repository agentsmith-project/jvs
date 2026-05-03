# Design Docs

**Status:** Active reference index, non-release-facing, and not part of the v0 public contract.

Use this index when changing JVS design or reviewing a design decision. It is
not the user learning path; users should start at `../user/README.md`.

Active design entry points:

| Document | Use it for |
| --- | --- |
| `../21_SAVE_POINT_WORKSPACE_SEMANTICS.md` | Detailed save point/workspace semantics and clean redesign source of truth |
| `../22_WORKSPACE_EXPLICIT_PATH_BEHAVIOR.md` | Explicit workspace path behavior handoff |
| `../23_FILESYSTEM_AWARE_TRANSFER_PLANNING.md` | Filesystem-aware transfer planning handoff |
| `../24_REPO_CLONE_PRODUCT_PLAN.md` | Repo clone product handoff |
| `../25_REPO_WORKSPACE_LIFECYCLE_PRODUCT_PLAN.md` | Repo and workspace lifecycle operations product handoff |
| `../26_EXTERNAL_CONTROL_METADATA_PRODUCT_PLAN.md` | Separated control root repo product handoff |
| `../PRODUCT_PLAN.md` | Product contract and release-phase plan |
| `../ARCHITECTURE.md` | Architecture aligned to the save point contract |
| `../02_CLI_SPEC.md` | Public CLI contract |
| `../06_RESTORE_SPEC.md` | Restore preview/run/recovery contract |
| `../PRODUCT_GAPS_FOR_NEXT_PLAN.md` | Non-committal product gaps for future planning |

Historical documents are not product guidance. Active design work should use
the save point vocabulary and treat storage/package names as implementation
facts only.
