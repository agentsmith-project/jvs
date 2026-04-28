# Release Docs

**Status:** Active reference index, non-release-facing, and not part of the v0 public contract.

Active release entry points:

- `../12_RELEASE_POLICY.md` - release gates and candidate/final evidence rules
- `../99_CHANGELOG.md` - release notes
- `../RELEASE_EVIDENCE.md` - candidate and final evidence ledger
- `../SIGNING.md` - artifact signing and verification

A source archive is the immutable tag snapshot. It may contain readiness or
candidate evidence from tag time and is not moved to add publication facts.
publication final evidence belongs on the GitHub Release page and in the
post-release main ledger on `main`, where exact release URL, tag, workflow run,
commit, release state, asset count, checksum, signing identity, smoke, and
coverage facts are recorded.
