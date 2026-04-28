# Contributing to JVS

Thank you for your interest in contributing to JVS (Juicy Versioned Workspaces)!

## Quick Start

1. Create a personal GitHub copy of the repository
2. Clone your GitHub copy: `git clone https://github.com/YOUR_USERNAME/jvs.git`
3. Create a branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Run the local checks listed below
6. Commit: `git commit -m "Add some feature"`
7. Push your branch: `git push origin feature/your-feature-name`
8. Open a Pull Request

## Development Environment

### Prerequisites

- **Go**: Version 1.25.6 or later
- **Operating System**: Linux, macOS, or Windows (with WSL2)
- **Storage**: supported JuiceFS mount (optional but recommended for fast save points)

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

# Run the coverage threshold check
make test-cover
```

**All PRs must pass unit, conformance, lint, and coverage checks before being merged.**

### Test Coverage Requirements

JVS enforces a minimum **60%** coverage threshold in `make test-cover`, matching
the Makefile release gate.

- Overall coverage: minimum **60%** for the v0 line
- Critical paths should have focused tests for their risks
- New features must include tests
- Use `make test-cover` to run the threshold check

### Regression Tests

When fixing bugs, add a regression test to prevent recurrence. See `test/regression/REGRESSION_TESTS.md` for details.

```bash
# Run regression tests
go test -tags conformance -v ./test/regression/...
```

### Fuzzing Tests

Fuzzing tests use randomized inputs to find edge cases and security vulnerabilities. See `test/fuzz/FUZZING.md` for details.

```bash
# List and run release-blocking fuzz smoke targets
make fuzz-list
make fuzz

# Run a specific root-package fuzz target
go test ./test/fuzz -run='^$' -fuzz=FuzzValidateName -fuzztime=1m
```

**Regression test format:**

1. Name the test `TestRegression_<IssueNumber>_<BriefDescription>`
2. Document the bug with a comment block (issue, date fixed, PR)
3. Test the exact scenario that caused the bug
4. Update `test/regression/REGRESSION_TESTS.md` catalog

Example:

```go
// TestRegression_123_SavePointCleanup tests cleanup of unneeded save point storage.
//
// Bug: cleanup left unneeded save point storage after related metadata was removed
// Fixed: 2024-02-20, PR #456
// Issue: #123
func TestRegression_123_SavePointCleanup(t *testing.T) {
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
import "github.com/agentsmith-project/jvs/pkg/errclass"

// Use predefined error classes
return errclass.ErrNameInvalid.WithMessage("workspace name cannot be empty")

// For internal errors, wrap with context
return fmt.Errorf("failed to read descriptor: %w", err)
```

**Common error classes** (from `pkg/errclass/errors.go`):
- `ErrNameInvalid` - Invalid name format
- `ErrPathEscape` - Path traversal attempt
- `ErrDescriptorCorrupt` - Descriptor checksum failed
- `ErrPayloadHashMismatch` - Payload hash check failed
- `ErrLineageBroken` - Save point history relationship is inconsistent
- `ErrFormatUnsupported` - Format version not supported
- `ErrAuditChainBroken` - Audit hash chain validation failed

For the complete list, see `pkg/errclass/errors.go`.

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
feat(save): add save point labels

Users can now label important save points after creation.

Signed-off-by: Your Name <your.email@example.com>
```

### DCO Enforcement

- **CI Check**: All pull requests must pass the DCO check in CI
- **Automatic Check**: CI checks every commit in the PR for proper sign-off
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
feat(save): add save point labels

Users can now label important save points:
  jvs save -m "initial setup"

Labels appear in save point history output.

Fixes #123
```

```
fix(restore): prevent saving from an older restored source

Previously, users could save after restoring from an older source without clear
provenance. Now `jvs save` keeps restored-source provenance explicit.

Users can inspect candidates with `jvs history`, then continue in another
workspace with `jvs workspace new <name> --from <save>`.

Closes #145
```

## Pull Request Process

### Before Opening a PR

1. **Search existing PRs** to avoid duplicates
2. **Discuss large changes** via issue first
3. **Update specs** if changing behavior (docs/*_SPEC.md)
4. **Add tests** for new functionality
5. **Update CHANGELOG** for user-visible changes
6. **Run local checks** (`make test`, `make conformance`, `make lint`, `make test-cover`) and fix any issues
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

1. **Automated checks**: CI runs unit, conformance, lint, and coverage checks
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
│   ├── engine/        # Save point materialization engine abstraction
│   ├── gc/            # Cleanup internals
│   ├── integrity/     # Checksum and hash verification
│   ├── repo/          # Repository management
│   ├── restore/       # Restore operations
│   ├── snapshot/      # Save point creation internals
│   ├── verify/        # Internal integrity helpers
│   └── worktree/      # Workspace metadata internals
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
| `03_WORKTREE_SPEC.md` | Workspace lifecycle |
| `04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md` | Save point identity |
| `05_SNAPSHOT_ENGINE_SPEC.md` | Engine selection (juicefs-clone/reflink/copy) |
| `06_RESTORE_SPEC.md` | Restore semantics |
| `11_CONFORMANCE_TEST_PLAN.md` | Mandatory test requirements |

## Questions?

- **GitHub Issues**: Use [Issues](https://github.com/agentsmith-project/jvs/issues) for bugs and feature requests
- **Discussions**: Use [Discussions](https://github.com/agentsmith-project/jvs/discussions) for questions and ideas

## License

By contributing to JVS, you agree that your contributions will be licensed under the [MIT License](LICENSE).

---

Thank you for contributing to JVS!
