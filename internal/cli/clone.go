package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/engine"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/pkg/fsutil"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/jvs-project/jvs/pkg/uuidutil"
)

var cloneScope string

const fullCloneCopyMode = "full-repository"

var cloneCmd = &cobra.Command{
	Use:   "clone <source-repo> <dest-repo>",
	Short: "Clone a local JVS repository",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch cloneScope {
		case "full":
			return runCloneFull(args[0], args[1])
		case "current":
			return runCloneCurrent(args[0], args[1])
		default:
			return fmt.Errorf("invalid clone scope %q: expected full or current", cloneScope)
		}
	},
}

func runCloneFull(sourceArg, destArg string) error {
	sourceRoot, err := discoverRepoRoot(sourceArg)
	if err != nil {
		return fmt.Errorf("discover source repository: %w", err)
	}
	destRoot, err := repo.ValidateInitTarget(destArg)
	if err != nil {
		return fmt.Errorf("validate destination repository: %w", err)
	}
	if err := rejectDangerousOverlap("source repository", sourceRoot, "destination repository", destRoot); err != nil {
		return err
	}

	var transferResult *engine.CloneResult
	var transferEngine model.EngineType
	if err := repo.WithMutationLock(sourceRoot, "clone full", func() error {
		var err error
		transferResult, transferEngine, err = cloneFullRepository(sourceRoot, destRoot)
		return err
	}); err != nil {
		return fmt.Errorf("copy repository: %w", err)
	}
	newRepoID, err := rewriteRepoID(destRoot)
	if err != nil {
		return fmt.Errorf("write cloned repository identity: %w", err)
	}
	capabilities, err := engine.ProbeCapabilities(destRoot, true)
	if err != nil {
		return fmt.Errorf("probe destination capabilities: %w", err)
	}
	warnings := appendUniqueStrings(
		[]string{"full clone uses byte copy; runtime lock and intent state are excluded"},
		capabilities.Warnings...,
	)
	degraded := degradedReasons(transferResult)
	effectiveEngine := model.EngineType(effectiveTransferMode(transferEngine, transferResult))

	output := map[string]any{
		"scope":                  "full",
		"requested_scope":        "full",
		"source_repo":            sourceRoot,
		"provenance":             map[string]any{"source_repo": sourceRoot, "scope": "full"},
		"repo_root":              destRoot,
		"repo_id":                newRepoID,
		"copy_mode":              fullCloneCopyMode,
		"requested_engine":       model.EngineCopy,
		"transfer_engine":        transferEngine,
		"transfer_mode":          string(effectiveEngine),
		"optimized_transfer":     false,
		"runtime_state_excluded": true,
		"degraded_reasons":       stableStringSlice(degraded),
	}
	applySetupJSONFields(output, capabilities, effectiveEngine, warnings)
	if jsonOutput {
		return outputJSON(output)
	}

	fmt.Printf("Cloned JVS repository\n")
	fmt.Printf("  Scope: full\n")
	fmt.Printf("  Source repo: %s\n", sourceRoot)
	fmt.Printf("  Repo root: %s\n", destRoot)
	fmt.Printf("  Repo ID: %s\n", newRepoID)
	fmt.Printf("  Copy mode: %s\n", fullCloneCopyMode)
	fmt.Printf("  Requested engine: %s\n", model.EngineCopy)
	fmt.Printf("  Transfer engine: %s\n", transferEngine)
	fmt.Printf("  Effective engine: %s\n", output["effective_engine"])
	fmt.Printf("  Optimized transfer: false\n")
	fmt.Printf("  Runtime state excluded: true\n")
	for _, reason := range degraded {
		fmt.Printf("  Degraded: %s\n", reason)
	}
	for _, warning := range warnings {
		fmt.Printf("  Warning: %s\n", warning)
	}
	return nil
}

func runCloneCurrent(sourceArg, destArg string) error {
	sourceRepo, sourceWorktree, sourceWorkspace, err := discoverSourceWorkspace(sourceArg)
	if err != nil {
		return fmt.Errorf("discover source workspace: %w", err)
	}
	if err := rejectContainsJVS(sourceWorkspace); err != nil {
		return err
	}
	destRoot, err := repo.ValidateInitTarget(destArg)
	if err != nil {
		return fmt.Errorf("validate destination repository: %w", err)
	}
	if err := rejectDangerousOverlap("source repository", sourceRepo.Root, "destination repository", destRoot); err != nil {
		return err
	}

	var r *repo.Repo
	var mainWorkspace string
	var transferPlan *engine.TransferPlan
	if err := repo.WithMutationLock(sourceRepo.Root, "clone current", func() error {
		var err error
		r, err = repo.InitTarget(destRoot)
		if err != nil {
			return fmt.Errorf("initialize destination repository: %w", err)
		}
		mainWorkspace = filepath.Join(r.Root, "main")

		transferPlan, err = planTransfer(sourceWorkspace, mainWorkspace, r.Root)
		if err != nil {
			return fmt.Errorf("plan current workspace transfer: %w", err)
		}
		if _, err := cloneDirectory(sourceWorkspace, mainWorkspace, transferPlan); err != nil {
			return fmt.Errorf("copy current workspace: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	note := fmt.Sprintf("initial clone from %s workspace %s", sourceRepo.Root, sourceWorktree)
	desc, err := createInitialCheckpoint(r.Root, note, []string{"clone"})
	if err != nil {
		return fmt.Errorf("create initial checkpoint: %w", err)
	}

	output := map[string]any{
		"scope":              "current",
		"requested_scope":    "current",
		"source_workspace":   sourceWorktree,
		"repo_root":          r.Root,
		"repo_id":            r.RepoID,
		"main_workspace":     mainWorkspace,
		"provenance":         map[string]any{"source_repo": sourceRepo.Root, "source_workspace": sourceWorktree, "scope": "current"},
		"initial_checkpoint": desc.SnapshotID,
		"engine":             desc.Engine,
	}
	applyTransferJSONFields(output, transferPlan)
	if jsonOutput {
		return outputJSON(output)
	}

	fmt.Printf("Cloned current workspace into new JVS repository\n")
	fmt.Printf("  Scope: current\n")
	fmt.Printf("  Source repo: %s\n", sourceRepo.Root)
	fmt.Printf("  Source workspace: %s\n", sourceWorktree)
	fmt.Printf("  Repo root: %s\n", r.Root)
	fmt.Printf("  Main workspace: %s\n", mainWorkspace)
	fmt.Printf("  Initial checkpoint: %s\n", desc.SnapshotID)
	fmt.Printf("  Engine: %s\n", desc.Engine)
	fmt.Printf("  Requested engine: %s\n", transferPlan.RequestedEngine)
	fmt.Printf("  Transfer engine: %s\n", transferPlan.TransferEngine)
	fmt.Printf("  Effective engine: %s\n", transferPlan.EffectiveEngine)
	fmt.Printf("  Optimized transfer: %t\n", transferPlan.OptimizedTransfer)
	for _, reason := range transferPlan.DegradedReasons {
		fmt.Printf("  Degraded: %s\n", reason)
	}
	for _, warning := range transferPlan.Warnings {
		fmt.Printf("  Warning: %s\n", warning)
	}
	return nil
}

func discoverRepoRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	r, err := repo.Discover(abs)
	if err != nil {
		return "", err
	}
	return r.Root, nil
}

func discoverSourceWorkspace(path string) (*repo.Repo, string, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("resolve path: %w", err)
	}
	r, wtName, err := repo.DiscoverWorktree(abs)
	if err == nil && wtName != "" {
		workspace, err := repo.WorktreePayloadPath(r.Root, wtName)
		if err != nil {
			return nil, "", "", err
		}
		return r, wtName, workspace, nil
	}

	r, err = repo.Discover(abs)
	if err != nil {
		return nil, "", "", err
	}
	workspace, err := repo.WorktreePayloadPath(r.Root, "main")
	if err != nil {
		return nil, "", "", err
	}
	return r, "main", workspace, nil
}

func cloneFullRepository(sourceRoot, destRoot string) (*engine.CloneResult, model.EngineType, error) {
	result, err := copyFullRepositoryTree(sourceRoot, destRoot)
	if err != nil {
		return nil, model.EngineCopy, err
	}
	if err := rebuildFullCloneRuntimeState(destRoot); err != nil {
		return nil, model.EngineCopy, err
	}
	return result, model.EngineCopy, nil
}

func copyFullRepositoryTree(sourceRoot, destRoot string) (*engine.CloneResult, error) {
	result := &engine.CloneResult{}
	var dirs []cloneDirMode

	err := filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		if rel != "." && fullCloneRuntimeStatePath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat source entry %s: %w", path, err)
		}
		dstPath := filepath.Join(destRoot, rel)

		switch {
		case info.IsDir():
			if err := copyCloneDir(dstPath, info); err != nil {
				return err
			}
			dirs = append(dirs, cloneDirMode{path: dstPath, mode: info.Mode().Perm()})
			return nil
		case info.Mode()&os.ModeSymlink != 0:
			return copyCloneSymlink(path, dstPath)
		default:
			return copyCloneFile(path, dstPath, info)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("copy: %w", err)
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i].path, dirs[i].mode); err != nil {
			return nil, fmt.Errorf("copy: chmod dir %s: %w", dirs[i].path, err)
		}
	}
	if err := fsutil.FsyncDir(destRoot); err != nil {
		return nil, fmt.Errorf("fsync destination repository: %w", err)
	}
	return result, nil
}

func fullCloneRuntimeStatePath(rel string) bool {
	rel = filepath.ToSlash(rel)
	return rel == ".jvs/locks" ||
		strings.HasPrefix(rel, ".jvs/locks/") ||
		rel == ".jvs/intents" ||
		strings.HasPrefix(rel, ".jvs/intents/")
}

func rebuildFullCloneRuntimeState(repoRoot string) error {
	for _, rel := range []string{
		filepath.Join(repo.JVSDirName, "locks"),
		filepath.Join(repo.JVSDirName, "intents"),
	} {
		path := filepath.Join(repoRoot, rel)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", path, err)
		}
		if err := fsutil.FsyncDir(path); err != nil {
			return fmt.Errorf("fsync runtime directory %s: %w", path, err)
		}
	}
	return fsutil.FsyncDir(filepath.Join(repoRoot, repo.JVSDirName))
}

func copyCloneDir(dst string, info fs.FileInfo) error {
	if err := os.MkdirAll(dst, info.Mode().Perm()|0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dst, err)
	}
	return nil
}

func copyCloneSymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("readlink %s: %w", src, err)
	}
	if err := os.Symlink(target, dst); err != nil {
		return fmt.Errorf("symlink %s: %w", dst, err)
	}
	return nil
}

func copyCloneFile(src, dst string, info fs.FileInfo) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src %s: %w", src, err)
	}
	defer srcFile.Close()

	mode := info.Mode().Perm()
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create dst %s: %w", dst, err)
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close()
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	if err := dstFile.Sync(); err != nil {
		_ = dstFile.Close()
		return fmt.Errorf("sync %s: %w", dst, err)
	}
	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dst, err)
	}
	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

type cloneDirMode struct {
	path string
	mode os.FileMode
}

func rewriteRepoID(repoRoot string) (string, error) {
	newID := uuidutil.NewV4()
	path := filepath.Join(repoRoot, repo.JVSDirName, repo.RepoIDFile)
	if err := os.WriteFile(path, []byte(newID+"\n"), 0600); err != nil {
		return "", err
	}
	return newID, nil
}

func init() {
	cloneCmd.Flags().StringVar(&cloneScope, "scope", "full", "clone scope: full or current")
	rootCmd.AddCommand(cloneCmd)
}
