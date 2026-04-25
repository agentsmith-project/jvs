# Sources referenced during design

**Status:** Archived, non-release-facing

> Archived note: this design bibliography is not part of the v0 public contract.
> Release-facing guarantees are defined by the active specs and
> conformance gates; the links below are retained only as non-current design
> context.

The following primary sources inform JVS behavior and constraints.

## JuiceFS
- JuiceFS command reference (clone, sync): https://juicefs.com/docs/community/command_reference
- JuiceFS sync guide: https://juicefs.com/docs/community/guide/sync/
- JuiceFS clone behavior and options: https://juicefs.com/docs/community/guide/clone/

## Linux and filesystem semantics
- Linux `rename(2)`: https://man7.org/linux/man-pages/man2/rename.2.html
- Linux `fsync(2)`: https://man7.org/linux/man-pages/man2/fsync.2.html
- Linux FICLONE ioctl: https://man7.org/linux/man-pages/man2/ioctl_ficlonerange.2.html
- GNU `cp` reflink option: https://www.gnu.org/software/coreutils/manual/html_node/cp-invocation.html

## Security references
- NIST Digital Signature Standard (FIPS 186-5): https://csrc.nist.gov/pubs/fips/186-5/final
- NIST Secure Software Development Framework: https://csrc.nist.gov/Projects/ssdf
- Ed25519 (RFC 8032): https://datatracker.ietf.org/doc/html/rfc8032

## Documentation policy
- Each normative claim in specs SHOULD map to one or more links above.
- Update this file with new source URLs and access date when spec behavior changes.

**Last reviewed:** 2026-02-20
