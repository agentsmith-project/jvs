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

1. **Email**: Send a report to `security@jvs-project.org` (if configured) or open a [GitHub Security Advisory](https://github.com/jvs-project/jvs/security/advisories) as a draft.

2. **Include**: Please provide as much detail as possible to help us understand and reproduce the issue:
   - A clear description of the vulnerability
   - Steps to reproduce the issue
   - Affected versions of JVS
   - Potential impact of the vulnerability
   - Any proof-of-concept code or screenshots (if applicable)

3. **Response Timeline**: We will acknowledge your report within 48 hours and provide a detailed response within 7 days, including:
   - Confirmation of the vulnerability
   - Severity assessment
   - Planned remediation timeline
   - Coordinate disclosure date

## Security Model Overview

JVS is designed with a **checkpoint-first, filesystem-native** security architecture:

### Integrity Protection (Two-Layer Model)

1. **Descriptor Checksum**: Each checkpoint descriptor includes a SHA-256 checksum covering all descriptor fields
2. **Payload Root Hash**: Each checkpoint includes a SHA-256 hash of the complete payload directory tree

Verification requires both layers to pass:
```bash
jvs verify --all  # Strong verification (checksum + payload hash)
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

4. **Path Traversal Protection**: JVS validates all workspace and checkpoint names to prevent path escape attacks. Rejects `..`, `/`, `\`, and absolute paths.

5. **Crash Safety**: Checkpoint publish uses a 12-step atomic protocol with `.READY` file as publish gate. Crashes before `.READY` are ignored; crashes after `.READY` may leave partial checkpoints (detectable via `jvs doctor`).

## Security Best Practices for Users

1. **Run `jvs verify --all`** after any suspicious system activity
2. **Run `jvs doctor --strict`** periodically to check repository health
3. **Backup `.jvs/` directory** using `juicefs sync` (excludes runtime state automatically)
4. **Use JuiceFS authentication** to control access to underlying storage
5. **Never commit `.jvs/` directory** to Git (contains metadata, not payload)

## Vulnerability Disclosure Process

1. Report received via private channel
2. Maintainers triage and confirm vulnerability (within 48 hours)
3. Develop fix in private branch
4. Coordinate disclosure date (typically with release)
5. Release fix with security advisory
6. Credit reporter (unless anonymity requested)

## Security Contacts

- **Security Email**: security@jvs-project.org (to be configured)
- **GitHub Security Advisories**: https://github.com/jvs-project/jvs/security/advisories
- **Security Policy Docs**: See [docs/09_SECURITY_MODEL.md](docs/09_SECURITY_MODEL.md) and [docs/10_THREAT_MODEL.md](docs/10_THREAT_MODEL.md)

## Related Documentation

- [Security Model Specification](docs/09_SECURITY_MODEL.md)
- [Threat Model](docs/10_THREAT_MODEL.md)
- [Conformance Test Plan](docs/11_CONFORMANCE_TEST_PLAN.md) (includes integrity tests)
- [Release Signing and Verification](docs/SIGNING.md) (binary signature verification)

## Release Verification

Release signing is not part of the stable v0 public contract yet. If a release
channel publishes Sigstore/cosign artifacts, verify the downloaded binary before
using it:

```bash
cosign verify-blob jvs-linux-amd64 \
  --certificate-identity https://github.com/jvs-project/jvs/.github/workflows/ci.yml@refs/tags/vX.Y.Z \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

See [docs/SIGNING.md](docs/SIGNING.md) for verification instructions when
signature artifacts are available.

---

*This security policy follows [CNCF best practices](https://github.com/cncf/foundation/blob/main/security-policy.md) and [OpenSSF guidelines](https://github.com/ossf/wg-security-controls/blob/main/SECURITY.md).*
