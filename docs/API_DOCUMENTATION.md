# JVS Go Library API Documentation

**Version:** v7.0
**Last Updated:** 2026-02-23

---

## Overview

JVS can be used as a Go library for programmatic workspace versioning. The public API is organized under `pkg/` packages:

| Package | Purpose |
|---------|---------|
| `pkg/model` | Core data models for checkpoints, workspaces, and GC |
| `pkg/config` | Configuration file handling |
| `pkg/errclass` | Stable error classes |
| `pkg/uuidutil` | UUID generation utilities |
| `pkg/pathutil` | Path validation utilities |
| `pkg/fsutil` | Atomic file operations |
| `pkg/jsonutil` | Canonical JSON serialization |
| `pkg/logging` | Structured logging |
| `pkg/progress` | Progress reporting |

---

## Quick Example

```go
package main

import (
    "fmt"
    "github.com/jvs-project/jvs/pkg/model"
)

func main() {
    // Generate a new checkpoint ID
    id := model.NewSnapshotID()
    fmt.Printf("Checkpoint ID: %s\n", id.String())
    fmt.Printf("Short ID: %s\n", id.ShortID())
}
```

---

## Internal Compatibility: pkg/model

### SnapshotID

Unique identifier for snapshots. Format: `<unix_ms>-<rand8hex>`

```go
type SnapshotID string
```

**Methods:**

| Method | Returns | Description |
|--------|---------|-------------|
| `NewSnapshotID()` | `SnapshotID` | Generate a new unique snapshot ID |
| `ShortID()` | `string` | First 8 characters (for display) |
| `String()` | `string` | Full snapshot ID |

**Example:**
```go
id := model.NewSnapshotID()
fmt.Println(id)          // "1708694400000-a3b2c1d4"
fmt.Println(id.ShortID()) // "17086944"
```

---

### Descriptor

On-disk snapshot metadata.

```go
type Descriptor struct {
    SnapshotID         SnapshotID     `json:"snapshot_id"`
    ParentID           *SnapshotID    `json:"parent_id,omitempty"`
    WorktreeName       string         `json:"worktree_name"`
    CreatedAt          time.Time      `json:"created_at"`
    Note               string         `json:"note,omitempty"`
    Tags               []string       `json:"tags,omitempty"`
    Engine             EngineType     `json:"engine"`
    PayloadRootHash    HashValue      `json:"payload_root_hash"`
    DescriptorChecksum HashValue      `json:"descriptor_checksum"`
    IntegrityState     IntegrityState `json:"integrity_state"`
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `SnapshotID` | `SnapshotID` | Unique identifier |
| `ParentID` | `*SnapshotID` | Parent snapshot (nil for root) |
| `WorktreeName` | `string` | Worktree that created this snapshot |
| `CreatedAt` | `time.Time` | Timestamp when snapshot was created |
| `Note` | `string` | User-provided description |
| `Tags` | `[]string` | Organization tags |
| `Engine` | `EngineType` | Snapshot engine used |
| `PayloadRootHash` | `HashValue` | SHA-256 of payload tree |
| `DescriptorChecksum` | `HashValue` | SHA-256 of descriptor JSON |
| `IntegrityState` | `IntegrityState` | Verification status |

---

### ReadyMarker

The `.READY` file content indicating complete snapshot.

```go
type ReadyMarker struct {
    SnapshotID         SnapshotID `json:"snapshot_id"`
    CompletedAt        time.Time  `json:"completed_at"`
    PayloadHash        HashValue  `json:"payload_root_hash"`
    Engine             EngineType `json:"engine"`
    DescriptorChecksum HashValue  `json:"descriptor_checksum"`
}
```

---

### IntentRecord

Tracks in-progress snapshot creation for crash recovery.

```go
type IntentRecord struct {
    SnapshotID   SnapshotID `json:"snapshot_id"`
    WorktreeName string     `json:"worktree_name"`
    StartedAt    time.Time  `json:"started_at"`
    Engine       EngineType `json:"engine"`
}
```

---

### HashValue

SHA-256 hash value as hex-encoded string.

```go
type HashValue string
```

---

### EngineType

Snapshot engine type.

```go
type EngineType string

const (
    EngineJuiceFSClone EngineType = "juicefs-clone" // O(1) on JuiceFS
    EngineReflink      EngineType = "reflink"        // O(1) on CoW filesystems
    EngineCopy         EngineType = "copy"           // O(n) fallback
)
```

---

### IntegrityState

Snapshot verification status.

```go
type IntegrityState string

const (
    IntegrityStateUnknown   IntegrityState = "unknown"
    IntegrityStateVerified  IntegrityState = "verified"
    IntegrityStateCorrupted IntegrityState = "corrupted"
    IntegrityStatePartial   IntegrityState = "partial"
)
```

---

## Internal Compatibility: pkg/model Workspace Storage

### WorktreeConfig

Worktree metadata stored in `.jvs/worktrees/<name>/config.json`.

```go
type WorktreeConfig struct {
    Name       string     `json:"name"`
    RootPath   string     `json:"root_path"`
    SnapshotID SnapshotID `json:"snapshot_id"`
    CreatedAt  time.Time  `json:"created_at"`
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` | Worktree name (unique) |
| `RootPath` | `string` | Absolute path to worktree payload |
| `SnapshotID` | `SnapshotID` | Current snapshot |
| `CreatedAt` | `time.Time` | When worktree was created |

---

## Internal Compatibility: pkg/model GC Storage

### GCPolicy

Garbage collection retention policy.

```go
type GCPolicy struct {
    KeepMinSnapshots int           `json:"keep_min_snapshots"`
    KeepMinAge       time.Duration `json:"keep_min_age"`
    KeepTagged       bool           `json:"keep_tagged"`
    KeepPinned       []SnapshotID   `json:"keep_pinned"`
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `KeepMinSnapshots` | `int` | Minimum snapshots to retain regardless of age |
| `KeepMinAge` | `time.Duration` | Minimum age before GC consideration |
| `KeepTagged` | `bool` | Protect tagged snapshots |
| `KeepPinned` | `[]SnapshotID` | Explicitly protected snapshots |

---

### GCPlan

Garbage collection plan with tombstones.

```go
type GCPlan struct {
    PlanID     string       `json:"plan_id"`
    CreatedAt  time.Time    `json:"created_at"`
    Policy     GCPolicy     `json:"policy"`
    Tombstones []Tombstone   `json:"tombstones"`
    TotalBytes int64        `json:"total_bytes"`
}

type Tombstone struct {
    SnapshotID SnapshotID `json:"snapshot_id"`
    Reason     string     `json:"reason"`
}
```

---

## Internal Compatibility: pkg/model Audit Storage

### AuditRecord

Tamper-evident audit log entry.

```go
type AuditRecord struct {
    EventID    string    `json:"event_id"`    // UUID v4
    Timestamp  time.Time `json:"timestamp"`   // ISO 8601
    Operation  string    `json:"operation"`   // snapshot, restore, gc_run, etc.
    Actor      string    `json:"actor"`       // user@host
    Target     string    `json:"target"`      // affected snapshot/worktree
    Reason     string    `json:"reason"`      // for dangerous operations
    PrevHash   HashValue `json:"prev_hash"`   // previous record hash
    RecordHash HashValue `json:"record_hash"` // this record hash
}
```

---

## pkg/config

### Config

JVS configuration from `.jvs/config.yaml`.

```go
type Config struct {
    Engine          string                `yaml:"engine"`
    RetentionPolicy RetentionPolicyConfig `yaml:"retention_policy"`
    Logging         LoggingConfig         `yaml:"logging"`
}
```

**Functions:**

| Function | Returns | Description |
|----------|---------|-------------|
| `Default()` | `*Config` | Default configuration values |
| `Load(repoRoot string)` | `(*Config, error)` | Load from `.jvs/config.yaml` (returns default if missing) |
| `Save(path string)` | `error` | Write configuration to file |

**Example:**
```go
cfg, err := config.Load("/path/to/repo")
if err != nil {
    return err
}
fmt.Printf("Engine: %s\n", cfg.Engine)
```

---

## pkg/errclass

### JVSError

Stable, machine-readable error class for user-facing errors.

```go
type JVSError struct {
    Code    string
    Message string
}
```

**Methods:**

| Method | Returns | Description |
|--------|---------|-------------|
| `Error()` | `string` | Formatted error string |
| `Is(target error)` | `bool` | Error code comparison |
| `WithMessage(msg string)` | `*JVSError` | New error with same code, custom message |
| `WithMessagef(format string, args ...any)` | `*JVSError` | New error with formatted message |

**Predefined Error Classes:**

| Error Code | Usage |
|------------|-------|
| `E_NAME_INVALID` | Invalid worktree/snapshot name |
| `E_PATH_ESCAPE` | Path traversal attempt |
| `E_DESCRIPTOR_CORRUPT` | Descriptor checksum failed |
| `E_PAYLOAD_HASH_MISMATCH` | Payload hash verification failed |
| `E_LINEAGE_BROKEN` | Snapshot lineage inconsistency |
| `E_PARTIAL_SNAPSHOT` | Incomplete snapshot detected |
| `E_GC_PLAN_MISMATCH` | GC plan ID mismatch |
| `E_FORMAT_UNSUPPORTED` | Format version not supported |
| `E_AUDIT_CHAIN_BROKEN` | Audit hash chain validation failed |

**Example:**
```go
import "github.com/jvs-project/jvs/pkg/errclass"

// Return a name validation error
return errclass.ErrNameInvalid.WithMessage("worktree name cannot be empty")

// Check for specific error type
if errors.Is(err, errclass.ErrDescriptorCorrupt) {
    // Handle descriptor corruption
}
```

---

## pkg/uuidutil

### UUID Generation

Cryptographically secure UUID v4 generation.

```go
func NewV4() string
```

**Example:**
```go
import "github.com/jvs-project/jvs/pkg/uuidutil"

eventID := uuidutil.NewV4()
```

---

## pkg/pathutil

### Path Validation

Validate worktree and snapshot names for security.

```go
func ValidateName(name string) error
```

**Rules:**
- Must match `[a-zA-Z0-9._-]+`
- Cannot be empty
- Cannot start with `.` or `-`
- Cannot contain `..` (path escape)

**Example:**
```go
import "github.com/jvs-project/jvs/pkg/pathutil"

err := pathutil.ValidateName("my-worktree")
if err != nil {
    return err
}
```

---

## pkg/fsutil

### Atomic Operations

Atomic file write with fsync for durability.

```go
func AtomicWrite(path string, data []byte) error
```

Writes data atomically:
1. Write to temporary file (`.tmp` suffix)
2. fsync temporary file
3. Rename to final path (atomic)
4. fsync parent directory

**Example:**
```go
import "github.com/jvs-project/jvs/pkg/fsutil"

data := []byte(`{"key": "value"}`)
err := fsutil.AtomicWrite("/path/to/file.json", data)
```

---

## pkg/jsonutil

### Canonical JSON

Canonical JSON serialization for hashing.

```go
func CanonicalMarshal(v any) ([]byte, error)
```

**Canonical form rules:**
- Keys sorted lexicographically
- No whitespace
- UTF-8 encoding
- No trailing newline

**Example:**
```go
import "github.com/jvs-project/jvs/pkg/jsonutil"

data := map[string]any{"b": 2, "a": 1}
jsonBytes, err := jsonutil.CanonicalMarshal(data)
// Result: {"a":1,"b":2}
```

---

## pkg/logging

### Structured Logging

Simple leveled logging interface.

```go
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
}

func New(level string, format string) Logger
```

**Formats:**
- `text` - Human-readable plain text
- `json` - Machine-readable JSON Lines

**Example:**
```go
import "github.com/jvs-project/jvs/pkg/logging"

logger := logging.New("info", "text")
logger.Info("Repository initialized", "path", "/path/to/repo")
// Output: INFO Repository initialized path=/path/to/repo
```

---

## pkg/progress

### Progress Reporting

Progress tracking for long-running operations.

```go
type Reporter interface {
    Start(msg string, total int64)
    Increment(n int64)
    SetCurrent(current int64)
    Complete(msg string)
}

func NewQuiet() Reporter
func NewBar() Reporter
```

**Example:**
```go
import "github.com/jvs-project/jvs/pkg/progress"

reporter := progress.NewBar()
reporter.Start("Computing payload hash", 1000)
// ... during operation
reporter.Increment(100)
reporter.Complete("Hash computed")
```

---

## Integration Example

Creating a checkpoint programmatically:

```go
package main

import (
    "github.com/jvs-project/jvs/pkg/model"
    "github.com/jvs-project/jvs/pkg/errclass"
    "github.com/jvs-project/jvs/pkg/fsutil"
    "github.com/jvs-project/jvs/pkg/jsonutil"
)

func CreateCheckpoint(workspacePath, note string, tags []string) (*model.Descriptor, error) {
    // 1. Generate checkpoint ID
    id := model.NewSnapshotID()

    // 2. Create intent record (for crash recovery)
    intent := &model.IntentRecord{
        SnapshotID:   id,
        WorktreeName: "main",
        StartedAt:    time.Now(),
        Engine:       model.EngineJuiceFSClone,
    }

    // 3. Build descriptor
    desc := &model.Descriptor{
        SnapshotID:   id,
        WorktreeName: "main",
        CreatedAt:    time.Now(),
        Note:         note,
        Tags:         tags,
        Engine:       model.EngineJuiceFSClone,
    }

    // 4. Serialize descriptor
    data, err := jsonutil.CanonicalMarshal(desc)
    if err != nil {
        return nil, err
    }

    // 5. Write descriptor atomically
    descPath := fmt.Sprintf(".jvs/descriptors/%s.json", id)
    if err := fsutil.AtomicWrite(descPath, data); err != nil {
        return nil, errclass.ErrDescriptorCorrupt.WithMessage(err.Error())
    }

    return desc, nil
}
```

---

## Best Practices

1. **Error Handling**: Always use `errclass` errors for user-facing issues
2. **Atomic Operations**: Use `fsutil.AtomicWrite` for all metadata
3. **Canonical JSON**: Use `jsonutil.CanonicalMarshal` for hashing
4. **Path Validation**: Always validate names with `pathutil.ValidateName`
5. **Logging**: Use structured logging for operational visibility

---

## Stability Guarantees

- **Stable**: Public APIs in `pkg/` will follow Semantic Versioning
- **Experimental**: `internal/` packages are not for external use
- **Error Classes**: Error codes are stable (v7.x)

---

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - System design and components
- [02_CLI_SPEC.md](02_CLI_SPEC.md) - CLI command reference
- [CONTRIBUTING.md](../CONTRIBUTING.md) - Contributing guidelines

---

*For API changes between versions, see [CHANGELOG.md](99_CHANGELOG.md)*
