# JVS Release Signing

Official tagged JVS release artifacts produced by the release workflow are signed
with [Sigstore/cosign](https://github.com/sigstore/cosign) to provide
distribution authenticity and integrity verification.

Release artifact signing is separate from the v0 JVS repository format and
from any future in-repository trust model. Descriptor signing, signer trust
policy, and in-JVS key management are not part of the stable v0 public
contract.

## What is Signed?

Each signed release includes:

1. **Binaries** - Pre-built executables for multiple platforms
2. **Binary signatures** - `.sig` files containing signatures for each binary
3. **Binary certificates** - `.pem` files containing X.509 certificates from the signing workflow
4. **Checksums** - `SHA256SUMS` containing SHA-256 hashes for the published `jvs-*` artifacts
5. **Checksums signature** - `SHA256SUMS.sig` and `SHA256SUMS.pem` for the checksums file

## Verification

### Installing cosign

Install the `cosign` tool:

```bash
# macOS/Linux (AMD64)
curl -O -L "https://github.com/sigstore/cosign/releases/latest/download/cosign-$(uname -s)-$(uname -m)"
chmod +x cosign
sudo mv cosign /usr/local/bin/

# Using go install
go install github.com/sigstore/cosign/v2/cmd/cosign@latest
```

### Verifying a Binary

To verify a downloaded binary, download the binary and its matching `.sig` and
`.pem` sidecar files from the same release:

```bash
# Download the binary, signature, and certificate
wget https://github.com/jvs-project/jvs/releases/download/vX.Y.Z/jvs-linux-amd64
wget https://github.com/jvs-project/jvs/releases/download/vX.Y.Z/jvs-linux-amd64.sig
wget https://github.com/jvs-project/jvs/releases/download/vX.Y.Z/jvs-linux-amd64.pem

# Verify using cosign
cosign verify-blob jvs-linux-amd64 \
  --signature jvs-linux-amd64.sig \
  --certificate jvs-linux-amd64.pem \
  --certificate-identity=https://github.com/jvs-project/jvs/.github/workflows/ci.yml@<workflow-ref> \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

Successful verification output:
```
Verified OK
```

### Verifying Checksums

To verify the SHA256SUMS file:

```bash
# Download checksums and signature
wget https://github.com/jvs-project/jvs/releases/download/vX.Y.Z/SHA256SUMS
wget https://github.com/jvs-project/jvs/releases/download/vX.Y.Z/SHA256SUMS.sig
wget https://github.com/jvs-project/jvs/releases/download/vX.Y.Z/SHA256SUMS.pem

# Verify the checksums file
cosign verify-blob SHA256SUMS \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --certificate-identity=https://github.com/jvs-project/jvs/.github/workflows/ci.yml@<workflow-ref> \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

Then verify your binary against the checksums:

```bash
sha256sum -c --ignore-missing SHA256SUMS
```

## Certificate Identity

The release workflow signs artifacts with the following certificate identity
shape:

- **Identity**: `https://github.com/jvs-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- **Issuer**: `https://token.actions.githubusercontent.com`

For tag-push releases, `<workflow-ref>` is the tag ref, for example
`refs/tags/vX.Y.Z`. For manual `workflow_dispatch` releases, the release
workflow checks out and publishes `refs/tags/<tag>`, but the signing
certificate identity can remain the workflow ref that launched the run. Use the
identity printed in the release notes or inspect the downloaded `.pem`
certificate if you need to confirm the exact ref.

This ensures the binary was built and signed by the official JVS CI workflow
running on GitHub Actions.

## Checksum Verification Without Signature Validation

If cosign is not available, checksum validation can detect download corruption
but does not prove release authenticity:

1. Download the `SHA256SUMS` file
2. Download your binary
3. Compare the SHA256 hash:

```bash
sha256sum -c --ignore-missing SHA256SUMS
```

Treat a release artifact as unsigned if the matching `.sig` and `.pem` files are
not present.

## Security Considerations

- **Keyless signing**: JVS uses Sigstore's keyless signing, which eliminates the need for managing private keys
- **OIDC identity**: Signatures are bound to GitHub Actions OIDC identity
- **Reproducibility**: All builds are performed in a transparent CI environment
- **Certificate transparency**: All signing events are recorded in the public Rekor transparency log

## Reporting Issues

If you encounter any verification issues or suspect a compromised release:

1. Do not run the binary
2. Report the issue immediately at [https://github.com/jvs-project/jvs/security/advisories](https://github.com/jvs-project/jvs/security/advisories)
3. Include the version, checksum, and any error messages
