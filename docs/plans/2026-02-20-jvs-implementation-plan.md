# JVS Go Implementation Plan

**Status:** Archived, non-release-facing

> Archived note: this historical implementation plan is not part of the v0 public contract.
> It is retained only as non-current design context.

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the complete JVS v0.x CLI tool in Go, covering all v6.5 spec requirements with 29 conformance tests passing.

**Architecture:** Layered single-binary CLI with domain packages under `internal/`, shared utilities under `pkg/`, cobra-based CLI, TDD throughout. Exclusive-only isolation, no signing system. See `docs/plans/2026-02-20-jvs-go-implementation-design.md` for full design.

**Tech Stack:** Go 1.26, cobra (CLI), testify (test), x/text (NFC), stdlib crypto/sha256

**Specs:** All specs are in `docs/` directory. Key references per task are noted inline.

---

## Phase 1: Foundation (pkg/ utilities)

### Task 1.1: Project Skeleton and go.mod

**Files:**
- Create: `go.mod`
- Create: `cmd/jvs/main.go`
- Create: `Makefile`

**Step 1: Initialize Go module**

Run:
```bash
cd /home/percy/works/jvs
go mod init github.com/jvs-project/jvs
```

**Step 2: Create main.go stub**

Create `cmd/jvs/main.go`:
```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "jvs: not yet implemented")
	os.Exit(1)
}
```

**Step 3: Create Makefile**

Create `Makefile`:
```makefile
.PHONY: build test lint conformance verify

build:
	go build -o bin/jvs ./cmd/jvs

test:
	go test ./internal/... ./pkg/...

conformance:
	go test -tags conformance -count=1 -v ./test/conformance/...

lint:
	golangci-lint run ./...

verify: test lint
```

**Step 4: Verify build**

Run: `make build`
Expected: `bin/jvs` binary created, exits 1 with message

**Step 5: Commit**

```bash
git add go.mod cmd/ Makefile
git commit -m "chore: initialize Go module and project skeleton"
```

---

### Task 1.2: Error Classes (pkg/errclass/)

**Ref:** `docs/02_CLI_SPEC.md` — Stable error classes

**Files:**
- Create: `pkg/errclass/errors.go`
- Create: `pkg/errclass/errors_test.go`

**Step 1: Write failing test**

Create `pkg/errclass/errors_test.go`:
```go
package errclass_test

import (
	"errors"
	"testing"

	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJVSError_Error(t *testing.T) {
	err := errclass.ErrLockConflict.WithMessage("worktree main is locked")
	assert.Equal(t, "E_LOCK_CONFLICT: worktree main is locked", err.Error())
}

func TestJVSError_Is(t *testing.T) {
	err := errclass.ErrLockConflict.WithMessage("specific message")
	require.True(t, errors.Is(err, errclass.ErrLockConflict))
	require.False(t, errors.Is(err, errclass.ErrLockExpired))
}

func TestJVSError_Code(t *testing.T) {
	assert.Equal(t, "E_LOCK_CONFLICT", errclass.ErrLockConflict.Code)
	assert.Equal(t, "E_FENCING_MISMATCH", errclass.ErrFencingMismatch.Code)
}

func TestJVSError_AllErrorsDefined(t *testing.T) {
	// All 15 v0.x error classes must exist
	all := []error{
		errclass.ErrNameInvalid,
		errclass.ErrPathEscape,
		errclass.ErrLockConflict,
		errclass.ErrLockExpired,
		errclass.ErrLockNotHeld,
		errclass.ErrFencingMismatch,
		errclass.ErrClockSkewExceeded,
		errclass.ErrConsistencyUnavailable,
		errclass.ErrDescriptorCorrupt,
		errclass.ErrPayloadHashMismatch,
		errclass.ErrLineageBroken,
		errclass.ErrPartialSnapshot,
		errclass.ErrGCPlanMismatch,
		errclass.ErrFormatUnsupported,
		errclass.ErrAuditChainBroken,
	}
	assert.Len(t, all, 15)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/errclass/... -v`
Expected: FAIL — package does not exist

**Step 3: Install testify and write implementation**

Run: `go get github.com/stretchr/testify`

Create `pkg/errclass/errors.go`:
```go
package errclass

import "fmt"

// JVSError is a stable, machine-readable error class.
type JVSError struct {
	Code    string
	Message string
}

func (e *JVSError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *JVSError) Is(target error) bool {
	t, ok := target.(*JVSError)
	return ok && e.Code == t.Code
}

// WithMessage returns a new JVSError with the same Code but a specific message.
func (e *JVSError) WithMessage(msg string) *JVSError {
	return &JVSError{Code: e.Code, Message: msg}
}

// WithMessagef returns a new JVSError with a formatted message.
func (e *JVSError) WithMessagef(format string, args ...any) *JVSError {
	return &JVSError{Code: e.Code, Message: fmt.Sprintf(format, args...)}
}

// All stable error classes for v0.x (15 total).
var (
	ErrNameInvalid            = &JVSError{Code: "E_NAME_INVALID"}
	ErrPathEscape             = &JVSError{Code: "E_PATH_ESCAPE"}
	ErrLockConflict           = &JVSError{Code: "E_LOCK_CONFLICT"}
	ErrLockExpired            = &JVSError{Code: "E_LOCK_EXPIRED"}
	ErrLockNotHeld            = &JVSError{Code: "E_LOCK_NOT_HELD"}
	ErrFencingMismatch        = &JVSError{Code: "E_FENCING_MISMATCH"}
	ErrClockSkewExceeded      = &JVSError{Code: "E_CLOCK_SKEW_EXCEEDED"}
	ErrConsistencyUnavailable = &JVSError{Code: "E_CONSISTENCY_UNAVAILABLE"}
	ErrDescriptorCorrupt      = &JVSError{Code: "E_DESCRIPTOR_CORRUPT"}
	ErrPayloadHashMismatch    = &JVSError{Code: "E_PAYLOAD_HASH_MISMATCH"}
	ErrLineageBroken          = &JVSError{Code: "E_LINEAGE_BROKEN"}
	ErrPartialSnapshot        = &JVSError{Code: "E_PARTIAL_SNAPSHOT"}
	ErrGCPlanMismatch         = &JVSError{Code: "E_GC_PLAN_MISMATCH"}
	ErrFormatUnsupported      = &JVSError{Code: "E_FORMAT_UNSUPPORTED"}
	ErrAuditChainBroken       = &JVSError{Code: "E_AUDIT_CHAIN_BROKEN"}
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/errclass/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/errclass/ go.mod go.sum
git commit -m "feat: add stable error classes (pkg/errclass)"
```

---

### Task 1.3: UUID v4 (pkg/uuidutil/)

**Files:**
- Create: `pkg/uuidutil/uuid.go`
- Create: `pkg/uuidutil/uuid_test.go`

**Step 1: Write failing test**

Create `pkg/uuidutil/uuid_test.go`:
```go
package uuidutil_test

import (
	"regexp"
	"testing"

	"github.com/jvs-project/jvs/pkg/uuidutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewV4_Format(t *testing.T) {
	id := uuidutil.NewV4()
	require.Regexp(t, uuidPattern, id)
}

func TestNewV4_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := uuidutil.NewV4()
		assert.False(t, seen[id], "duplicate UUID: %s", id)
		seen[id] = true
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/uuidutil/... -v`
Expected: FAIL

**Step 3: Write implementation**

Create `pkg/uuidutil/uuid.go`:
```go
package uuidutil

import (
	"crypto/rand"
	"fmt"
)

// NewV4 generates a random UUID v4 string.
func NewV4() string {
	var u [16]byte
	if _, err := rand.Read(u[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/uuidutil/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/uuidutil/
git commit -m "feat: add UUID v4 generator (pkg/uuidutil)"
```

---

### Task 1.4: Canonical JSON (pkg/jsonutil/)

**Ref:** `docs/09_SECURITY_MODEL.md` — canonical JSON rules for record_hash and checksum computation

**Files:**
- Create: `pkg/jsonutil/canonical.go`
- Create: `pkg/jsonutil/canonical_test.go`

**Step 1: Write failing tests**

Create `pkg/jsonutil/canonical_test.go`:
```go
package jsonutil_test

import (
	"testing"

	"github.com/jvs-project/jvs/pkg/jsonutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalMarshal_SortedKeys(t *testing.T) {
	input := map[string]any{
		"zebra": 1,
		"alpha": 2,
		"mid":   3,
	}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Equal(t, `{"alpha":2,"mid":3,"zebra":1}`, string(out))
}

func TestCanonicalMarshal_Nested(t *testing.T) {
	input := map[string]any{
		"b": map[string]any{"z": 1, "a": 2},
		"a": 0,
	}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Equal(t, `{"a":0,"b":{"a":2,"z":1}}`, string(out))
}

func TestCanonicalMarshal_NullValue(t *testing.T) {
	input := map[string]any{"key": nil}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Equal(t, `{"key":null}`, string(out))
}

func TestCanonicalMarshal_NoWhitespace(t *testing.T) {
	input := map[string]any{"a": []any{1, 2, 3}}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Equal(t, `{"a":[1,2,3]}`, string(out))
}

func TestCanonicalMarshal_Unicode(t *testing.T) {
	input := map[string]any{"名前": "テスト"}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Contains(t, string(out), "名前")
}

func TestCanonicalMarshal_StructSortsFields(t *testing.T) {
	type sample struct {
		Zebra int    `json:"zebra"`
		Alpha string `json:"alpha"`
	}
	input := sample{Zebra: 1, Alpha: "a"}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	// Keys must be sorted alphabetically regardless of struct field order
	assert.Equal(t, `{"alpha":"a","zebra":1}`, string(out))
}

func TestCanonicalMarshal_Deterministic(t *testing.T) {
	input := map[string]any{"c": 3, "a": 1, "b": 2}
	out1, _ := jsonutil.CanonicalMarshal(input)
	out2, _ := jsonutil.CanonicalMarshal(input)
	assert.Equal(t, string(out1), string(out2))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/jsonutil/... -v`
Expected: FAIL

**Step 3: Write implementation**

Create `pkg/jsonutil/canonical.go`:
```go
package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalMarshal produces deterministic JSON:
// - keys sorted lexicographically
// - no whitespace
// - UTF-8 encoding
// - null serialized as null
func CanonicalMarshal(v any) ([]byte, error) {
	// First marshal to standard JSON to normalize the value
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical marshal: %w", err)
	}

	// Unmarshal into generic structure
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("canonical unmarshal: %w", err)
	}

	// Re-serialize with sorted keys
	var buf bytes.Buffer
	if err := writeCanonical(&buf, generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			keyBytes, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(keyBytes)
			buf.WriteByte(':')
			if err := writeCanonical(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')

	case []any:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')

	default:
		// Primitives: string, float64, bool, nil
		raw, err := json.Marshal(val)
		if err != nil {
			return err
		}
		buf.Write(raw)
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/jsonutil/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/jsonutil/
git commit -m "feat: add canonical JSON serialization (pkg/jsonutil)"
```

---

### Task 1.5: Path and Name Safety (pkg/pathutil/)

**Ref:** `docs/02_CLI_SPEC.md` — Path and name safety; `docs/03_WORKTREE_SPEC.md` — Naming rules

**Files:**
- Create: `pkg/pathutil/validate.go`
- Create: `pkg/pathutil/validate_test.go`

**Step 1: Write failing tests**

Create `pkg/pathutil/validate_test.go`:
```go
package pathutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/pathutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateName_Valid(t *testing.T) {
	valid := []string{"main", "feature-1", "v1.0", "my_branch", "A-Z.test"}
	for _, name := range valid {
		assert.NoError(t, pathutil.ValidateName(name), "should accept: %s", name)
	}
}

func TestValidateName_Empty(t *testing.T) {
	err := pathutil.ValidateName("")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

func TestValidateName_DotDot(t *testing.T) {
	err := pathutil.ValidateName("..")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

func TestValidateName_Separators(t *testing.T) {
	for _, name := range []string{"a/b", "a\\b"} {
		err := pathutil.ValidateName(name)
		require.ErrorIs(t, err, errclass.ErrNameInvalid, "should reject: %s", name)
	}
}

func TestValidateName_ControlChars(t *testing.T) {
	err := pathutil.ValidateName("hello\x00world")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

func TestValidatePathSafety_UnderRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "worktrees", "test")
	require.NoError(t, os.MkdirAll(target, 0755))
	assert.NoError(t, pathutil.ValidatePathSafety(root, target))
}

func TestValidatePathSafety_Escape(t *testing.T) {
	root := t.TempDir()
	err := pathutil.ValidatePathSafety(root, "/tmp/evil")
	require.ErrorIs(t, err, errclass.ErrPathEscape)
}

func TestValidatePathSafety_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "escape")
	os.Symlink("/tmp", link)
	err := pathutil.ValidatePathSafety(root, link)
	require.ErrorIs(t, err, errclass.ErrPathEscape)
}

func TestValidatePathSafety_NonExistentTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "worktrees", "new-wt")
	// parent exists, target does not
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktrees"), 0755))
	assert.NoError(t, pathutil.ValidatePathSafety(root, target))
}
```

**Step 2: Run test to verify it fails**

Run: `go get golang.org/x/text && go test ./pkg/pathutil/... -v`
Expected: FAIL

**Step 3: Write implementation**

Create `pkg/pathutil/validate.go`:
```go
package pathutil

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/jvs-project/jvs/pkg/errclass"
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ValidateName checks worktree/ref name safety per spec 02/03.
func ValidateName(name string) error {
	if name == "" {
		return errclass.ErrNameInvalid.WithMessage("name must not be empty")
	}

	// NFC normalize
	name = norm.NFC.String(name)

	if name == ".." || strings.Contains(name, "..") {
		return errclass.ErrNameInvalid.WithMessagef("name must not contain '..': %s", name)
	}

	if strings.ContainsAny(name, "/\\") {
		return errclass.ErrNameInvalid.WithMessagef("name must not contain separators: %s", name)
	}

	// Check for control characters
	for _, r := range name {
		if unicode.IsControl(r) {
			return errclass.ErrNameInvalid.WithMessagef("name must not contain control characters: %q", name)
		}
	}

	if !nameRegex.MatchString(name) {
		return errclass.ErrNameInvalid.WithMessagef("name must match [a-zA-Z0-9._-]+: %s", name)
	}

	return nil
}

// ValidatePathSafety verifies target path does not escape repo root.
func ValidatePathSafety(repoRoot, targetPath string) error {
	// Resolve repo root symlinks
	resolvedRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return errclass.ErrPathEscape.WithMessagef("cannot resolve repo root: %v", err)
	}

	// Try resolving target; if it doesn't exist, resolve closest ancestor
	resolvedTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			resolvedTarget = resolveClosestAncestor(targetPath)
		} else {
			return errclass.ErrPathEscape.WithMessagef("cannot resolve target: %v", err)
		}
	}

	// Ensure resolved target is under resolved root
	if !strings.HasPrefix(resolvedTarget+"/", resolvedRoot+"/") &&
		resolvedTarget != resolvedRoot {
		return errclass.ErrPathEscape.WithMessagef("path escapes repo root: %s", targetPath)
	}

	return nil
}

// resolveClosestAncestor walks up from path to find the closest existing
// ancestor, resolves it, then appends the remaining components.
func resolveClosestAncestor(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Recurse up
			resolved = resolveClosestAncestor(dir)
		} else {
			return filepath.Clean(path)
		}
	}
	return filepath.Join(resolved, base)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/pathutil/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/pathutil/ go.mod go.sum
git commit -m "feat: add path and name validation (pkg/pathutil)"
```

---

### Task 1.6: Filesystem Utilities (pkg/fsutil/)

**Ref:** `docs/05_SNAPSHOT_ENGINE_SPEC.md` — fsync, atomic publish

**Files:**
- Create: `pkg/fsutil/atomic.go`
- Create: `pkg/fsutil/atomic_test.go`

**Step 1: Write failing tests**

Create `pkg/fsutil/atomic_test.go`:
```go
package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jvs-project/jvs/pkg/fsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []byte(`{"key": "value"}`)

	err := fsutil.AtomicWrite(path, data, 0644)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, content)
}

func TestAtomicWrite_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	os.WriteFile(path, []byte("old"), 0644)

	err := fsutil.AtomicWrite(path, []byte("new"), 0644)
	require.NoError(t, err)

	content, _ := os.ReadFile(path)
	assert.Equal(t, "new", string(content))
}

func TestAtomicWrite_NoTmpLeftOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	fsutil.AtomicWrite(path, []byte("data"), 0644)

	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 1, "only the target file should exist")
}

func TestRenameAndSync(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.WriteFile(src, []byte("data"), 0644)

	err := fsutil.RenameAndSync(src, dst)
	require.NoError(t, err)

	assert.NoFileExists(t, src)
	content, _ := os.ReadFile(dst)
	assert.Equal(t, "data", string(content))
}

func TestFsyncDir(t *testing.T) {
	dir := t.TempDir()
	err := fsutil.FsyncDir(dir)
	assert.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/fsutil/... -v`
Expected: FAIL

**Step 3: Write implementation**

Create `pkg/fsutil/atomic.go`:
```go
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to a temporary file, fsyncs, then renames to target path.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".jvs-tmp-*")
	if err != nil {
		return fmt.Errorf("atomic write create tmp: %w", err)
	}
	tmpPath := tmp.Name()

	// Clean up on failure
	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("atomic write: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("atomic write chmod: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("atomic write fsync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomic write close: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomic write rename: %w", err)
	}
	if err := FsyncDir(dir); err != nil {
		return fmt.Errorf("atomic write fsync dir: %w", err)
	}

	success = true
	return nil
}

// RenameAndSync renames old to new and fsyncs the parent directory.
func RenameAndSync(oldpath, newpath string) error {
	if err := os.Rename(oldpath, newpath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return FsyncDir(filepath.Dir(newpath))
}

// FsyncDir fsyncs a directory to ensure rename visibility is durable.
func FsyncDir(dirPath string) error {
	d, err := os.Open(dirPath)
	if err != nil {
		return fmt.Errorf("fsync dir open: %w", err)
	}
	defer d.Close()
	return d.Sync()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/fsutil/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/fsutil/
git commit -m "feat: add atomic write and fsync utilities (pkg/fsutil)"
```

---

## Phase 2: Core Domain

### Task 2.1: Data Models (pkg/model/)

**Ref:** `docs/04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md` — Descriptor schema, Snapshot ID; `docs/03_WORKTREE_SPEC.md` — config.json schema

**Files:**
- Create: `pkg/model/snapshot.go` — SnapshotID, Descriptor, ReadyMarker, IntentRecord
- Create: `pkg/model/worktree.go` — WorktreeConfig
- ~~Create: `pkg/model/lock.go` — LockRecord, LockSession, LockPolicy~~ **REMOVED in v6.7** (lock mechanism deemed over-engineered for local-first workflows)
- Create: `pkg/model/audit.go` — AuditRecord
- Create: `pkg/model/ref.go` — RefRecord
- Create: `pkg/model/gc.go` — Pin, GCPlan, Tombstone, RetentionPolicy
- Create: `pkg/model/types.go` — EngineType, ConsistencyLevel, IntegrityState, HashValue, Isolation constants
- Create: `pkg/model/snapshot_test.go`

**Step 1: Write failing tests for SnapshotID**

Create `pkg/model/snapshot_test.go`:
```go
package model_test

import (
	"regexp"
	"testing"

	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var snapshotIDPattern = regexp.MustCompile(`^\d{13}-[0-9a-f]{8}$`)

func TestNewSnapshotID_Format(t *testing.T) {
	id := model.NewSnapshotID()
	require.Regexp(t, snapshotIDPattern, string(id))
}

func TestSnapshotID_ShortID(t *testing.T) {
	id := model.SnapshotID("1708300800000-a3f7c1b2")
	assert.Equal(t, "17083008", id.ShortID())
}

func TestNewSnapshotID_Uniqueness(t *testing.T) {
	seen := make(map[model.SnapshotID]bool)
	for i := 0; i < 100; i++ {
		id := model.NewSnapshotID()
		assert.False(t, seen[id], "duplicate: %s", id)
		seen[id] = true
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/model/... -v`
Expected: FAIL

**Step 3: Write all model files**

Implement all model types as defined in the design document (Section 2). Each type in its own file, all in `package model`. The complete code is specified in `docs/plans/2026-02-20-jvs-go-implementation-design.md` Section 2.

Key implementation notes:
- `NewSnapshotID()`: use `time.Now().UnixMilli()` zero-padded to 13 digits + `crypto/rand` 4 bytes hex
- `ShortID()`: return `string(id)[:8]`
- All time fields use `time.Time` with custom JSON marshal for ISO 8601
- Enum constants: `EngineJuiceFSClone`, `EngineReflinkCopy`, `EngineCopy`, etc.

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/model/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/model/
git commit -m "feat: add all domain model types (pkg/model)"
```

---

### Task 2.2: Repository Discovery and Init (internal/repo/)

**Ref:** `docs/01_REPO_LAYOUT_SPEC.md` — on-disk layout, format_version, worktree discovery

**Files:**
- Create: `internal/repo/repo.go` — Repo struct, Discover(), Init()
- Create: `internal/repo/repo_test.go`

**Step 1: Write failing tests**

Test Init creates correct directory structure, Discover finds repo from nested path, format_version is read and validated.

**Step 2: Implement**

Key functions:
- `Init(name string) (*Repo, error)` — creates `.jvs/` skeleton with all required subdirs, `format_version` (writes "1"), `repo_id`, `main/` payload dir, `.jvs/worktrees/main/config.json`
- `Discover(cwd string) (*Repo, error)` — walks up from CWD to find `.jvs/`, reads `format_version`
- `DiscoverWorktree(cwd string) (*Repo, string, error)` — Discover + maps CWD to worktree name

**Step 3: Run tests, commit**

```bash
git commit -m "feat: add repository init and discovery (internal/repo)"
```

---

### Task 2.3: Audit Appender (internal/audit/)

**Ref:** `docs/09_SECURITY_MODEL.md` — Audit log format, hash chain, rotation

**Files:**
- Create: `internal/audit/appender.go`
- Create: `internal/audit/appender_test.go`

**Step 1: Write failing tests**

Test: append creates JSONL line, hash chain links records, first record has empty prev_hash, flock serializes concurrent appends.

**Step 2: Implement**

`FileAppender` with `sync.Mutex` + `syscall.Flock`. Use `jsonutil.CanonicalMarshal` for record_hash computation. Reverse-scan for last record hash.

**Step 3: Run tests, commit**

```bash
git commit -m "feat: add audit log appender with hash chain (internal/audit)"
```

---

### Task 2.4: Integrity — Descriptor Checksum and Payload Hash (internal/integrity/)

**Ref:** `docs/04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md` — checksum coverage; `docs/05_SNAPSHOT_ENGINE_SPEC.md` — payload root hash algorithm

**Files:**
- Create: `internal/integrity/checksum.go`
- Create: `internal/integrity/payload_hash.go`
- Create: `internal/integrity/checksum_test.go`
- Create: `internal/integrity/payload_hash_test.go`

**Step 1: Write failing tests**

Test checksum: excludes `descriptor_checksum` and `integrity_state`, uses canonical JSON, SHA-256.
Test payload hash: deterministic for identical content, includes dirs, detects file content changes, permission changes, symlink changes. (Conformance test 19)

**Step 2: Implement**

- `ComputeDescriptorChecksum`: zero out excluded fields, canonical marshal, SHA-256
- `ComputePayloadRootHash`: recursive walk in byte-order sorted path order, `<type>:<path>:<metadata>:<hash>\n` format, SHA-256 of concatenation

**Step 3: Run tests, commit**

```bash
git commit -m "feat: add descriptor checksum and payload root hash (internal/integrity)"
```

---

## Phase 3: Engines and Operations

### Task 3.1: Engine Interface and Copy Engine (internal/engine/)

**Ref:** `docs/05_SNAPSHOT_ENGINE_SPEC.md` — Engine selection, metadata behavior

**Files:**
- Create: `internal/engine/engine.go` — Engine interface, types
- Create: `internal/engine/copy.go` — CopyEngine
- Create: `internal/engine/copy_test.go`

**Step 1: Write failing tests**

Test: deep copy preserves files, symlinks, directories, mode, timestamps. Reports hardlink degradation.

**Step 2: Implement CopyEngine**

Recursive walk with `os.Open` + `io.Copy`. Symlinks via `os.Readlink` + `os.Symlink`. Preserve mode/timestamps. Fsync all files and dirs. Report hardlink degradation.

**Step 3: Run tests, commit**

```bash
git commit -m "feat: add engine interface and copy engine (internal/engine)"
```

---

### Task 3.2: Reflink Engine

**Files:**
- Create: `internal/engine/reflink.go`
- Create: `internal/engine/reflink_test.go`
- Create: `internal/engine/detect.go`
- Create: `internal/engine/detect_test.go`

**Step 1-3: Implement reflink engine**

Uses `ioctl FICLONE` on Linux. Falls back to error if reflink not supported. Engine detection logic: env var → JuiceFS check → reflink probe → copy fallback.

**Step 4: Commit**

```bash
git commit -m "feat: add reflink engine and engine detection (internal/engine)"
```

---

### Task 3.3: JuiceFS Clone Engine

**Files:**
- Create: `internal/engine/juicefs.go`
- Create: `internal/engine/juicefs_test.go`

**Step 1-3: Implement**

Exec `juicefs clone <src> <dst> -p`. Parse exit code. Test with mock (exec test helper).

**Step 4: Commit**

```bash
git commit -m "feat: add JuiceFS clone engine (internal/engine)"
```

---

### Task 3.4: Lock Manager (internal/lock/) ~~REMOVED in v6.7~~

**Ref:** `docs/07_LOCKING_AND_CONSISTENCY_SPEC.md` — full protocol ~~(SPEC REMOVED)~~

**Files:**
- ~~Create: `internal/lock/manager.go`~~ **REMOVED**
- ~~Create: `internal/lock/manager_test.go`~~ **REMOVED**

**Step 1: Write failing tests**

Test acquire (O_CREAT|O_EXCL), conflict on double acquire, renew with nonce match, release, steal after expiry, fencing validation, clock skew detection, session file persistence.

Use short lease durations (100ms) for tests.

**Step 2: Implement**

Full Acquire/Renew/Steal/Release/ValidateFencing as described in design Section 5. Session file write/read for cross-process nonce persistence.

**Step 3: Run tests, commit**

```bash
git commit -m "feat: add SWMR lock manager with fencing (internal/lock)"
```
~~**REMOVED in v6.7** - This commit was reverted as part of lock mechanism removal.~~

---

### Task 3.5: Worktree Manager (internal/worktree/)

**Ref:** `docs/03_WORKTREE_SPEC.md` — lifecycle, config.json

**Files:**
- Create: `internal/worktree/manager.go`
- Create: `internal/worktree/manager_test.go`

**Step 1: Write failing tests**

Test: create writes config.json with correct schema, list reads all worktrees, rename with active lock fails (E_LOCK_CONFLICT), remove deletes payload + metadata, path returns canonical absolute path, UpdateHead atomically updates head_snapshot_id.

**Step 2: Implement**

CRUD operations on `.jvs/worktrees/<name>/config.json` and `repo/worktrees/<name>/`. Name validation via `pathutil.ValidateName`. Lock check on rename.

**Step 3: Run tests, commit**

```bash
git commit -m "feat: add worktree manager (internal/worktree)"
```

---

### Task 3.6: Snapshot Creator (internal/snapshot/)

**Ref:** `docs/05_SNAPSHOT_ENGINE_SPEC.md` — 12-step atomic publish protocol

**Files:**
- Create: `internal/snapshot/creator.go`
- Create: `internal/snapshot/creator_test.go`

**Step 1: Write failing tests**

Test: full snapshot creates .READY + descriptor + updates head, fencing re-validation at step 8.5, orphan tmp on failure is invisible, consistency level recorded in descriptor.

**Step 2: Implement the 12-step protocol**

Wire together: engine.Clone → integrity.ComputePayloadRootHash → integrity.ComputeDescriptorChecksum → fsutil.AtomicWrite → lock.ValidateFencing → fsutil.RenameAndSync → worktree.UpdateHead → audit.Append.

**Step 3: Run tests, commit**

```bash
git commit -m "feat: add snapshot creator with 12-step protocol (internal/snapshot)"
```

---

### Task 3.7: Restore (internal/restore/)

**Ref:** `docs/06_RESTORE_SPEC.md`

**Files:**
- Create: `internal/restore/restorer.go`
- Create: `internal/restore/restorer_test.go`

**Step 1: Write failing tests**

Test safe restore: creates new worktree from snapshot, auto-names, verifies integrity. Test inplace restore: requires lock+fencing, rejects without --force+--reason, rename-swap atomicity, rollback on failure.

**Step 2: Implement**

SafeRestore: verify → engine.Clone → write config.json → audit. InplaceRestore: verify → re-validate fencing → rename-swap → engine.Clone → cleanup → update head → audit.

**Step 3: Run tests, commit**

```bash
git commit -m "feat: add safe and inplace restore (internal/restore)"
```

---

## Phase 4: Management and Verification

### Task 4.1: Ref Manager (internal/ref/)

**Ref:** `docs/01_REPO_LAYOUT_SPEC.md` — refs/

**Files:**
- Create: `internal/ref/manager.go`
- Create: `internal/ref/manager_test.go`

**Step 1-3: Implement Create/List/Delete with name validation, immutability, audit events.**

**Step 4: Commit**

```bash
git commit -m "feat: add named reference manager (internal/ref)"
```

---

### Task 4.2: GC Collector (internal/gc/)

**Ref:** `docs/08_GC_SPEC.md` — plan/mark/commit protocol

**Files:**
- Create: `internal/gc/collector.go`
- Create: `internal/gc/collector_test.go`

**Step 1: Write failing tests**

Test: protection set computation (heads, lineage, pins, intents, refs), plan determinism, run revalidation (E_GC_PLAN_MISMATCH), retry from failed tombstones, idempotent committed skip.

**Step 2: Implement**

Plan: compute protected set → apply retention → write plan. Run: load plan → revalidate → mark tombstones → delete → commit tombstones → audit.

**Step 3: Commit**

```bash
git commit -m "feat: add two-phase GC collector (internal/gc)"
```

---

### Task 4.3: Verify Orchestrator (internal/verify/)

**Ref:** `docs/02_CLI_SPEC.md` — verify JSON fields

**Files:**
- Create: `internal/verify/verifier.go`
- Create: `internal/verify/verifier_test.go`

**Step 1-3: Implement single-snapshot and full-repo verification**

Checksum validation → payload hash validation → audit chain validation. Returns structured result with `checksum_valid`, `payload_hash_valid`, `tamper_detected`, `severity`.

**Step 4: Commit**

```bash
git commit -m "feat: add verification orchestrator (internal/verify)"
```

---

### Task 4.4: Doctor (internal/doctor/)

**Ref:** `docs/05_SNAPSHOT_ENGINE_SPEC.md` — crash recovery; `docs/02_CLI_SPEC.md` — doctor

**Files:**
- Create: `internal/doctor/doctor.go`
- Create: `internal/doctor/doctor_test.go`

**Step 1: Write failing tests**

Test: detects orphan tmp, detects head orphan (advance_head), detects expired locks, detects broken audit chain, --repair-runtime auto-fixes safe subset, format_version validation.

**Step 2: Implement**

7-category check as described in design Section 10. Finding + RepairAction structure. Auto-repair for clean_locks, clean_intents, rebuild_index.

**Step 3: Commit**

```bash
git commit -m "feat: add doctor diagnostics and repair (internal/doctor)"
```

---

## Phase 5: CLI Integration

### Task 5.1: CLI Root and Init Command

**Ref:** `docs/02_CLI_SPEC.md`

**Files:**
- Create: `internal/cli/root.go`
- Create: `internal/cli/context.go` — requireRepo, requireWorktree helpers
- Create: `internal/cli/init.go`
- Modify: `cmd/jvs/main.go` — wire to cli.Execute()

**Step 1: Install cobra**

Run: `go get github.com/spf13/cobra`

**Step 2: Implement root command with --json flag, init command**

**Step 3: Build and test**

Run: `make build && bin/jvs init myrepo && ls myrepo/.jvs/`
Expected: `.jvs/` with format_version, worktrees/main/config.json, all required subdirs

**Step 4: Commit**

```bash
git commit -m "feat: add CLI root and jvs init command"
```

---

### Task 5.2: Snapshot and History Commands

**Files:**
- Create: `internal/cli/snapshot.go`
- Create: `internal/cli/history.go`

**Step 1-3: Implement**

`jvs snapshot [note] [--consistency] [--json]` — calls snapshot.Creator.
`jvs history [--limit N] [--json]` — traverses lineage chain.

**Step 4: Commit**

```bash
git commit -m "feat: add jvs snapshot and history commands"
```

---

### Task 5.3: Lock Commands ~~REMOVED in v6.7~~

**Files:**
- ~~Create: `internal/cli/lock.go`~~ **REMOVED**

**Step 1-3: Implement acquire/status/renew/release subcommands**

Wire to lock.Manager. Output session info on acquire.

**Step 4: Commit**

```bash
git commit -m "feat: add jvs lock commands"
```

---

### Task 5.4: Worktree Commands

**Files:**
- Create: `internal/cli/worktree.go`

**Step 1-3: Implement create/list/path/rename/remove subcommands**

**Step 4: Commit**

```bash
git commit -m "feat: add jvs worktree commands"
```

---

### Task 5.5: Restore Command

**Files:**
- Create: `internal/cli/restore.go`

**Step 1-3: Implement safe and inplace restore modes**

**Step 4: Commit**

```bash
git commit -m "feat: add jvs restore command"
```

---

### Task 5.6: Verify, Doctor, Info Commands

**Files:**
- Create: `internal/cli/verify.go`
- Create: `internal/cli/doctor.go`
- Create: `internal/cli/info.go`

**Step 1-3: Implement**

Info aggregates: format_version, engine, isolation default, lock policy, totals.
Verify and doctor wire to their respective packages.

**Step 4: Commit**

```bash
git commit -m "feat: add jvs verify, doctor, and info commands"
```

---

### Task 5.7: GC and Ref Commands

**Files:**
- Create: `internal/cli/gc.go`
- Create: `internal/cli/ref.go`

**Step 1-3: Implement**

GC plan/run subcommands. Ref create/list/delete subcommands.

**Step 4: Commit**

```bash
git commit -m "feat: add jvs gc and ref commands"
```

---

### Task 5.8: End-to-End Smoke Test

**Step 1: Run a full workflow manually**

```bash
make build
cd /tmp && bin/jvs init smoketest && cd smoketest/main
echo "hello" > test.txt
bin/jvs lock acquire
bin/jvs snapshot "first snapshot"
bin/jvs history --json
bin/jvs verify --all
bin/jvs doctor --strict
bin/jvs lock release
bin/jvs restore <snapshot-id>
bin/jvs worktree list --json
bin/jvs ref create v1 <snapshot-id>
bin/jvs ref list --json
```

**Step 2: Fix any issues found**

**Step 3: Commit fixes**

```bash
git commit -m "fix: resolve issues found in smoke test"
```

---

## Phase 6: Conformance Tests

### Task 6.1: Conformance Test Infrastructure

**Files:**
- Create: `test/conformance/helpers_test.go` — shared setup, build binary, run jvs
- Create: `test/conformance/doc_test.go` — package doc with build tag

**Step 1: Implement test helpers**

```go
//go:build conformance

package conformance

// initTestRepo creates a temp repo, returns repo path and cleanup func
// runJVS executes the jvs binary with args, returns stdout, stderr, exit code
// shortLeasePolicy returns LockPolicy with 100ms lease for fast tests
```

**Step 2: Commit**

```bash
git commit -m "feat: add conformance test infrastructure"
```

---

### Task 6.2: Conformance Tests 1-9 (Lock + Snapshot + Restore + Path + Integrity)

**Files:**
- ~~Create: `test/conformance/lock_test.go` — tests 1, 2, 3~~ **REMOVED in v6.7** (lock conformance tests removed with lock mechanism)
- Create: `test/conformance/snapshot_test.go` — tests 4, 5
- Create: `test/conformance/restore_test.go` — test 6
- Create: `test/conformance/path_test.go` — test 7
- Create: `test/conformance/integrity_test.go` — tests 8, 9

**Step 1: Implement all 9 tests**

Each test follows pattern: setup repo → perform operation → assert expected behavior/error class.

**Step 2: Run and verify**

Run: `make conformance`
Expected: 9 PASS

**Step 3: Commit**

```bash
git commit -m "test: add conformance tests 1-9"
```

---

### Task 6.3: Conformance Tests 10-22 (Doctor + Migration + GC + Audit + Format + Lock + Snapshot + Ref + Doctor)

**Files:**
- Create: `test/conformance/doctor_test.go` — tests 10, 21
- Create: `test/conformance/migration_test.go` — test 11
- Create: `test/conformance/gc_test.go` — tests 12, 13, 22
- Create: `test/conformance/audit_test.go` — tests 14, 15
- Create: `test/conformance/format_test.go` — test 16
- ~~Extend: `test/conformance/lock_test.go` — tests 17, 23 (added to existing file)~~ **REMOVED in v6.7**
- Extend: `test/conformance/snapshot_test.go` — tests 18, 19, 24 (added to existing file)
- Create: `test/conformance/ref_test.go` — test 20

**Step 1: Implement all 15 tests**

**Step 2: Run and verify**

Run: `make conformance`
Expected: 24 PASS

**Step 3: Commit**

```bash
git commit -m "test: add conformance tests 10-24"
```

---

### Task 6.4: Conformance Tests 25-29 (Worktree + Init)

**Files:**
- Extend: `test/conformance/doctor_test.go` — test 25
- Create: `test/conformance/worktree_test.go` — tests 26, 28
- Create: `test/conformance/init_test.go` — test 27
- Create: `test/conformance/migration_test.go` — test 29 (extend)

**Step 1: Implement remaining 5 tests**

**Step 2: Run full suite**

Run: `make conformance`
Expected: 29 PASS — all conformance tests green

**Step 3: Commit**

```bash
git commit -m "test: add conformance tests 25-29, full suite passing"
```

---

### Task 6.5: Final Verification

**Step 1: Run all tests**

```bash
make verify
```

Expected: unit tests PASS, conformance tests 29/29 PASS, lint clean

**Step 2: Run release gates (per docs/12_RELEASE_POLICY.md)**

```bash
bin/jvs doctor --strict
bin/jvs verify --all
```

**Step 3: Final commit**

```bash
git commit -m "chore: all tests passing, v0.x implementation complete"
```

---

## Task Dependency Graph

```
Phase 1: 1.1 → 1.2 → 1.3 → 1.4 → 1.5 → 1.6
Phase 2: 2.1 → 2.2 → 2.3 → 2.4
Phase 3: 3.1 → 3.2 → 3.3 (engines parallel-safe)
          3.4 (lock, depends on 2.3 audit)
          3.5 (worktree, depends on 3.4 lock)
          3.6 (snapshot, depends on 3.1+3.4+3.5+2.4)
          3.7 (restore, depends on 3.6)
Phase 4: 4.1 (ref, depends on 2.3)
          4.2 (gc, depends on 4.1+3.5)
          4.3 (verify, depends on 2.4)
          4.4 (doctor, depends on 4.3+3.4)
Phase 5: 5.1 → 5.2-5.7 (parallel-safe) → 5.8
Phase 6: 6.1 → 6.2 → 6.3 → 6.4 → 6.5
```

Total: ~35 tasks, ~35 commits
