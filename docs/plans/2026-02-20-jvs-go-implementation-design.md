# JVS Go Implementation Design

**Status:** Archived, non-release-facing

> Archived note: this historical implementation design is not part of the v0 public contract.
> It is retained only as non-current design context.

**Date:** 2026-02-20
**Spec version:** v6.5
**Current version:** v7.0
**Language:** Go 1.26
**Module:** `github.com/jvs-project/jvs`

> **Note:** This design document was written for v6.5 and contains references to the lock mechanism that was **removed in v6.7**. The lock-related sections (fencing tokens, holder nonces, session files, lock manager) are no longer applicable to the current implementation. This document is kept for historical reference.

---

## 1. Architecture Overview

JVS is a layered single-binary CLI. Internal packages are organized by domain with strict unidirectional dependencies.

```
github.com/jvs-project/jvs/
├── cmd/jvs/              # CLI entry point
├── internal/
│   ├── repo/             # Repository discovery, format_version, initialization
│   ├── worktree/         # Worktree CRUD, config.json, isolation
│   ├── snapshot/         # Snapshot lifecycle: creation, READY protocol, descriptors
│   ├── engine/           # Engine interface + 3 implementations
│   ├── restore/          # Safe restore + inplace restore
│   ├── lock/             # SWMR lock/lease/fencing/steal protocol
│   ├── integrity/        # Checksum, payload root hash computation
│   ├── gc/               # Retention policy, plan/mark/commit GC
│   ├── audit/            # JSONL audit log, hash chain, rotation
│   ├── ref/              # Named reference CRUD
│   ├── doctor/           # Layout validation, lineage checks, repairs
│   ├── verify/           # Strong verification orchestration
│   └── cli/              # Cobra command definitions
├── pkg/
│   ├── model/            # Shared types: SnapshotID, Descriptor, LockRecord, etc.
│   ├── jsonutil/         # Canonical JSON serialization
│   ├── pathutil/         # Path safety, NFC normalization, symlink escape detection
│   ├── fsutil/           # Fsync helpers, atomic write, reflink probe
│   ├── errclass/         # Stable error classes with exit code mapping
│   └── uuidutil/         # Minimal UUID v4 (stdlib crypto/rand)
└── test/
    └── conformance/      # 29 conformance tests (spec 11)
```

**Dependency direction:** `cli/ → {snapshot, restore, lock, gc, ...} → {repo, engine, integrity, audit} → {model, jsonutil, pathutil, fsutil, errclass}`. No reverse imports or sibling cycles.

**External dependencies (4 total):**
- `github.com/spf13/cobra` — CLI framework
- `github.com/stretchr/testify` — test assertions (test only)
- `golang.org/x/text` — NFC normalization
- Standard library `crypto/sha256`, `crypto/ed25519` (reserved for v1.x signing)

---

## 2. Core Data Models (pkg/model/)

All domain types in `pkg/model/`, serialization strictly matches spec.

```go
// SnapshotID format: <timestamp_ms_13digits>-<random_hex8>
type SnapshotID string

func NewSnapshotID() SnapshotID
func (id SnapshotID) ShortID() string // first 8 chars

// Descriptor — .jvs/descriptors/<id>.json (spec 04 v6.5)
type Descriptor struct {
    SnapshotID       SnapshotID       `json:"snapshot_id"`
    WorktreeID       string           `json:"worktree_id"`
    ParentSnapshotID *SnapshotID      `json:"parent_snapshot_id"`
    CreatedAt        time.Time        `json:"created_at"`
    Note             string           `json:"note,omitempty"`
    Engine           EngineType       `json:"engine"`
    ConsistencyLevel ConsistencyLevel `json:"consistency_level"`
    FencingToken     *uint64          `json:"fencing_token"`
    DescriptorChecksum HashValue      `json:"descriptor_checksum"`
    PayloadRootHash    HashValue      `json:"payload_root_hash"`
    IntegrityState   IntegrityState   `json:"integrity_state"`
}

type HashValue struct {
    Algo  string `json:"algo"`  // "sha256"
    Value string `json:"value"` // hex encoded
}

// WorktreeConfig — .jvs/worktrees/<name>/config.json
type WorktreeConfig struct {
    WorktreeID     string      `json:"worktree_id"`
    Isolation      string      `json:"isolation"`        // "exclusive" (only value in v0.x)
    CreatedAt      time.Time   `json:"created_at"`
    BaseSnapshotID *SnapshotID `json:"base_snapshot_id"`
    HeadSnapshotID *SnapshotID `json:"head_snapshot_id"`
    Label          string      `json:"label,omitempty"`
}

// LockRecord — .jvs/locks/<worktree_id>.lock
type LockRecord struct {
    LockID          string    `json:"lock_id"`
    WorktreeID      string    `json:"worktree_id"`
    HolderID        string    `json:"holder_id"`        // host:user:pid:start_time
    HolderNonce     string    `json:"holder_nonce"`
    SessionID       string    `json:"session_id"`
    AcquireSeq      uint64    `json:"acquire_seq"`
    CreatedAt       time.Time `json:"created_at"`
    LastRenewedAt   time.Time `json:"last_renewed_at"`
    LeaseDurationMs int64     `json:"lease_duration_ms"` // default 30000
    RenewIntervalMs int64     `json:"renew_interval_ms"` // default 10000
    MaxClockSkewMs  int64     `json:"max_clock_skew_ms"` // default 2000
    StealGraceMs    int64     `json:"steal_grace_ms"`    // default 1000
    LeaseExpiresAt  time.Time `json:"lease_expires_at"`
    FencingToken    uint64    `json:"fencing_token"`
}

// LockSession — .jvs/locks/<worktree_id>.session (runtime state)
type LockSession struct {
    HolderNonce string `json:"holder_nonce"`
    SessionID   string `json:"session_id"`
    AcquiredAt  string `json:"acquired_at"`
}

// AuditRecord — .jvs/audit/audit.jsonl per line
type AuditRecord struct {
    EventID      string  `json:"event_id"`
    Timestamp    string  `json:"timestamp"`
    Operation    string  `json:"operation"`
    Actor        string  `json:"actor"`
    Target       string  `json:"target"`
    FencingToken *uint64 `json:"fencing_token"`
    SessionID    string  `json:"session_id"`
    Reason       *string `json:"reason"`
    PrevHash     string  `json:"prev_hash"`
    RecordHash   string  `json:"record_hash"`
}

// IntentRecord — .jvs/intents/snapshot-<id>.json
type IntentRecord struct {
    IntentID    string     `json:"intent_id"`
    Operation   string     `json:"operation"`
    SnapshotID  SnapshotID `json:"snapshot_id"`
    WorktreeID  string     `json:"worktree_id"`
    CreatedAt   time.Time  `json:"created_at"`
    State       string     `json:"state"` // "active"|"completed"|"abandoned"
    CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ReadyMarker — .jvs/snapshots/<id>/.READY
type ReadyMarker struct {
    SnapshotID        string `json:"snapshot_id"`
    CreatedAt         string `json:"created_at"`
    Engine            string `json:"engine"`
    DescriptorChecksum string `json:"descriptor_checksum"`
    PayloadRootHash   string `json:"payload_root_hash"`
}

// RefRecord — .jvs/refs/<name>.json
type RefRecord struct {
    RefName    string `json:"ref_name"`
    SnapshotID string `json:"snapshot_id"`
    CreatedAt  string `json:"created_at"`
    CreatedBy  string `json:"created_by"`
    Note       string `json:"note,omitempty"`
}

// GC types
type Pin struct {
    PinID      string     `json:"pin_id"`
    SnapshotID SnapshotID `json:"snapshot_id"`
    Reason     string     `json:"reason"`
    CreatedAt  time.Time  `json:"created_at"`
    ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type GCPlan struct {
    PlanID               string       `json:"plan_id"`
    Candidates           []SnapshotID `json:"candidates"`
    CandidateCount       int          `json:"candidate_count"`
    ProtectedByPin       int          `json:"protected_by_pin"`
    ProtectedByLineage   int          `json:"protected_by_lineage"`
    DeletableBytesEstimate int64      `json:"deletable_bytes_estimate"`
    CreatedAt            time.Time    `json:"created_at"`
}

type Tombstone struct {
    SnapshotID    SnapshotID `json:"snapshot_id"`
    PlanID        string     `json:"plan_id"`
    GCState       string     `json:"gc_state"` // "marked"|"committed"|"failed"
    MarkedAt      time.Time  `json:"marked_at"`
    CommittedAt   *time.Time `json:"committed_at,omitempty"`
    FailureReason *string    `json:"failure_reason,omitempty"`
}

// Retention policy — .jvs/gc/retention.json
type RetentionPolicy struct {
    KeepLastN       *int     `json:"keep_last_n,omitempty"`
    KeepDays        *int     `json:"keep_days,omitempty"`
    KeepTagPrefixes []string `json:"keep_tag_prefixes,omitempty"`
    MaxRepoBytes    *int64   `json:"max_repo_bytes,omitempty"`
}
```

**Design notes:**
- All `time.Time` serialized as ISO 8601 with timezone via custom format function
- `SnapshotID` is a value type, not bare string
- Enum types (`EngineType`, `ConsistencyLevel`, `IntegrityState`) use string constants + validation
- `fencing_token` is non-nullable in v0.x (exclusive-only), but schema keeps pointer for forward compat

---

## 3. Engine Abstraction (internal/engine/)

```go
type Engine interface {
    Name() string
    Clone(ctx context.Context, src, dst string) (CloneResult, error)
    MetadataPreservation() MetadataCapabilities
}

type CloneResult struct {
    Degradations []Degradation // empty = full preservation
}

type Degradation struct {
    Path   string
    Kind   string // "hardlink", "xattr", "acl"
    Detail string
}

type MetadataCapabilities struct {
    Symlinks   Preservation
    Hardlinks  Preservation
    ModeOwner  Preservation
    Timestamps Preservation
    Xattrs     Preservation
    ACLs       Preservation
}
```

**Three implementations:**

| Engine | File | Mechanism | fsync |
|--------|------|-----------|-------|
| `juicefs-clone` | juicefs.go | `juicefs clone <src> <dst> [-p]` subprocess | Metadata engine guarantees durability |
| `reflink-copy` | reflink.go | Recursive walk + `ioctl FICLONE` per file | Explicit fsync all files + dirs |
| `copy` | copy.go | `os.Open` + `io.Copy` deep copy | Explicit fsync all files + dirs |

**Engine selection** (detect.go):
1. `JVS_SNAPSHOT_ENGINE` env var → forced override
2. JuiceFS mount + `juicefs` CLI available → `juicefs-clone`
3. Reflink probe succeeds (actual test file, not heuristic) → `reflink-copy`
4. Fallback → `copy`

**Key constraint:** Silent metadata downgrade is forbidden. Non-empty `Degradations` in `CloneResult` is reported to the caller; caller decides whether to abort or record degraded state.

**fsync is the engine's responsibility.** When `Clone` returns, data is durable.

---

## 4. Snapshot Lifecycle (internal/snapshot/)

12-step atomic publish protocol (spec 05 v6.5):

```go
type Creator struct {
    repo    *repo.Repo
    engine  engine.Engine
    hasher  integrity.Hasher
    auditor audit.Appender
    locker  lock.Manager
}

func (c *Creator) Create(ctx context.Context, opts CreateOpts) (*model.Descriptor, error)
```

| Step | Action | Implementation |
|------|--------|----------------|
| 1 | Verify preconditions | Check worktree exists, validate lock + fencing token |
| 2 | Write intent | `fsutil.AtomicWrite(.jvs/intents/snapshot-<id>.json)` |
| 3 | Materialize payload | `engine.Clone(worktree_path, .jvs/snapshots/<id>.tmp/)` |
| 4 | Compute payload hash | `integrity.ComputePayloadRootHash()` over tmp dir |
| 5 | Fsync snapshot tree | Engine's responsibility (done in step 3) |
| 6 | Build descriptor tmp | Populate fields, compute `descriptor_checksum` |
| 7 | Fsync descriptor tmp | `fsutil.AtomicWrite(.jvs/descriptors/<id>.json.tmp)` |
| 8 | Write .READY | Write marker in snapshot tmp with checksum; fsync |
| 8.5 | **Re-validate fencing** | `locker.ValidateFencing()` — critical before visibility |
| 9 | Rename snapshot | `fsutil.RenameAndSync(.tmp/ → final/)` — atomic visibility boundary |
| 10 | Rename descriptor | `fsutil.RenameAndSync(.json.tmp → .json)` |
| 11 | Update head | `worktree.UpdateHead()` via `fsutil.AtomicWrite` |
| 12 | Complete intent + audit | Mark intent completed, `auditor.Append()` |

**Failure semantics:** Any step failure returns error immediately. No rollback — orphan tmp and uncompleted intents are invisible (no .READY) and cleaned by `doctor --strict`.

**Consistency level:** In v0.x (exclusive-only), lock holder guarantees no concurrent payload writers, so `quiesced` is naturally satisfied. `best_effort` is available as an explicit opt-in via `--consistency best_effort`; the descriptor carries the risk label.

---

## 5. Lock Mechanism (internal/lock/)

```go
type Manager struct {
    repoRoot string
    policy   LockPolicy
    auditor  audit.Appender
}

type LockPolicy struct {
    LeaseDurationMs int64 // 30000
    RenewIntervalMs int64 // 10000
    MaxClockSkewMs  int64 // 2000
    StealGraceMs    int64 // 1000
}
```

### Acquire

- `os.OpenFile(path, O_CREAT|O_EXCL|O_WRONLY, 0644)` on `.jvs/locks/<worktree_id>.lock`
- `EEXIST` → read existing lock, evaluate expiry:
  - Active non-expired → `E_LOCK_CONFLICT`
  - Expired but within skew+grace window → `E_LOCK_CONFLICT`
  - Past `lease_expires_at + max_clock_skew_ms + steal_grace_ms` → steal flow
- On success: fsync file + parent dir
- Write session file `.jvs/locks/<worktree_id>.session` with `holder_nonce` + `session_id`
- `holder_id` = `hostname:username:pid:process_start_time`
- `fencing_token` initial value = 1

### Renew

- Pre-check: if local lease already expired → `E_LOCK_EXPIRED` (don't attempt)
- Read session file for `holder_nonce` + `session_id`
- Read lock file, verify nonce+session match → mismatch = `E_LOCK_NOT_HELD`
- Update `last_renewed_at`, `lease_expires_at`, atomic write (tmp + rename + fsync)
- Post-check: re-read lock file, verify nonce still matches → mismatch = `E_LOCK_EXPIRED`

### Steal (internal, triggered by Acquire)

- Build new lock: `fencing_token = expired.FencingToken + 1`, `acquire_seq` incremented
- Write `.lock.tmp`, fsync
- `os.Rename(.lock.tmp → .lock)`, fsync parent dir
- Multiple stealer race: rename winner gets the lock; losers' tmp files are harmless (doctor cleans)
- Mandatory audit event (`lock_steal`)

### Release

- Read session file for nonce+session
- Verify match → mismatch = `E_LOCK_NOT_HELD`
- `os.Remove` lock file + session file
- Audit event

### Fencing validation

```go
func (m *Manager) ValidateFencing(ctx context.Context, worktreeID string, expectedToken uint64) error
```
Reads current lock file fencing_token; mismatch → `E_FENCING_MISMATCH`. Called before all irreversible mutations (snapshot step 8.5, restore step 5.5).

### Clock skew detection

On steal evaluation: `observed_skew = abs(local_now - lock.lease_expires_at - lock.lease_duration_ms)`. If `> max_clock_skew_ms` → `E_CLOCK_SKEW_EXCEEDED`.

---

## 6. Integrity (internal/integrity/)

Two-layer model (v0.x, no signing):

### Descriptor checksum

```go
func ComputeDescriptorChecksum(d *model.Descriptor) (HashValue, error) {
    // 1. Copy descriptor, zero out: descriptor_checksum, integrity_state
    // 2. jsonutil.CanonicalMarshal() → sorted keys, no whitespace, UTF-8
    // 3. SHA-256
}
```

### Payload root hash (spec 05)

```go
func ComputePayloadRootHash(dir string) (HashValue, error) {
    // 1. Walk recursively in byte-order sorted path order
    // 2. Each entry: "<type>:<relative_path>:<metadata>:<content_hash>\n"
    //    - file: content_hash=SHA-256(file), metadata=mode:size
    //    - symlink: content_hash=SHA-256(target), metadata=empty
    //    - dir: content_hash=empty, metadata=empty
    // 3. SHA-256 of concatenated result
}
```

Deterministic: same payload always produces same hash (conformance test 19).

---

## 7. Audit Log (internal/audit/)

```go
type Appender interface {
    Append(ctx context.Context, rec AppendInput) error
}

type FileAppender struct {
    auditDir string
    mu       sync.Mutex
}
```

### Append flow

```go
func (a *FileAppender) Append(ctx context.Context, input AppendInput) error {
    a.mu.Lock()         // in-process
    defer a.mu.Unlock()

    f := os.OpenFile(auditPath, O_WRONLY|O_CREATE|O_APPEND, 0644)
    syscall.Flock(f.Fd(), LOCK_EX)  // cross-process (safe for future shared mode)
    defer syscall.Flock(f.Fd(), LOCK_UN)

    prevHash := readLastRecordHash(f)    // reverse scan from EOF
    rec := buildAuditRecord(input, prevHash)
    rec.RecordHash = computeRecordHash(rec) // canonical JSON → SHA-256
    writeJSONLine(f, rec)
    f.Sync()
}
```

### Hash chain

Each record's `prev_hash` links to prior `record_hash`. `doctor --strict` and `verify --all` validate the full chain. Break → `E_AUDIT_CHAIN_BROKEN`.

### Rotation

When `audit.jsonl` exceeds 100 MB: write chain-closing record, rename to `audit-<timestamp>.jsonl`, new file gets chain-opening record with `prev_hash` referencing old file's last hash. Cross-file chain continuity preserved.

---

## 8. Restore (internal/restore/)

### Safe restore (default)

`jvs restore <snapshot-id> [--name <worktree>]`

1. Validate snapshot exists + has .READY marker
2. Verify descriptor checksum + payload hash
3. Auto-generate name if not specified: `restore-<shortid>-<YYYYMMDD>-<HHMMSS>-<rand4>`
4. Name safety check (`pathutil.ValidateName`)
5. Create `repo/worktrees/<name>/` via engine materialization
6. Write `.jvs/worktrees/<name>/config.json` (base_snapshot_id + head_snapshot_id = source snapshot; new lineage branch)
7. Audit record, return absolute path

### In-place restore (dangerous)

`jvs restore <id> --inplace --force --reason <text>`

Hard checks (--force does NOT bypass):
1. Caller holds valid lock → `E_LOCK_NOT_HELD`
2. Fencing token matches → `E_FENCING_MISMATCH`
3. Snapshot checksum + payload hash pass
4. `--reason` non-empty

Execution:
5. Record pre-restore state (head, holder_id, fencing_token, decision_id, reason)
5.5. **Re-validate fencing token**
6. Rename current payload to `<worktree>.pre-restore.tmp/`
7. Engine materialize snapshot to original path
8. Success → delete `.pre-restore.tmp/`; Failure → rename back; Both fail → explicit failed state + recovery steps
9. Update head in config.json
10. Audit record

---

## 9. GC (internal/gc/)

### Protection rules (non-deletable)

- All worktree head snapshots
- Ancestors reachable from protected heads (lineage traversal)
- Pinned snapshots (unexpired)
- Snapshots referenced by active intents
- Snapshots referenced by refs

Note: v0.x has no pin CLI. Pins managed via `.jvs/gc/pins/<pin_id>.json` files. CLI planned for v1.x.

### `jvs gc plan`

Read-only, deterministic. Same inputs → same candidate set (sorted by snapshot ID).
Writes plan to `.jvs/gc/<plan_id>.json`.

### `jvs gc run --plan-id <id>`

**Phase A (Mark):** Load plan → revalidate candidates (mismatch → `E_GC_PLAN_MISMATCH`) → write tombstones with `gc_state=marked`.

**Phase B (Commit):** Per tombstone: delete snapshot dir + descriptor file → `gc_state=committed`. Single failure → stop immediately, `gc_state=failed` + reason. Batch audit event.

**Retry safety:** Rerun same plan-id continues from failed tombstones. Already committed = skip (idempotent).

---

## 10. Doctor (internal/doctor/)

```go
type Finding struct {
    Severity     string       // "error"|"warning"|"info"
    Code         string       // e.g. "head_orphan", "orphan_tmp"
    Message      string
    RepairAction *RepairAction
}

type RepairAction struct {
    Name       string // "clean_tmp"|"rebuild_index"|"audit_repair"|
                      // "advance_head"|"clean_locks"|"clean_intents"
    AutoRepair bool   // true for --repair-runtime subset
}
```

### `doctor --strict` checks

1. **Layout:** format_version valid, .jvs/ not in payload roots, all worktrees have config.json, payload roots clean
2. **Snapshots:** all have .READY; orphan tmp → `clean_tmp` repair
3. **Lineage:** parent chain no cycles, parent descriptors exist, head points to valid READY snapshot; head orphan → `advance_head` repair
4. **Descriptors:** checksum validation, payload hash validation (mandatory in --strict)
5. **Audit chain:** prev_hash → record_hash chain validation; break → `E_AUDIT_CHAIN_BROKEN`; `audit_repair` can recompute from existing records
6. **Runtime state:** expired locks → `clean_locks`; completed/abandoned intents → `clean_intents`; stale index → `rebuild_index`
7. **Clock skew:** warn if offset > `max_clock_skew_ms / 2`

### `--repair-runtime` auto-fixes

Safe subset only: `clean_locks`, `clean_intents`, `rebuild_index`. Other repairs (advance_head, clean_tmp, audit_repair) reported but require explicit action.

---

## 11. CLI Layer (internal/cli/)

### Command tree

```
jvs
├── init <name> [--json]
├── info [--json]
├── doctor [--strict] [--repair-runtime] [--json]
├── verify [--snapshot <id>|--all] [--json]
├── conformance run [--profile dev|release] [--json]
├── snapshot [note] [--consistency quiesced|best_effort] [--json]
├── history [--limit N] [--json]
├── restore <snapshot-id> [--name <worktree>] [--json]
│   └── (with --inplace --force --reason <text>)
├── worktree
│   ├── create <name> [--from <snapshot-id>]
│   ├── list [--json]
│   ├── path <name>
│   ├── rename <old> <new>
│   └── remove <name> [--force]
├── lock
│   ├── acquire [--worktree <name>] [--lease-ms <n>] [--json]
│   ├── status [--worktree <name>] [--json]
│   ├── renew [--worktree <name>] [--json]
│   └── release [--worktree <name>] [--json]
├── gc
│   ├── plan [--policy <name>] [--json]
│   └── run --plan-id <id> [--json]
└── ref
    ├── create <name> <snapshot-id> [--json]
    ├── list [--json]
    └── delete <name> [--json]
```

### Design principles

- **One file per command group** (init.go, snapshot.go, worktree.go, lock.go, gc.go, ref.go, etc.)
- **No PersistentPreRunE for discovery.** Explicit helper functions per command:
  ```go
  func requireRepo(cmd *cobra.Command) (*repo.Repo, error)
  func requireWorktree(cmd *cobra.Command) (*repo.Repo, *worktree.Context, error)
  ```
- **`--json` as PersistentFlag** on root command. JSON mode: structured output to stdout, human info to stderr.
- **Error output:** All errors map to `errclass` codes. JSON mode: `{"error": "E_LOCK_CONFLICT", "message": "..."}` + non-zero exit.
- **Input safety:** All `<name>` args validated via `pathutil.ValidateName()` before entering domain logic.
- **No CWD mutation:** CLI reads CWD for discovery, never chdir.

### History command

`jvs history [--limit N] [--json]`

Traverses from `config.json` head_snapshot_id, follows `parent_snapshot_id` chain through descriptors. Collects up to N records. Marks `best_effort` snapshots with risk label in output.

---

## 12. Utility Layer (pkg/)

### jsonutil — Canonical JSON

```go
func CanonicalMarshal(v any) ([]byte, error)
```

Process: `json.Marshal(v)` → unmarshal to `any` → recursively sort all map keys → re-serialize with no whitespace. Handles: sorted keys, no whitespace, UTF-8, RFC 8259 escaping, number normalization.

Foundation for descriptor checksum and audit record hash. Requires comprehensive tests: null, unicode, nested objects, float edge cases.

### pathutil — Path and Name Safety

```go
func ValidateName(name string) error        // NFC normalize → regex → reject ".." / separators
func ValidatePathSafety(repoRoot, target string) error  // resolve symlinks → prefix check
```

`ValidatePathSafety` handles non-existent targets by resolving the closest existing ancestor directory, then appending the remaining path components for prefix check.

### fsutil — Filesystem Primitives

```go
func AtomicWrite(path string, data []byte, perm os.FileMode) error  // write tmp + fsync + rename + fsync parent
func RenameAndSync(old, new string) error                            // rename + fsync parent
func FsyncDir(dir string) error                                     // fsync directory for rename visibility
func ReflinkProbe(dir string) bool                                  // actual reflink test on temp file
```

### errclass — Stable Error Classes

```go
var (
    ErrNameInvalid       = &JVSError{Code: "E_NAME_INVALID", Exit: 1}
    ErrPathEscape        = &JVSError{Code: "E_PATH_ESCAPE", Exit: 1}
    ErrLockConflict      = &JVSError{Code: "E_LOCK_CONFLICT", Exit: 1}
    ErrLockExpired       = &JVSError{Code: "E_LOCK_EXPIRED", Exit: 1}
    ErrLockNotHeld       = &JVSError{Code: "E_LOCK_NOT_HELD", Exit: 1}
    ErrFencingMismatch   = &JVSError{Code: "E_FENCING_MISMATCH", Exit: 1}
    ErrClockSkewExceeded = &JVSError{Code: "E_CLOCK_SKEW_EXCEEDED", Exit: 1}
    ErrConsistencyUnavail= &JVSError{Code: "E_CONSISTENCY_UNAVAILABLE", Exit: 1}
    ErrDescriptorCorrupt = &JVSError{Code: "E_DESCRIPTOR_CORRUPT", Exit: 1}
    ErrPayloadHashMismatch= &JVSError{Code: "E_PAYLOAD_HASH_MISMATCH", Exit: 1}
    ErrLineageBroken     = &JVSError{Code: "E_LINEAGE_BROKEN", Exit: 1}
    ErrPartialSnapshot   = &JVSError{Code: "E_PARTIAL_SNAPSHOT", Exit: 1}
    ErrGCPlanMismatch    = &JVSError{Code: "E_GC_PLAN_MISMATCH", Exit: 1}
    ErrFormatUnsupported = &JVSError{Code: "E_FORMAT_UNSUPPORTED", Exit: 1}
    ErrAuditChainBroken  = &JVSError{Code: "E_AUDIT_CHAIN_BROKEN", Exit: 1}
)

func (e *JVSError) Is(target error) bool {
    t, ok := target.(*JVSError)
    return ok && e.Code == t.Code
}

func (e *JVSError) WithMessage(msg string) *JVSError // new instance, preserves Code
```

15 error classes (v0.x). Removed: `E_SIGNATURE_INVALID`, `E_SIGNING_KEY_MISSING`, `E_TRUST_POLICY_VIOLATION`.

### uuidutil — UUID v4

```go
func NewV4() string // 16 bytes crypto/rand + version/variant bits → standard format
```

---

## 13. Testing Strategy

### Three layers

**Unit tests** (per package, `*_test.go`): test individual functions in isolation using `t.TempDir()`.

**Conformance tests** (`test/conformance/`, build tag `conformance`): 29 mandatory tests from spec 11. Each test named `TestConformance_XX_Description`. Primarily black-box via CLI binary execution.

**Integration tests** (`test/integration/`): end-to-end workflows crossing multiple subsystems.

### Conformance test file mapping

| File | Tests |
|------|-------|
| lock_test.go | 1, 2, 3, 17, 23 |
| snapshot_test.go | 4, 5, 18, 19, 24 |
| restore_test.go | 6 |
| path_test.go | 7 |
| integrity_test.go | 8, 9 |
| doctor_test.go | 10, 21, 25 |
| migration_test.go | 11, 29 |
| gc_test.go | 12, 13, 22 |
| audit_test.go | 14, 15 |
| format_test.go | 16 |
| ref_test.go | 20 |
| init_test.go | 27 |
| worktree_test.go | 26, 28 |

### Test policy

- Conformance tests use short lease durations (100ms lease, 50ms skew/grace) — no 30s waits
- All tests use real filesystem (`t.TempDir()`), no mocks
- Build tag `conformance` prevents accidental execution via `go test ./...`
- `jvs conformance run` CLI command runs embedded checks against current repo (doctor + verify + additional conformance logic)

---

## 14. Build and Development

### go.mod

```
module github.com/jvs-project/jvs

go 1.26

require (
    github.com/spf13/cobra v1.8.x
    golang.org/x/text v0.x.x
)

require (
    github.com/stretchr/testify v1.9.x // test
)
```

### Makefile

```makefile
build:
	go build -o bin/jvs ./cmd/jvs

test:
	go test ./internal/... ./pkg/...

conformance:
	go test -tags conformance ./test/conformance/ -v -count=1

lint:
	golangci-lint run ./...

verify: test conformance lint
```

### cmd/jvs/main.go

```go
package main

import (
    "os"
    "github.com/jvs-project/jvs/internal/cli"
)

func main() {
    if err := cli.Execute(); err != nil {
        os.Exit(1)
    }
}
```

### Development phases (dependency-ordered)

```
Phase 1: Foundation (no domain dependencies)
  pkg/errclass → pkg/uuidutil → pkg/jsonutil → pkg/pathutil → pkg/fsutil

Phase 2: Core domain
  pkg/model → internal/repo → internal/audit → internal/integrity

Phase 3: Engines and operations
  internal/engine (copy → reflink → juicefs)
  internal/lock → internal/worktree → internal/snapshot → internal/restore

Phase 4: Management and verification
  internal/ref → internal/gc → internal/verify → internal/doctor

Phase 5: CLI integration
  internal/cli (all commands)

Phase 6: Conformance
  test/conformance/ (29 tests pass)
```

Each phase: complete unit tests before proceeding. Phase 5: end-to-end smoke test. Phase 6: final quality gate.

---

## 15. v0.x Scope Notes

### Included
- Full exclusive-mode SWMR with lock/lease/fencing
- Three snapshot engines (juicefs-clone, reflink-copy, copy)
- 12-step atomic publish protocol with READY marker
- Two-layer integrity (descriptor checksum + payload root hash)
- Audit hash chain with rotation
- Safe + in-place restore
- Two-phase GC with plan/mark/commit
- Named refs
- Doctor diagnosis with 6 repair actions
- 29 conformance tests

### Deferred to v1.x
- `shared` isolation mode
- Descriptor signing (Ed25519) and trust policy (keyring, revocations)
- `--allow-unsigned` verify flag
- `jvs gc pin/unpin` CLI
- `index.sqlite` performance optimization

### Design additions beyond spec
- `.jvs/repo_id` file — UUID generated at init for key path resolution (future signing support). Portable history state.
- `.jvs/locks/<worktree_id>.session` — persists holder_nonce + session_id across CLI invocations. Runtime state (non-portable).
