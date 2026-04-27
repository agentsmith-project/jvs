package workspacepath

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot/publishstate"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

// ReconcilePathSources projects active path sources against the real workspace.
// Missing restored paths are cleared; changed restored paths are downgraded.
func ReconcilePathSources(repoRoot string, boundary repo.WorktreePayloadBoundary, sources model.PathSources) (model.PathSources, error) {
	reconciled := model.NewPathSources()
	for _, source := range sources.RestoredPaths() {
		if err := pathutil.ValidateNoSymlinkParents(boundary.Root, source.TargetPath); err != nil {
			return nil, fmt.Errorf("target path parent containment for %s: %w", source.TargetPath, err)
		}
		targetPath := filepath.Join(boundary.Root, filepath.FromSlash(source.TargetPath))
		if _, err := os.Lstat(targetPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat target path %s: %w", source.TargetPath, err)
		}

		expectedParent, err := os.MkdirTemp("", "jvs-path-source-reconcile-*")
		if err != nil {
			return nil, fmt.Errorf("create path source reconciliation area: %w", err)
		}
		expectedRoot := filepath.Join(expectedParent, "expected")
		if err := os.MkdirAll(expectedRoot, 0755); err != nil {
			os.RemoveAll(expectedParent)
			return nil, fmt.Errorf("create expected root: %w", err)
		}
		err = overlayExpectedPathSource(repoRoot, expectedRoot, source)
		if err == nil {
			var matches bool
			matches, err = ManagedPathEqual(boundary.Root, expectedRoot, source.TargetPath, boundary.ExcludesRelativePath)
			if err == nil {
				if restoreErr := reconciled.RestoreFromPath(source.TargetPath, source.SourceSnapshotID, source.SourcePath); restoreErr != nil {
					err = restoreErr
				} else if !matches {
					err = reconciled.MarkModified(source.TargetPath)
				}
			}
		}
		cleanupErr := os.RemoveAll(expectedParent)
		if err != nil {
			return nil, err
		}
		if cleanupErr != nil {
			return nil, fmt.Errorf("cleanup path source reconciliation area: %w", cleanupErr)
		}
	}
	return reconciled, nil
}

func MaterializeExpectedWorkspace(repoRoot string, cfg *model.WorktreeConfig, boundary repo.WorktreePayloadBoundary) (string, func(), error) {
	tempParent, err := os.MkdirTemp("", "jvs-expected-workspace-*")
	if err != nil {
		return "", nil, fmt.Errorf("create expected workspace area: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempParent)
	}
	expectedRoot := filepath.Join(tempParent, "expected")
	if cfg != nil && cfg.HeadSnapshotID != "" {
		if err := MaterializeSavePointPayload(repoRoot, cfg.HeadSnapshotID, expectedRoot); err != nil {
			cleanup()
			return "", nil, err
		}
		if err := repo.ValidateManagedPayloadOnly(boundary, expectedRoot); err != nil {
			cleanup()
			return "", nil, err
		}
	} else if err := os.MkdirAll(expectedRoot, 0755); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("create expected workspace root: %w", err)
	}

	if cfg != nil {
		for _, source := range cfg.PathSources.RestoredPaths() {
			if err := overlayExpectedPathSource(repoRoot, expectedRoot, source); err != nil {
				cleanup()
				return "", nil, err
			}
		}
	}
	return expectedRoot, cleanup, nil
}

func MaterializeSavePointPayload(repoRoot string, savePointID model.SnapshotID, targetRoot string) error {
	state, issue := publishstate.Inspect(repoRoot, savePointID, publishstate.Options{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})
	if issue != nil {
		return &errclass.JVSError{Code: issue.Code, Message: issue.Message}
	}
	opts := snapshotpayload.OptionsFromDescriptor(state.Descriptor)
	if err := snapshotpayload.MaterializeToNew(state.SnapshotDir, targetRoot, opts, func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	}); err != nil {
		return fmt.Errorf("materialize save point: %w", err)
	}
	return nil
}

func overlayExpectedPathSource(repoRoot, expectedRoot string, source model.RestoredPathSource) error {
	tempParent, err := os.MkdirTemp("", "jvs-expected-path-source-*")
	if err != nil {
		return fmt.Errorf("create expected path source area: %w", err)
	}
	defer os.RemoveAll(tempParent)

	sourceRoot := filepath.Join(tempParent, "source")
	if err := MaterializeSavePointPayload(repoRoot, source.SourceSnapshotID, sourceRoot); err != nil {
		return err
	}
	if err := pathutil.ValidateNoSymlinkParents(sourceRoot, source.SourcePath); err != nil {
		return fmt.Errorf("path source parent containment for %s: %w", source.SourcePath, err)
	}
	sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(source.SourcePath))
	if _, err := os.Lstat(sourcePath); err != nil {
		return fmt.Errorf("path source %s: %w", source.SourcePath, err)
	}
	if err := pathutil.ValidateNoSymlinkParents(expectedRoot, source.TargetPath); err != nil {
		return fmt.Errorf("target path parent containment for %s: %w", source.TargetPath, err)
	}
	targetPath := filepath.Join(expectedRoot, filepath.FromSlash(source.TargetPath))
	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("replace expected path %s: %w", source.TargetPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("create expected path parent %s: %w", source.TargetPath, err)
	}
	_, err = engine.CloneToNew(engine.NewCopyEngine(), sourcePath, targetPath)
	if err != nil {
		return fmt.Errorf("copy expected path source %s: %w", source.TargetPath, err)
	}
	return nil
}

func ManagedPathEqual(actualRoot, expectedRoot, rel string, excluded func(string) bool) (bool, error) {
	if rel != "" {
		if excluded != nil && excluded(rel) {
			return true, nil
		}
		if err := pathutil.ValidateNoSymlinkParents(actualRoot, rel); err != nil {
			return false, err
		}
		if err := pathutil.ValidateNoSymlinkParents(expectedRoot, rel); err != nil {
			return false, err
		}
	}
	return compareManagedEntry(actualRoot, expectedRoot, filepath.FromSlash(rel), excluded)
}

func compareManagedEntry(actualRoot, expectedRoot, rel string, excluded func(string) bool) (bool, error) {
	slashRel := filepath.ToSlash(rel)
	if slashRel != "." && slashRel != "" && excluded != nil && excluded(slashRel) {
		return true, nil
	}
	actualPath := actualRoot
	expectedPath := expectedRoot
	if rel != "" {
		actualPath = filepath.Join(actualRoot, rel)
		expectedPath = filepath.Join(expectedRoot, rel)
	}

	actualInfo, actualErr := os.Lstat(actualPath)
	expectedInfo, expectedErr := os.Lstat(expectedPath)
	if os.IsNotExist(actualErr) && os.IsNotExist(expectedErr) {
		return true, nil
	}
	if os.IsNotExist(actualErr) || os.IsNotExist(expectedErr) {
		return false, nil
	}
	if actualErr != nil {
		return false, actualErr
	}
	if expectedErr != nil {
		return false, expectedErr
	}
	if actualInfo.Mode().Type() != expectedInfo.Mode().Type() {
		return false, nil
	}
	if rel != "" && actualInfo.Mode().Perm() != expectedInfo.Mode().Perm() {
		return false, nil
	}
	if actualInfo.Mode()&os.ModeSymlink != 0 {
		actualTarget, err := os.Readlink(actualPath)
		if err != nil {
			return false, err
		}
		expectedTarget, err := os.Readlink(expectedPath)
		if err != nil {
			return false, err
		}
		return actualTarget == expectedTarget, nil
	}
	if actualInfo.IsDir() {
		return compareManagedDirs(actualRoot, expectedRoot, rel, excluded)
	}
	if actualInfo.Size() != expectedInfo.Size() {
		return false, nil
	}
	return regularFilesEqual(actualPath, expectedPath)
}

func compareManagedDirs(actualRoot, expectedRoot, rel string, excluded func(string) bool) (bool, error) {
	actualEntries, err := comparableEntryNames(filepath.Join(actualRoot, rel), rel, excluded)
	if err != nil {
		return false, err
	}
	expectedEntries, err := comparableEntryNames(filepath.Join(expectedRoot, rel), rel, excluded)
	if err != nil {
		return false, err
	}
	if len(actualEntries) != len(expectedEntries) {
		return false, nil
	}
	for i := range actualEntries {
		if actualEntries[i] != expectedEntries[i] {
			return false, nil
		}
		childRel := filepath.Join(rel, actualEntries[i])
		matches, err := compareManagedEntry(actualRoot, expectedRoot, childRel, excluded)
		if err != nil || !matches {
			return matches, err
		}
	}
	return true, nil
}

func comparableEntryNames(dir, rel string, excluded func(string) bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		entryRel := filepath.ToSlash(filepath.Join(rel, entry.Name()))
		if excluded != nil && excluded(entryRel) {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func regularFilesEqual(left, right string) (bool, error) {
	leftFile, err := os.Open(left)
	if err != nil {
		return false, err
	}
	defer leftFile.Close()
	rightFile, err := os.Open(right)
	if err != nil {
		return false, err
	}
	defer rightFile.Close()

	leftBytes, err := io.ReadAll(leftFile)
	if err != nil {
		return false, err
	}
	rightBytes, err := io.ReadAll(rightFile)
	if err != nil {
		return false, err
	}
	if len(leftBytes) != len(rightBytes) {
		return false, nil
	}
	for i := range leftBytes {
		if leftBytes[i] != rightBytes[i] {
			return false, nil
		}
	}
	return true, nil
}
