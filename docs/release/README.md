# Release Docs

**Status:** Active reference index, non-release-facing, and not part of the v0 public contract.

Use this index when preparing, validating, or publishing a release. Users should
start with `../user/README.md`; release evidence is for maintainers and
contributors.

Active release entry points:

| Document | Use it for |
| --- | --- |
| `../12_RELEASE_POLICY.md` | Release gates and candidate/final evidence rules |
| `../99_CHANGELOG.md` | Release notes |
| `../RELEASE_EVIDENCE.md` | Candidate and final evidence ledger |
| `../SIGNING.md` | Artifact signing and verification |
| `../11_CONFORMANCE_TEST_PLAN.md` | Required conformance evidence |

A source archive is the immutable tag snapshot. It may contain readiness or
candidate evidence from tag time and is not moved to add publication facts.
Publication final evidence belongs on the GitHub Release page and in the
post-release main ledger on `main`, where exact release URL, tag, workflow run,
commit, release state, asset count, checksum, signing identity, smoke, and
coverage facts are recorded.
