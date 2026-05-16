# JVS Documentation

This directory has two audiences:

- Current pre-GA users start in [the repository README](../README.md) and
  [the collapsed CLI spec](02_CLI_SPEC.md).
- Contributors and maintainers use the contract, architecture, operations,
  release, and design indexes below.

`docs/user/` is historical legacy save/restore guidance until it is rewritten
for the collapsed direct model. Do not use it for new release notes, support
answers, or active product guidance.

## User Documentation

The release-facing user path is:

| Document | Purpose |
| --- | --- |
| [README](../README.md) | Current pre-GA public entry point |
| [Overview](00_OVERVIEW.md) | Collapsed active surface and direct-model boundaries |
| [CLI Spec](02_CLI_SPEC.md) | Current release-facing public command surface |
| [AFSCP Direct Contract](contracts/jvs-afscp-direct-v1.md) | Internal trusted platform contract |

The top-level [Quickstart](QUICKSTART.md), [FAQ](FAQ.md),
[Examples](EXAMPLES.md), [Troubleshooting](TROUBLESHOOTING.md), and
`docs/user/` pages are legacy references until rewritten. Keep new active
guidance in the collapsed docs listed above.

## Developer And Maintainer Documentation

Use these entry points when changing JVS, validating release behavior, or
maintaining a published version:

### Current Release-Facing Contracts

These are the source of truth for support answers, QA release gates, user-visible
behavior, public JSON fields, and help text:

| Area | Start here | Use it for |
| --- | --- | --- |
| Specification map | [Overview](00_OVERVIEW.md) | Current release-facing and supporting document map |
| Product contract | [Product Plan](PRODUCT_PLAN.md) | Current product promises and release scope |
| CLI contract | [CLI Spec](02_CLI_SPEC.md) | Public command behavior and JSON fields |
| AFSCP direct contract | [AFSCP Direct Contract](contracts/jvs-afscp-direct-v1.md) | Trusted platform save/list/restore/status/doctor JSON surface |
| Restore contract | [Restore Spec](06_RESTORE_SPEC.md) | Historical preview/run behavior, safety choices, and recovery |
| Cleanup contract | [Cleanup Spec](08_GC_SPEC.md) | Preview/run cleanup behavior and protection reasons |
| Go facade | [API Documentation](API_DOCUMENTATION.md) | Stable library entry points |
| Architecture | [Architecture](ARCHITECTURE.md) | Implementation boundaries and component responsibilities |
| Security | [Security Model](09_SECURITY_MODEL.md) and [Threat Model](10_THREAT_MODEL.md) | Trust boundaries and risk labels |
| Conformance | [Conformance Test Plan](11_CONFORMANCE_TEST_PLAN.md) and [Traceability Matrix](14_TRACEABILITY_MATRIX.md) | Required evidence and test coverage |
| Operations | [Operations Index](ops/README.md) | Maintainer runbooks, migration, and repair flow |
| Release | [Release Index](release/README.md) | Release policy, evidence, changelog, and signing |

### Implemented Design Records

These records explain why current release-facing behavior looks the way it does.
They are useful for engineering, product, and QA, but they are not the public
contract. If a design record conflicts with user docs or `02_CLI_SPEC.md`, the
release-facing contract wins and the design record needs cleanup.

| Area | Start here | Use it for |
| --- | --- | --- |
| Save point/workspace model | [Save Point And Workspace Semantics](21_SAVE_POINT_WORKSPACE_SEMANTICS.md) | Supporting mental model for save points, workspace pointers, and history |
| Explicit workspace folders | [Workspace Path And History Pointer Semantics](22_WORKSPACE_EXPLICIT_PATH_BEHAVIOR.md) | Implemented explicit-folder and `history from` design record |
| Smart copy reporting | [Smart Copy Boundaries](23_FILESYSTEM_AWARE_TRANSFER_PLANNING.md) | Implemented transfer-reporting model and remaining copy-planning refinements |
| Repo clone | [Repo Clone Product Plan](24_REPO_CLONE_PRODUCT_PLAN.md) | Implemented repo clone behavior and release evidence checklist |
| Repo/workspace lifecycle | [Repo/Workspace Lifecycle Product Plan](25_REPO_WORKSPACE_LIFECYCLE_PRODUCT_PLAN.md) | Implemented move, rename, delete, and detach design record |
| External control root | [External Control Root Product Handoff](26_EXTERNAL_CONTROL_METADATA_PRODUCT_PLAN.md) | Implemented operator/platform profile and later lifecycle parity notes |

### Future And Later Work

These documents are not current commitments unless promoted into the
release-facing contracts above:

| Area | Start here | Use it for |
| --- | --- | --- |
| Design index | [Design Index](design/README.md) | Non-release-facing design notes and active design references |
| Product research | [Product Gaps For Next Plan](PRODUCT_GAPS_FOR_NEXT_PLAN.md) | Non-committal product gaps to consider later |
| Later copy planning | [Smart Copy Boundaries](23_FILESYSTEM_AWARE_TRANSFER_PLANNING.md) | Remaining filesystem gates and optimization refinements not required for GA |
| External lifecycle parity | [External Control Root Product Handoff](26_EXTERNAL_CONTROL_METADATA_PRODUCT_PLAN.md) | Later move/rename/delete/detach parity for external control roots |

## Boundary Rules

- User docs use product vocabulary: folder, workspace, save point, history,
  view, restore, cleanup, recovery, and doctor.
- Developer docs may discuss implementation and release mechanics, but should
  not become the first stop for ordinary users.
- Product gaps are research notes, not current commitments.
- Implemented design records are explanatory. They do not override the
  release-facing user docs, CLI spec, or conformance contract.
