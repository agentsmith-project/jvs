# Contributing to JVS

Thank you for your interest in contributing to JVS (Juicy Versioned Workspaces)!

## Quick Start

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/jvs.git`
3. Create a branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Run tests: `make verify`
6. Commit: `git commit -m "Add some feature"`
7. Push: `git push origin feature/your-feature-name`
8. Open a Pull Request

## Development Environment

### Prerequisites

- **Go**: Version 1.25.6 or later
- **Operating System**: Linux, macOS, or Windows (with WSL2)
- **Storage**: supported JuiceFS mount (optional but recommended for O(1) checkpoints)

### Building

```bash
# Build the jvs binary
make build

# The binary will be output to bin/jvs
./bin/jvs --help
```

### Running Tests

```bash
# Run unit tests
make test

# Run conformance tests (required before merging)
make conformance

# Run linters
make lint

# Run all verification (test + lint)
make verify
```

**All PRs must pass `make verify` before being merged.**

### Test Coverage Requirements

JVS maintains a target of **80%+ test coverage** for production readiness.

- Overall coverage: target **80%+** for the v0 line
- Critical paths must have higher coverage
- New features must include tests
- Use `go test -cover ./...` to check coverage

### Regression Tests

When fixing bugs, add a regression test to prevent recurrence. See `test/regression/REGRESSION_TESTS.md` for details.

```bash
# Run regression tests
go test -tags conformance -v ./test/regression/...
```

### Fuzzing Tests

Fuzzing tests use randomized inputs to find edge cases and security vulnerabilities. See `test/fuzz/FUZZING.md` for details.

```bash
# Run all fuzz tests briefly (smoke test)
go test -fuzz=. -fuzztime=10s ./test/fuzz/...

# Run a specific fuzz target
go test -fuzz=FuzzValidateName -fuzztime=1m ./test/fuzz/...
```

**Regression test format:**

1. Name the test `TestRegression_<IssueNumber>_<BriefDescription>`
2. Document the bug with a comment block (issue, date fixed, PR)
3. Test the exact scenario that caused the bug
4. Update `test/regression/REGRESSION_TESTS.md` catalog

Example:

```go
// TestRegression_123_GarbageCollectionLeak tests GC cleanup.
//
// Bug: GC was not cleaning up orphaned checkpoints when parent was deleted
// Fixed: 2024-02-20, PR #456
// Issue: #123
func TestRegression_123_GarbageCollectionLeak(t *testing.T) {
    // Test the exact scenario that caused the bug
}
```

## Code Style Guidelines

### Go Conventions

JVS follows standard Go conventions:

1. **Effective Go**: Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
2. **gofmt**: All code must be formatted with `gofmt -s -w`
3. **golint**: Use `golangci-lint run` to catch issues
4. **Package names**: Short, lowercase, single words when possible
5. **Error handling**: Never ignore errors, use `errclass` for user-facing errors

### Error Class Usage

JVS uses stable error classes for user-facing errors:

```go
// Import the errclass package
import "github.com/jvs-project/jvs/pkg/errclass"

// Use predefined error classes
return errclass.ErrNameInvalid.WithMessage("workspace name cannot be empty")

// For internal errors, wrap with context
return fmt.Errorf("failed to read descriptor: %w", err)
```

**Available error classes** (from `pkg/errclass/errors.go`):
- `ErrNameInvalid` - Invalid name format
- `ErrPathEscape` - Path traversal attempt
- `ErrDescriptorCorrupt` - Descriptor checksum failed
- `ErrPayloadHashMismatch` - Payload hash verification failed
- `ErrLineageBroken` - Checkpoint lineage inconsistency
- `ErrPartialSnapshot` - Incomplete checkpoint detected
- `ErrGCPlanMismatch` - GC plan ID mismatch
- `ErrFormatUnsupported` - Format version not supported
- `ErrAuditChainBroken` - Audit hash chain validation failed

### Comment Guidelines

- **Public functions**: Must have godoc comments
- **Exported types**: Must have documentation
- **Complex logic**: Add explanatory comments
- **TODOs**: Use `// TODO:` for future work

## Developer Certificate of Origin (DCO)

All contributions to JVS must follow the [Developer Certificate of Origin (DCO)](https://developercertificate.org/). This requirement is part of our commitment to the CNCF/CII Best Practices Badge.

### What is DCO?

DCO is a simple statement that you have the right to submit your contribution and that it follows the project's license (MIT).

### How to Add DCO Sign-off

Every commit must include a `Signed-off-by` line. You can add it automatically:

```bash
# Configure git to automatically sign off commits
git config --local commit.signoff true

# Or manually add sign-off when committing
git commit -m "feat: add new feature" --signoff
```

The sign-off line will look like:

```
feat(checkpoint): add --tag flag for checkpoint tagging

Users can now attach tags during checkpoint creation.

Signed-off-by: Your Name <your.email@example.com>
```

### DCO Enforcement

- **CI Check**: All pull requests must pass the DCO check in CI
- **Automatic Verification**: CI checks every commit in the PR for proper sign-off
- **Failed Checks**: If DCO check fails, amend your commits with sign-off:

```bash
# Amend the most recent commit
git commit --amend --signoff

# Or amend multiple commits interactively
git rebase -i HEAD~n  # Use 'reword' for each commit
```

## Commit Message Conventions

JVS follows a structured commit message format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Test changes (adding/modifying tests)
- `refactor`: Code refactoring (no behavior change)
- `spec`: Specification document changes
- `chore`: Maintenance tasks
- `perf`: Performance improvements

### Examples

```
feat(checkpoint): add --tag flag for checkpoint tagging

Users can now attach tags during checkpoint creation:
  jvs checkpoint "initial setup" --tag v1.0 --tag stable

Tags are stored in the checkpoint descriptor and appear in
`jvs checkpoint list` output.

Fixes #123
```

```
fix(restore): prevent checkpoint creation from historical state

Previously, users could create checkpoints after restoring an older
checkpoint, leading to unclear lineage. Now `jvs checkpoint` returns an
error when the workspace is not at its latest checkpoint.

Users must run `jvs restore latest` or `jvs fork <name>` first.

Closes #145
```

## Pull Request Process

### Before Opening a PR

1. **Search existing PRs** to avoid duplicates
2. **Discuss large changes** via issue first
3. **Update specs** if changing behavior (docs/*_SPEC.md)
4. **Add tests** for new functionality
5. **Update CHANGELOG** for user-visible changes
6. **Run `make verify`** and fix any issues
7. **Ensure all commits have DCO sign-off** (`git commit --signoff`)

### PR Description Template

```markdown
## Summary
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] Conformance tests pass
- [ ] Manual testing completed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Comments added to complex code
- [ ] Documentation updated
- [ ] No new warnings generated
- [ ] Specs updated if applicable
- [ ] CHANGELOG.md updated
- [ ] All commits have DCO sign-off (`Signed-off-by` line)
```

### Review Process

1. **Automated checks**: CI runs `make verify`
2. **Maintainer review**: At least one maintainer must approve
3. **Conformance tests**: All 29 tests must pass
4. **Spec alignment**: Changes must align with `docs/CONSTITUTION.md`

## Project Structure

```
jvs/
├── cmd/jvs/           # Main CLI entry point
├── internal/          # Private implementation
│   ├── audit/         # Audit logging
│   ├── cli/           # CLI command handlers
│   ├── doctor/        # Repository health checks
│   ├── engine/        # Checkpoint engine abstraction
│   ├── gc/            # Garbage collection
│   ├── integrity/     # Checksum and hash verification
│   ├── repo/          # Repository management
│   ├── restore/       # Restore operations
│   ├── snapshot/      # Checkpoint creation
│   ├── verify/        # Verification commands
│   └── worktree/      # Workspace management
├── pkg/               # Public libraries
│   ├── config/        # Configuration
│   ├── errclass/      # Stable error classes
│   ├── fsutil/        # Filesystem utilities
│   ├── jsonutil/      # JSON handling
│   ├── logging/       # Logging utilities
│   ├── model/         # Data models
│   ├── pathutil/      # Path utilities
│   ├── progress/      # Progress reporting
│   └── uuidutil/      # UUID generation
├── test/              # Test suites
│   ├── conformance/   # Conformance tests (29+ mandatory)
│   └── regression/    # Regression tests for fixed bugs
├── docs/              # Specification documents
└── Makefile           # Build automation
```

## Specification Documents

Before modifying behavior, review the relevant spec:

| Document | Purpose |
|----------|---------|
| `CONSTITUTION.md` | Core principles and design governance |
| `00_OVERVIEW.md` | Frozen design decisions |
| `01_REPO_LAYOUT_SPEC.md` | On-disk structure |
| `02_CLI_SPEC.md` | Command contract and error classes |
| `03_WORKTREE_SPEC.md` | Worktree lifecycle |
| `04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md` | Checkpoint identity |
| `05_SNAPSHOT_ENGINE_SPEC.md` | Engine selection (juicefs-clone/reflink/copy) |
| `06_RESTORE_SPEC.md` | Restore semantics |
| `11_CONFORMANCE_TEST_PLAN.md` | Mandatory test requirements |

## Questions?

- **GitHub Issues**: Use [Issues](https://github.com/jvs-project/jvs/issues) for bugs and feature requests
- **Discussions**: Use [Discussions](https://github.com/jvs-project/jvs/discussions) for questions and ideas

## License

By contributing to JVS, you agree that your contributions will be licensed under the [MIT License](LICENSE).

---

Thank you for contributing to JVS!
