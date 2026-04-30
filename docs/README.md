# JVS Documentation

This directory has two audiences:

- Users start in [docs/user/README.md](user/README.md).
- Contributors and maintainers use the contract, architecture, operations,
  release, and design indexes below.

User tutorials should not depend on the implementation specs. When linking from
an issue, release note, or support answer, send users to `docs/user/` unless
they are explicitly working on JVS itself.

## User Documentation

The release-facing user path is:

| Document | Purpose |
| --- | --- |
| [User Docs](user/README.md) | User documentation index |
| [Quickstart](user/quickstart.md) | First folder, first save point, first restore |
| [Best Practices](user/best-practices.md) | Daily habits for saving, previewing, restoring, workspaces, and cleanup |
| [Concepts](user/concepts.md) | Product vocabulary and mental model |
| [Command Reference](user/commands.md) | Public command reference |
| [Examples](user/examples.md) | Common workflows |
| [Tutorials](user/tutorials.md) | Longer step-by-step project stories |
| [FAQ](user/faq.md) | Frequently asked questions |
| [Troubleshooting](user/troubleshooting.md) | Problems and fixes |
| [Safety](user/safety.md) | What JVS changes, refuses, and protects |
| [Recovery](user/recovery.md) | Interrupted restore recovery |

The top-level [Quickstart](QUICKSTART.md), [FAQ](FAQ.md),
[Examples](EXAMPLES.md), and [Troubleshooting](TROUBLESHOOTING.md) pages are
compatibility bridges to `docs/user/`. Keep new user-facing guidance in
`docs/user/`.

## Developer And Maintainer Documentation

Use these entry points when changing JVS, validating release behavior, or
maintaining a published version:

| Area | Start here | Use it for |
| --- | --- | --- |
| Specification map | [Overview](00_OVERVIEW.md) | Current release-facing and supporting document map |
| Product contract | [Product Plan](PRODUCT_PLAN.md) | Current product promises and release scope |
| CLI contract | [CLI Spec](02_CLI_SPEC.md) | Public command behavior and JSON fields |
| Restore contract | [Restore Spec](06_RESTORE_SPEC.md) | Preview/run behavior, safety choices, and recovery |
| Cleanup contract | [Cleanup Spec](08_GC_SPEC.md) | Preview/run cleanup behavior and protection reasons |
| Go facade | [API Documentation](API_DOCUMENTATION.md) | Stable library entry points |
| Architecture | [Architecture](ARCHITECTURE.md) | Implementation boundaries and component responsibilities |
| Security | [Security Model](09_SECURITY_MODEL.md) and [Threat Model](10_THREAT_MODEL.md) | Trust boundaries and risk labels |
| Conformance | [Conformance Test Plan](11_CONFORMANCE_TEST_PLAN.md) and [Traceability Matrix](14_TRACEABILITY_MATRIX.md) | Required evidence and test coverage |
| Operations | [Operations Index](ops/README.md) | Maintainer runbooks, migration, and repair flow |
| Release | [Release Index](release/README.md) | Release policy, evidence, changelog, and signing |
| Design | [Design Index](design/README.md) | Non-release-facing design notes and active design references |
| Future product research | [Product Gaps For Next Plan](PRODUCT_GAPS_FOR_NEXT_PLAN.md) | Non-committal product gaps to consider later |

## Boundary Rules

- User docs use product vocabulary: folder, workspace, save point, history,
  view, restore, cleanup, recovery, and doctor.
- Developer docs may discuss implementation and release mechanics, but should
  not become the first stop for ordinary users.
- Product gaps are research notes, not current commitments.
