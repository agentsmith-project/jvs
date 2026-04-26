// Package restore handles snapshot restore operations.
package restore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jvs-project/jvs/internal/audit"
	"github.com/jvs-project/jvs/internal/engine"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/snapshotpayload"
	"github.com/jvs-project/jvs/internal/verify"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/fsutil"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/jvs-project/jvs/pkg/pathutil"
	"github.com/jvs-project/jvs/pkg/uuidutil"
)

// Restorer handles snapshot restore operations.
type Restorer struct {
	repoRoot    string
	engineType  model.EngineType
	engine      engine.Engine
	auditLogger *audit.FileAppender
	updateHead  func(*worktree.Manager, string, model.SnapshotID) error
}

var (
	restoreRename   = os.Rename
	restoreFsyncDir = fsutil.FsyncDir
)

// NewRestorer creates a new restorer.
func NewRestorer(repoRoot string, engineType model.EngineType) *Restorer {
	eng := engine.NewEngine(engineType)

	auditPath := filepath.Join(repoRoot, ".jvs", "audit", "audit.jsonl")
	return &Restorer{
		repoRoot:    repoRoot,
		engineType:  engineType,
		engine:      eng,
		auditLogger: audit.NewFileAppender(auditPath),
		updateHead: func(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
			return wtMgr.UpdateHead(worktreeName, snapshotID)
		},
	}
}

// Restore replaces the content of a worktree with a snapshot.
// This puts the worktree into a "detached" state (unless restoring to HEAD).
// The worktree is specified by name, not derived from the snapshot.
func (r *Restorer) Restore(worktreeName string, snapshotID model.SnapshotID) error {
	return repo.WithMutationLock(r.repoRoot, "restore", func() error {
		return r.restore(worktreeName, snapshotID)
	})
}

// restore performs the actual restore operation.
func (r *Restorer) restore(worktreeName string, snapshotID model.SnapshotID) error {
	if worktreeName == "" {
		return fmt.Errorf("worktree name is required")
	}
	if snapshotID == "" {
		return fmt.Errorf("snapshot ID is required")
	}
	if err := snapshotID.Validate(); err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	// Load and verify snapshot
	desc, err := snapshot.LoadDescriptor(r.repoRoot, snapshotID)
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}
	if desc.SnapshotID != snapshotID {
		return fmt.Errorf("load snapshot: descriptor snapshot ID %s does not match requested %s", desc.SnapshotID, snapshotID)
	}

	verifyResult, err := verify.NewVerifier(r.repoRoot).VerifySnapshot(snapshotID, true)
	if err != nil {
		return fmt.Errorf("verify snapshot: %w", err)
	}
	if verifyResult.TamperDetected {
		return verifySnapshotResultError(verifyResult, "tamper detected")
	}

	// Get worktree info
	wtMgr := worktree.NewManager(r.repoRoot)
	cfg, err := wtMgr.Get(worktreeName)
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	payloadPath, err := wtMgr.Path(worktreeName)
	if err != nil {
		return fmt.Errorf("worktree payload path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(payloadPath); err != nil {
		return err
	}
	if err := r.auditLogger.EnsureAppendable(); err != nil {
		return fmt.Errorf("audit log not appendable: %w", err)
	}

	// Create backup directory for rollback while keeping payloadPath itself in place.
	backupPath := payloadPath + ".restore-backup-" + uuidutil.NewV4()[:8]
	snapshotDir, err := repo.SnapshotPathForRead(r.repoRoot, snapshotID)
	if err != nil {
		return fmt.Errorf("snapshot path: %w", err)
	}
	tempPath := payloadPath + ".restore-tmp-" + uuidutil.NewV4()[:8]

	// Step 1: Materialize logical snapshot payload to temp location
	if err := snapshotpayload.Materialize(snapshotDir, tempPath, snapshotpayload.OptionsFromDescriptor(desc), func(src, dst string) error {
		_, err := r.engine.Clone(src, dst)
		return err
	}); err != nil {
		os.RemoveAll(tempPath)
		return fmt.Errorf("materialize snapshot: %w", err)
	}
	defer os.RemoveAll(tempPath)
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(tempPath); err != nil {
		return fmt.Errorf("materialized snapshot payload: %w", err)
	}

	// Step 2: Replace or overlay contents inside the existing payload root.
	var partialChanges []partialChange
	if len(desc.PartialPaths) > 0 {
		partialChanges, err = overlayPartialPayload(payloadPath, tempPath, backupPath, desc.PartialPaths)
		if err != nil {
			return fmt.Errorf("overlay partial restore: %w", err)
		}
	} else {
		if err := replacePayloadContents(payloadPath, tempPath, backupPath); err != nil {
			return fmt.Errorf("replace payload contents: %w", err)
		}
	}

	// Step 3: Update head (NOT latest - this puts worktree in detached state)
	if err := r.updateHead(wtMgr, worktreeName, snapshotID); err != nil {
		if headUpdated, headErr := headMatchesSnapshot(wtMgr, worktreeName, snapshotID); headErr == nil && headUpdated {
			return retainBackup(backupPath, fmt.Errorf("update head: %w (metadata points to restored snapshot; payload left restored)", err))
		} else if headErr != nil {
			err = fmt.Errorf("%w (inspect head after failure: %v)", err, headErr)
		}
		if rollbackErr := rollbackRestoredPayload(payloadPath, backupPath, len(desc.PartialPaths) > 0, partialChanges); rollbackErr != nil {
			return retainBackup(backupPath, fmt.Errorf("update head: %w (rollback failed: %v)", err, rollbackErr))
		}
		return retainBackup(backupPath, fmt.Errorf("update head: %w (payload rolled back)", err))
	}

	// Step 4: Cleanup backup synchronously after metadata has been updated.
	if err := os.RemoveAll(backupPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to cleanup backup %s: %v\n", backupPath, err)
	}

	// Determine if we're now detached
	isDetached := snapshotID != cfg.LatestSnapshotID

	// Audit log
	if err := r.auditLogger.Append(model.EventTypeRestore, worktreeName, snapshotID, map[string]any{
		"detached": isDetached,
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	return nil
}

func verifySnapshotResultError(result *verify.Result, fallback string) error {
	message := fallback
	if result != nil && result.Error != "" {
		message = result.Error
	}
	if result != nil && result.ErrorCode != "" {
		return &errclass.JVSError{Code: result.ErrorCode, Message: fmt.Sprintf("verify snapshot: %s", message)}
	}
	return fmt.Errorf("verify snapshot: %s", message)
}

func replacePayloadContents(payloadPath, tempPath, backupPath string) error {
	if err := validateRealDir(payloadPath); err != nil {
		return err
	}
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	var backupMoves []string
	if err := moveContentsWithLedger(payloadPath, backupPath, func(rel string) {
		backupMoves = append(backupMoves, rel)
	}); err != nil {
		return rollbackFullBackupFailure(payloadPath, backupPath, backupMoves, fmt.Errorf("backup current contents: %w", err))
	}
	if err := moveContents(tempPath, payloadPath); err != nil {
		if rollbackErr := rollbackFullRestore(payloadPath, backupPath); rollbackErr != nil {
			return retainBackup(backupPath, fmt.Errorf("move restored contents: %w (rollback failed: %v)", err, rollbackErr))
		}
		return fmt.Errorf("move restored contents: %w", err)
	}
	if err := restoreFsyncDir(payloadPath); err != nil {
		return retainBackup(backupPath, fmt.Errorf("fsync restored payload: %w", err))
	}
	return nil
}

func overlayPartialPayload(payloadPath, tempPath, backupPath string, partialPaths []string) ([]partialChange, error) {
	paths, err := normalizePartialPaths(partialPaths)
	if err != nil {
		return nil, err
	}
	if err := preflightPartialPayload(payloadPath, tempPath, paths); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return nil, fmt.Errorf("create backup: %w", err)
	}

	var changes []partialChange
	for _, rel := range paths {
		src := filepath.Join(tempPath, rel)
		if _, err := os.Lstat(src); err != nil {
			if rollbackErr := rollbackPartialRestore(payloadPath, backupPath, changes); rollbackErr != nil {
				return nil, retainBackup(backupPath, fmt.Errorf("stat partial source %s: %w (rollback failed: %v)", rel, err, rollbackErr))
			}
			return nil, fmt.Errorf("stat partial source %s: %w", rel, err)
		}

		dst := filepath.Join(payloadPath, rel)
		backup := filepath.Join(backupPath, rel)
		change := partialChange{rel: rel}
		if _, err := os.Lstat(dst); err == nil {
			change.hadOriginal = true
			if err := os.MkdirAll(filepath.Dir(backup), 0755); err != nil {
				return nil, rollbackPartialBackupFailure(payloadPath, backupPath, changes, fmt.Errorf("create partial backup parent %s: %w", rel, err))
			}
			recordedBackupMove := false
			if err := moveEntryWithLedger(dst, backup, func() {
				changes = append(changes, change)
				recordedBackupMove = true
			}); err != nil {
				return nil, rollbackPartialBackupFailure(payloadPath, backupPath, changes, fmt.Errorf("backup partial path %s: %w", rel, err))
			}
			if !recordedBackupMove {
				changes = append(changes, change)
			}
		} else if !os.IsNotExist(err) {
			return nil, rollbackPartialBackupFailure(payloadPath, backupPath, changes, fmt.Errorf("stat partial destination %s: %w", rel, err))
		}
		if !change.hadOriginal {
			changes = append(changes, change)
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			if rollbackErr := rollbackPartialRestore(payloadPath, backupPath, changes); rollbackErr != nil {
				return nil, retainBackup(backupPath, fmt.Errorf("create partial destination parent %s: %w (rollback failed: %v)", rel, err, rollbackErr))
			}
			return nil, fmt.Errorf("create partial destination parent %s: %w", rel, err)
		}
		if err := pathutil.ValidateNoSymlinkParents(payloadPath, rel); err != nil {
			if rollbackErr := rollbackPartialRestore(payloadPath, backupPath, changes); rollbackErr != nil {
				return nil, retainBackup(backupPath, fmt.Errorf("validate partial destination parent %s: %w (rollback failed: %v)", rel, err, rollbackErr))
			}
			return nil, fmt.Errorf("validate partial destination parent %s: %w", rel, err)
		}
		if err := moveEntry(src, dst); err != nil {
			if moveRenamed(err) {
				return nil, retainBackup(backupPath, fmt.Errorf("restore partial path %s: %w", rel, err))
			}
			if rollbackErr := rollbackPartialRestore(payloadPath, backupPath, changes); rollbackErr != nil {
				return nil, retainBackup(backupPath, fmt.Errorf("restore partial path %s: %w (rollback failed: %v)", rel, err, rollbackErr))
			}
			return nil, fmt.Errorf("restore partial path %s: %w", rel, err)
		}
	}

	if err := restoreFsyncDir(payloadPath); err != nil {
		return nil, retainBackup(backupPath, fmt.Errorf("fsync restored payload: %w", err))
	}
	return changes, nil
}

type partialChange struct {
	rel         string
	hadOriginal bool
}

func moveContents(srcRoot, dstRoot string) error {
	return moveContentsWithLedger(srcRoot, dstRoot, nil)
}

func moveContentsWithLedger(srcRoot, dstRoot string, recordMove func(rel string)) error {
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcRoot, err)
	}
	if err := os.MkdirAll(dstRoot, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dstRoot, err)
	}
	for _, entry := range entries {
		rel := entry.Name()
		if err := moveEntryWithLedger(filepath.Join(srcRoot, rel), filepath.Join(dstRoot, rel), func() {
			if recordMove != nil {
				recordMove(rel)
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

func moveEntry(src, dst string) error {
	return moveEntryWithLedger(src, dst, nil)
}

func moveEntryWithLedger(src, dst string, recordMove func()) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create parent for %s: %w", dst, err)
	}
	if err := restoreRename(src, dst); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	if recordMove != nil {
		recordMove()
	}
	if err := restoreFsyncDir(filepath.Dir(dst)); err != nil {
		return &moveEntryError{
			renamed: true,
			err:     fmt.Errorf("fsync rename destination parent: %w", err),
		}
	}
	if err := restoreFsyncDir(filepath.Dir(src)); err != nil {
		return &moveEntryError{
			renamed: true,
			err:     fmt.Errorf("fsync rename source parent: %w", err),
		}
	}
	return nil
}

func rollbackFullBackupFailure(payloadPath, backupPath string, movedEntries []string, err error) error {
	if rollbackErr := rollbackMovedBackupEntries(payloadPath, backupPath, movedEntries); rollbackErr != nil {
		return retainBackup(backupPath, fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr))
	}
	return cleanupBackupAfterRollback(backupPath, err)
}

func rollbackMovedBackupEntries(payloadPath, backupPath string, movedEntries []string) error {
	for i := len(movedEntries) - 1; i >= 0; i-- {
		rel := movedEntries[i]
		if err := moveEntry(filepath.Join(backupPath, rel), filepath.Join(payloadPath, rel)); err != nil {
			return fmt.Errorf("restore backup entry %s: %w", rel, err)
		}
	}
	return nil
}

func rollbackPartialBackupFailure(payloadPath, backupPath string, changes []partialChange, err error) error {
	if rollbackErr := rollbackPartialRestore(payloadPath, backupPath, changes); rollbackErr != nil {
		return retainBackup(backupPath, fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr))
	}
	return cleanupBackupAfterRollback(backupPath, err)
}

func cleanupBackupAfterRollback(backupPath string, err error) error {
	if cleanupErr := os.RemoveAll(backupPath); cleanupErr != nil {
		return fmt.Errorf("%w (payload rolled back; cleanup backup: %v)", err, cleanupErr)
	}
	return fmt.Errorf("%w (payload rolled back)", err)
}

func rollbackFullRestore(payloadPath, backupPath string) error {
	if err := clearDirectory(payloadPath); err != nil {
		return err
	}
	return moveContents(backupPath, payloadPath)
}

func rollbackPartialRestore(payloadPath, backupPath string, changes []partialChange) error {
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if err := removeContained(payloadPath, change.rel); err != nil {
			return err
		}
		if change.hadOriginal {
			dst := filepath.Join(payloadPath, change.rel)
			if err := moveEntry(filepath.Join(backupPath, change.rel), dst); err != nil {
				return err
			}
		}
	}
	return nil
}

func rollbackRestoredPayload(payloadPath, backupPath string, partial bool, changes []partialChange) error {
	if partial {
		return rollbackPartialRestore(payloadPath, backupPath, changes)
	}
	return rollbackFullRestore(payloadPath, backupPath)
}

func headMatchesSnapshot(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) (bool, error) {
	cfg, err := wtMgr.Get(worktreeName)
	if err != nil {
		return false, err
	}
	return cfg.HeadSnapshotID == snapshotID, nil
}

func clearDirectory(root string) error {
	if err := validateRealDir(root); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read %s: %w", root, err)
	}
	for _, entry := range entries {
		if err := removeContained(root, entry.Name()); err != nil {
			return fmt.Errorf("remove %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func normalizePartialPaths(paths []string) ([]string, error) {
	normalized := make([]string, 0, len(paths))
	for _, p := range paths {
		clean, err := pathutil.CleanRel(p)
		if err != nil {
			return nil, fmt.Errorf("invalid partial path: %s: %w", p, err)
		}
		normalized = append(normalized, clean)
	}
	sort.Strings(normalized)

	collapsed := make([]string, 0, len(normalized))
	for _, p := range normalized {
		covered := false
		for _, existing := range collapsed {
			rel, err := filepath.Rel(existing, p)
			if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))) {
				covered = true
				break
			}
		}
		if !covered {
			collapsed = append(collapsed, p)
		}
	}
	return collapsed, nil
}

func preflightPartialPayload(payloadPath, tempPath string, paths []string) error {
	for _, rel := range paths {
		if err := pathutil.ValidateNoSymlinkParents(payloadPath, rel); err != nil {
			return fmt.Errorf("destination parent containment for %s: %w", rel, err)
		}
		if err := pathutil.ValidateNoSymlinkParents(tempPath, rel); err != nil {
			return fmt.Errorf("source parent containment for %s: %w", rel, err)
		}
		if _, err := os.Lstat(filepath.Join(tempPath, rel)); err != nil {
			return fmt.Errorf("stat partial source %s: %w", rel, err)
		}
	}
	return nil
}

func validateRealDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not directory: %s", path)
	}
	return nil
}

func removeContained(root, rel string) error {
	if err := pathutil.ValidateNoSymlinkParents(root, rel); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(root, rel))
}

type moveEntryError struct {
	renamed bool
	err     error
}

func (e *moveEntryError) Error() string {
	return e.err.Error()
}

func (e *moveEntryError) Unwrap() error {
	return e.err
}

func moveRenamed(err error) bool {
	var moveErr *moveEntryError
	return errors.As(err, &moveErr) && moveErr.renamed
}

type backupRetainedError struct {
	backupPath string
	err        error
}

func (e *backupRetainedError) Error() string {
	return fmt.Sprintf("%v; backup retained at %s", e.err, e.backupPath)
}

func (e *backupRetainedError) Unwrap() error {
	return e.err
}

func retainBackup(backupPath string, err error) error {
	return &backupRetainedError{backupPath: backupPath, err: err}
}

// RestoreToLatest restores a worktree to its latest snapshot (exits detached state).
func (r *Restorer) RestoreToLatest(worktreeName string) error {
	wtMgr := worktree.NewManager(r.repoRoot)
	cfg, err := wtMgr.Get(worktreeName)
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	if cfg.LatestSnapshotID == "" {
		return fmt.Errorf("worktree has no snapshots")
	}

	return r.Restore(worktreeName, cfg.LatestSnapshotID)
}
