# JVS Go Library API Documentation

**Version:** v0 public contract
**Last Updated:** 2026-04-28

## Overview

JVS can be used as a Go library for programmatic save point workflows. The
stable v0 Go facade is `pkg/jvs`. It follows the active save point CLI
vocabulary: save, history, view, restore, workspace, doctor, recovery, and
cleanup.

Product docs, examples, release notes, and integration guides should use the
facade names in this document. Internal package names, storage paths, and
storage-shaped record fields do not define public product vocabulary.

| Package | Boundary | Purpose |
| --- | --- | --- |
| `pkg/jvs` | Stable | Stable v0 client facade |
| `pkg/errclass` | Stable | Stable error classes |
| `pkg/model` | Existing type support | Returned data types and storage-shaped records |
| `pkg/config` | Existing type support | Configuration support for existing storage |
| `pkg/uuidutil` | Internal-only | Implementation utility |
| `pkg/pathutil` | Internal-only | Implementation utility |
| `pkg/fsutil` | Internal-only | Implementation utility |
| `pkg/jsonutil` | Internal-only | Implementation utility |
| `pkg/logging` | Internal-only | Implementation utility |
| `pkg/progress` | Internal-only | Implementation utility |

## Stable v0 Go Facade: pkg/jvs

Use `pkg/jvs` for integrations that initialize a folder, create save points,
read history, restore work, and run reviewed cleanup.

### Client Setup

```go
client, err := jvs.OpenOrInit(workspacePath, jvs.InitOptions{Name: "workspace"})
```

`OpenOrInit` opens an existing JVS project or adopts the folder when needed.

### Save Point Creation

```go
savePoint, err := client.Save(ctx, jvs.SaveOptions{
    Message: "initial save point",
})
```

`Save` creates an immutable save point from the managed files in the selected
workspace. The returned value is save point metadata suitable for logging,
audit evidence, and later restore selection.

### History

```go
history, err := client.History(ctx, "main", 20)
```

History lists save points without changing files. Integrations should choose a
concrete save point ID before restore or view operations.

### Restore

CLI restore is preview-first. The Go facade restores a workspace to a concrete
save point selected by the integration. `Target` must be a save point ID or ID
prefix; labels, messages, and tags are discovery metadata, not restore targets.

```go
err := client.Restore(ctx, jvs.RestoreOptions{
    WorkspaceName: "main",
    Target:        savePoint.SavePointID.String(),
})
```

### Cleanup

Cleanup is also two-stage: preview first, then run a reviewed plan.

```go
plan, err := client.PreviewCleanup(ctx, jvs.CleanupOptions{})
if err != nil {
    return err
}

err = client.RunCleanup(ctx, plan.PlanID)
```

Public cleanup plan fields use save point terminology:

```go
type CleanupPlan struct {
    PlanID                  string    `json:"plan_id"`
    CreatedAt               time.Time `json:"created_at"`
    ProtectedSavePoints     []SavePointID `json:"protected_save_points"`
    ProtectionGroups        []CleanupProtectionGroup `json:"protection_groups"`
    ProtectedByHistory      int       `json:"protected_by_history"`
    CandidateCount          int       `json:"candidate_count"`
    ReclaimableSavePoints   []SavePointID `json:"reclaimable_save_points"`
    ReclaimableBytesEstimate int64    `json:"reclaimable_bytes_estimate"`
}

type CleanupProtectionGroup struct {
    Reason     CleanupProtectionReason `json:"reason"`
    Count      int                     `json:"count"`
    SavePoints []SavePointID           `json:"save_points"`
}

type CleanupProtectionReason = string

const (
    CleanupProtectionReasonHistory         CleanupProtectionReason = "history"
    CleanupProtectionReasonOpenView        CleanupProtectionReason = "open_view"
    CleanupProtectionReasonActiveRecovery  CleanupProtectionReason = "active_recovery"
    CleanupProtectionReasonActiveOperation CleanupProtectionReason = "active_operation"
)
```

Public JSON fields: `plan_id`, `created_at`, `protected_save_points`,
`protection_groups`, `protected_by_history`, `candidate_count`,
`reclaimable_save_points`, and `reclaimable_bytes_estimate`.

Cleanup protection reason meanings:

- `history`: the save point is part of workspace history or a selected
  workspace base.
- `open_view`: the save point backs an open read-only view.
- `active_recovery`: the save point is referenced by an active recovery plan.
- `active_operation`: the save point is referenced by an active JVS operation
  that has not finished.

### EngineType

Engine constants identify the materialization engine used by save, view,
restore, workspace creation, and cleanup support operations.

```go
type EngineType string

const (
    EngineJuiceFSClone EngineType = "juicefs-clone"
    EngineReflinkCopy  EngineType = "reflink-copy"
    EngineCopy         EngineType = "copy"
)
```

## Quick Example

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/agentsmith-project/jvs/pkg/jvs"
)

func main() {
    ctx := context.Background()
    workspacePath := "."
    if len(os.Args) > 1 {
        workspacePath = os.Args[1]
    }

    client, err := jvs.OpenOrInit(workspacePath, jvs.InitOptions{Name: "workspace"})
    if err != nil {
        panic(err)
    }

    savePoint, err := client.Save(ctx, jvs.SaveOptions{
        Message: "initial save point",
    })
    if err != nil {
        panic(err)
    }

    fmt.Printf("Save point ID: %s\n", savePoint.SavePointID)
}
```

## Integration Example

Creating a save point programmatically through the v0 Go facade:

```go
package main

import (
    "context"
    "fmt"

    "github.com/agentsmith-project/jvs/pkg/jvs"
)

func CreateSavePoint(ctx context.Context, workspacePath, message string) (string, error) {
    client, err := jvs.OpenOrInit(workspacePath, jvs.InitOptions{Name: "workspace"})
    if err != nil {
        return "", fmt.Errorf("open or initialize JVS project: %w", err)
    }

    savePoint, err := client.Save(ctx, jvs.SaveOptions{
        Message: message,
    })
    if err != nil {
        return "", fmt.Errorf("create save point: %w", err)
    }

    return savePoint.SavePointID.String(), nil
}
```

## Existing Type Packages

`pkg/model` and `pkg/config` are importable because the v0 facade may return or
accept existing support types. They are support packages, not a broad stable
product API.

## pkg/model Storage Types

`pkg/model` contains storage-shaped records used behind the facade. New
integrations should prefer the save point and cleanup names exposed by
`pkg/jvs`; do not build user workflows around storage-shaped field names.

## pkg/config Storage Support

`pkg/config` supports existing configuration storage. New integrations should
open projects through `pkg/jvs` and avoid writing configuration files directly.

## pkg/errclass

`pkg/errclass` provides stable, machine-readable error classes for
user-facing errors.

```go
type JVSError struct {
    Code    string
    Message string
}
```

Representative stable codes include:

- `E_NAME_INVALID`
- `E_PATH_ESCAPE`
- `E_DESCRIPTOR_CORRUPT`
- `E_PAYLOAD_HASH_MISMATCH`
- `E_LINEAGE_BROKEN`
- `E_FORMAT_UNSUPPORTED`
- `E_AUDIT_CHAIN_BROKEN`

## Internal-Only Packages

The following importable support packages are internal-only for release
planning purposes. They may move, narrow, or change before v1 unless a symbol is
explicitly returned by `pkg/jvs`.

- `pkg/uuidutil`
- `pkg/pathutil`
- `pkg/fsutil`
- `pkg/jsonutil`
- `pkg/logging`
- `pkg/progress`

## Best Practices

1. Use `pkg/jvs` for project mutation.
2. Treat JVS control data as implementation-owned storage.
3. Treat storage-shaped names as implementation facts, not product vocabulary.
4. Use `errclass` checks for stable user-facing error classification.
5. Pass cancellation-aware contexts into client methods.

## Stability Guarantees

- **Stable**: the v0 public Go facade in `pkg/jvs` and stable error classes
  follow the release contract.
- **Existing type support**: `pkg/model` and `pkg/config` are importable
  implementation support and may change before v1 unless a type is explicitly
  returned by `pkg/jvs`.
- **Internal-only**: utility packages and all `internal/` packages are not for
  external use.

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - system design and components
- [02_CLI_SPEC.md](02_CLI_SPEC.md) - CLI command reference
- [CONTRIBUTING.md](../CONTRIBUTING.md) - contributing guidelines

For API changes between versions, see [99_CHANGELOG.md](99_CHANGELOG.md).
