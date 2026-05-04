# Security Policy

## Supported Versions

JVS is currently governed by the v0 public contract. Security fixes target
supported pre-release and v0 tags for that contract.

| Version | Supported |
|---------|-----------|
| Current pre-release and v0 tags | :white_check_mark: Yes |
| Superseded historical tags | :x: No |

## Reporting a Vulnerability

**If you discover a security vulnerability, please do NOT report it via public GitHub issues.**

Instead, please report vulnerabilities responsibly by:

1. **GitHub Security Advisory**: Open a private draft at [GitHub Security Advisory](https://github.com/agentsmith-project/jvs/security/advisories).

2. **Configured security contact**: If this repository has a configured security contact in GitHub security settings, use that channel for sensitive coordination.

3. **Include**: Please provide as much detail as possible to help us understand and reproduce the issue:
   - A clear description of the vulnerability
   - Steps to reproduce the issue
   - Affected versions of JVS
   - Potential impact of the vulnerability
   - Any proof-of-concept code or screenshots (if applicable)

4. **Response Timeline**: We will acknowledge your report within 48 hours and provide a detailed response within 7 days, including:
   - Confirmation of the vulnerability
   - Severity assessment
   - Planned remediation timeline
   - Coordinate disclosure date

## Security Model Overview

JVS is designed with a **save-point-first, filesystem-native** security architecture:

### Integrity Protection (Two-Layer Model)

1. **Descriptor Checksum**: Each save point descriptor includes a SHA-256 checksum covering all descriptor fields
2. **Content Root Hash**: Each save point includes a SHA-256 hash of the complete saved workspace-folder content tree

Verification requires both layers to pass:
```bash
jvs doctor --strict  # Strong repository health and integrity check
```

### Audit Trail

All mutating operations append an audit record to `.jvs/audit/audit.jsonl` with:
- Unique event ID (UUID v4)
- Timestamp (ISO 8601)
- Operation type
- Actor identity
- Hash chain linkage for tamper evidence

Run `jvs doctor --strict` to validate audit chain integrity.

### v0.x Accepted Risks

JVS v0.x intentionally defers some security features to v1.x:

| Feature | v0.x Status | v1.x Plan |
|---------|-------------|-----------|
| Descriptor signing | Not implemented | Ed25519 signatures with trust policy |
| Encryption-at-rest | Out of scope | Filesystem/JuiceFS responsibility |
| In-JVS authn/authz | Out of scope | OS-level permissions |

**Residual Risk**: An attacker with filesystem write access could theoretically rewrite both a descriptor and its checksum consistently. This is an accepted risk for v0.x local-first workflows.

### Filesystem Permissions

JVS relies on OS-level filesystem permissions for access control:

- **Repository access**: Controlled by filesystem permissions on `.jvs/` directory
- **Workspace isolation**: Workspaces are separate directories with standard filesystem permissions
- **JuiceFS integration**: Access control delegated to JuiceFS authentication layer

**Recommendation**: Run `jvs init` in directories with appropriate POSIX permissions (e.g., `0700` for single-user, `0750` for team access).

## Known Security Considerations

1. **No Remote Protocol**: JVS has no network-facing components. Security boundaries are filesystem permissions.

2. **Local-First Design**: All operations assume a trusted local execution environment. JVS does not protect against malicious code running on the same machine.

3. **JuiceFS Dependency**: Ensure JuiceFS mount points are properly secured. Refer to [JuiceFS security documentation](https://juicefs.com/docs/community/security/) for best practices.

4. **Path Traversal Protection**: JVS validates all workspace and save point names to prevent path escape attacks. Rejects `..`, `/`, `\`, and absolute paths.

5. **Crash Safety**: Save point publish uses an atomic protocol with `.READY` file as publish gate. Crashes before `.READY` are ignored; crashes after `.READY` may leave partial save points (detectable via `jvs doctor`).

## Security Best Practices for Users

1. **Run `jvs doctor --strict`** after any suspicious system activity
2. **Run `jvs doctor --strict`** periodically to check repository health
3. **Back up repository metadata safely** only as part of a JVS-aware
   migration procedure: treat non-portable JVS runtime state as
   destination-local, then run `jvs doctor --strict --repair-runtime` on the
   restored destination
4. **Use JuiceFS authentication** to control access to underlying storage
5. **Never commit JVS control data** to Git; it is metadata, not workspace content

## Vulnerability Disclosure Process

1. Report received via private channel
2. Maintainers triage and confirm vulnerability (within 48 hours)
3. Develop fix in private branch
4. Coordinate disclosure date (typically with release)
5. Release fix with security advisory
6. Credit reporter (unless anonymity requested)

## Security Contacts

- **Primary private channel**: https://github.com/agentsmith-project/jvs/security/advisories
- **Configured security contact**: Use the configured security contact in the GitHub repository security settings when available.
- **Security Policy Docs**: See [docs/09_SECURITY_MODEL.md](docs/09_SECURITY_MODEL.md) and [docs/10_THREAT_MODEL.md](docs/10_THREAT_MODEL.md)

## Related Documentation

- [Security Model Specification](docs/09_SECURITY_MODEL.md)
- [Threat Model](docs/10_THREAT_MODEL.md)
- [Conformance Test Plan](docs/11_CONFORMANCE_TEST_PLAN.md) (includes integrity tests)
- [Release Signing and Verification](docs/SIGNING.md) (binary bundle verification)

## Release Verification

Release artifact signing verifies distribution binaries and checksums. It is
separate from descriptor signing or in-JVS trust policy, which are not part of
the stable v0 repository format. For an official release, download the binary
and its matching `.bundle` before using it:

```bash
cosign verify-blob jvs-linux-amd64 \
  --bundle jvs-linux-amd64.bundle \
  --certificate-identity=https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref> \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

For tag-push releases, `<workflow-ref>` is usually `refs/tags/vX.Y.Z`. Manual
`workflow_dispatch` releases may have a certificate identity bound to the
workflow ref that launched the run while the artifacts are checked out from and
published as the requested tag. See [docs/SIGNING.md](docs/SIGNING.md) for the
full verification flow, including `SHA256SUMS.bundle`.

---

*This security policy follows [CNCF best practices](https://github.com/cncf/foundation/blob/main/security-policy.md) and [OpenSSF guidelines](https://github.com/ossf/wg-security-controls/blob/main/SECURITY.md).*
