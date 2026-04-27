package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

var viewCmd = &cobra.Command{
	Use:   "view <save-point> [path]",
	Short: "Open a read-only view of a save point",
	Long: `Open a read-only view of a save point, or a path inside it.

The real folder, workspace, and history are not changed. The save point must be
given as a full save point ID or a unique ID prefix.

Examples:
  jvs view 1771589abc
  jvs view 1771589abc src/config.json`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}

		savePointID, err := resolvePublicSavePointID(r.Root, args[0])
		if err != nil {
			return viewPointError(err)
		}

		pathInside := ""
		if len(args) > 1 {
			pathInside, err = normalizeViewPath(args[1])
			if err != nil {
				return viewPointError(err)
			}
		}

		result, err := openReadOnlySavePointView(r.Root, workspaceName, savePointID, pathInside)
		if err != nil {
			return viewPointError(err)
		}
		if jsonOutput {
			return outputJSON(result)
		}

		printViewResult(result)
		return nil
	},
}

type publicViewResult struct {
	Folder                      string `json:"folder"`
	Workspace                   string `json:"workspace"`
	SavePoint                   string `json:"save_point"`
	PathInsideSavePoint         string `json:"path_inside_save_point,omitempty"`
	ViewID                      string `json:"view_id"`
	ViewPath                    string `json:"view_path"`
	ReadOnly                    bool   `json:"read_only"`
	NoWorkspaceOrHistoryChanged bool   `json:"no_workspace_or_history_changed"`
}

func resolvePublicSavePointID(repoRoot, raw string) (model.SnapshotID, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return "", fmt.Errorf("save point ID is required. Choose a save point ID, then run the command again")
	}
	switch ref {
	case "current", "latest", "dirty":
		return "", fmt.Errorf("%q is not a save point ID. Choose a save point ID, then run the command again", ref)
	}

	if id := model.SnapshotID(ref); id.IsValid() {
		if _, err := snapshot.LoadDescriptor(repoRoot, id); err != nil {
			return "", fmt.Errorf("save point %s is not available: %w", id, err)
		}
		return id, nil
	}

	entries, err := snapshot.ListCatalogEntries(repoRoot)
	if err != nil {
		return "", err
	}
	var matches []snapshot.CatalogEntry
	for _, entry := range entries {
		if strings.HasPrefix(string(entry.SnapshotID), ref) {
			matches = append(matches, entry)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%q is not a save point ID. Choose a save point ID, then run the command again", ref)
	case 1:
		if matches[0].DescriptorErr != nil {
			return "", fmt.Errorf("save point %s is not available: %w", matches[0].SnapshotID, matches[0].DescriptorErr)
		}
		return matches[0].SnapshotID, nil
	default:
		return "", fmt.Errorf("%q matches multiple save points. Choose a full save point ID, then run the command again", ref)
	}
}

func normalizeViewPath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("path inside save point must be a workspace-relative path")
	}
	if strings.ContainsRune(raw, 0) || filepath.IsAbs(raw) || looksLikeWindowsPath(raw) {
		return "", fmt.Errorf("path inside save point must be a workspace-relative path")
	}
	clean := filepath.Clean(raw)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path inside save point must be a workspace-relative path")
	}
	if clean == repo.JVSDirName || strings.HasPrefix(clean, repo.JVSDirName+string(filepath.Separator)) {
		return "", fmt.Errorf("path inside save point must be a workspace-relative path; JVS control data is not managed")
	}
	return filepath.ToSlash(clean), nil
}

func looksLikeWindowsPath(path string) bool {
	if strings.HasPrefix(path, `\\`) || strings.Contains(path, `\`) {
		return true
	}
	if len(path) >= 2 && path[1] == ':' {
		first := path[0]
		return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')
	}
	return false
}

func openReadOnlySavePointView(repoRoot, workspaceName string, savePointID model.SnapshotID, pathInside string) (result publicViewResult, err error) {
	status, err := buildWorkspaceStatus(repoRoot, workspaceName)
	if err != nil {
		return publicViewResult{}, err
	}

	state, issue := snapshot.InspectPublishState(repoRoot, savePointID, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})
	if issue != nil {
		return publicViewResult{}, snapshot.PublishStateIssueError(issue)
	}

	viewID := "view-" + string(model.NewSnapshotID())
	viewRoot := filepath.Join(repoRoot, repo.JVSDirName, "views", viewID)
	payloadRoot := filepath.Join(viewRoot, "payload")
	if err := prepareViewRoot(viewRoot); err != nil {
		return publicViewResult{}, err
	}
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			if cleanupErr := removeFailedViewRoot(viewRoot); cleanupErr != nil {
				if err != nil {
					err = fmt.Errorf("%w; additionally failed to clean view: %v", err, cleanupErr)
				} else {
					err = fmt.Errorf("failed to clean view: %w", cleanupErr)
				}
			}
		}
	}()

	opts := snapshotpayload.OptionsFromDescriptor(state.Descriptor)
	if err := snapshotpayload.MaterializeToNew(state.SnapshotDir, payloadRoot, opts, func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	}); err != nil {
		return publicViewResult{}, err
	}

	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return publicViewResult{}, err
	}
	if err := repo.ValidateManagedPayloadOnly(boundary, payloadRoot); err != nil {
		return publicViewResult{}, err
	}
	if err := validateViewPayloadNoSymlinks(payloadRoot); err != nil {
		return publicViewResult{}, err
	}

	viewPath := payloadRoot
	if pathInside != "" {
		viewPath = filepath.Join(payloadRoot, filepath.FromSlash(pathInside))
		if err := validateViewPath(payloadRoot, viewPath); err != nil {
			return publicViewResult{}, err
		}
	}
	if err := makeReadOnly(payloadRoot); err != nil {
		return publicViewResult{}, err
	}
	cleanupOnError = false

	return publicViewResult{
		Folder:                      status.Folder,
		Workspace:                   status.Workspace,
		SavePoint:                   string(savePointID),
		PathInsideSavePoint:         pathInside,
		ViewID:                      viewID,
		ViewPath:                    viewPath,
		ReadOnly:                    true,
		NoWorkspaceOrHistoryChanged: true,
	}, nil
}

func removeFailedViewRoot(viewRoot string) error {
	if _, err := os.Lstat(viewRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat view: %w", err)
	}
	if err := restoreWriteBits(viewRoot); err != nil {
		return fmt.Errorf("restore write permissions: %w", err)
	}
	if err := os.RemoveAll(viewRoot); err != nil {
		return fmt.Errorf("remove view: %w", err)
	}
	return nil
}

func restoreWriteBits(root string) error {
	var errs []error
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			errs = append(errs, fmt.Errorf("walk %s: %w", path, walkErr))
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			errs = append(errs, fmt.Errorf("stat %s: %w", path, err))
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		mode := info.Mode().Perm()
		if info.IsDir() {
			mode |= 0700
		} else {
			mode |= 0600
		}
		if err := os.Chmod(path, mode); err != nil {
			errs = append(errs, fmt.Errorf("chmod %s: %w", path, err))
		}
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func prepareViewRoot(viewRoot string) error {
	controlDir := filepath.Dir(filepath.Dir(viewRoot))
	info, err := os.Lstat(controlDir)
	if err != nil {
		return fmt.Errorf("stat JVS control data: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("JVS control data is not a real directory")
	}
	if err := os.MkdirAll(filepath.Dir(viewRoot), 0700); err != nil {
		return fmt.Errorf("create view area: %w", err)
	}
	if err := os.Mkdir(viewRoot, 0700); err != nil {
		return fmt.Errorf("create view: %w", err)
	}
	return nil
}

func validateViewPath(root, path string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("view root: %w", err)
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("view path: %w", err)
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return fmt.Errorf("view path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path inside save point must be a workspace-relative path")
	}
	if err := pathutil.ValidateNoSymlinkParents(rootAbs, rel); err != nil {
		return fmt.Errorf("path inside save point must not traverse symlinks: %w", err)
	}
	info, err := os.Lstat(pathAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path inside save point does not exist: %s", filepath.ToSlash(rel))
		}
		return fmt.Errorf("stat path inside save point: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		rootReal, err := filepath.EvalSymlinks(rootAbs)
		if err != nil {
			return fmt.Errorf("view root: %w", err)
		}
		pathReal, err := filepath.EvalSymlinks(pathAbs)
		if err != nil {
			return fmt.Errorf("view path symlink: %w", err)
		}
		realRel, err := filepath.Rel(rootReal, pathReal)
		if err != nil {
			return fmt.Errorf("view path symlink: %w", err)
		}
		if realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("path inside save point must stay inside the save point")
		}
	}
	return nil
}

func validateViewPayloadNoSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk view payload: %w", err)
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return fmt.Errorf("view payload symlink: %w", relErr)
			}
			return fmt.Errorf("save point contains a symlink and cannot be opened as a read-only view: %s", filepath.ToSlash(rel))
		}
		return nil
	})
}

func makeReadOnly(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		mode := info.Mode().Perm() &^ 0222
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("make view read-only: %w", err)
		}
		return nil
	})
}

func printViewResult(result publicViewResult) {
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	fmt.Println("Opened read-only view.")
	fmt.Printf("Save point: %s\n", color.SnapshotID(result.SavePoint))
	if result.PathInsideSavePoint != "" {
		fmt.Printf("Path inside save point: %s\n", result.PathInsideSavePoint)
	}
	fmt.Printf("View: %s\n", result.ViewID)
	fmt.Printf("View path: %s\n", result.ViewPath)
	fmt.Println("No workspace or history changed.")
}

func viewPointError(err error) error {
	if err == nil {
		return nil
	}
	message := viewPointVocabulary(err.Error())
	if !strings.Contains(message, "No files or history changed.") {
		message += ". No files or history changed."
	}
	return fmt.Errorf("%s", message)
}

func viewPointVocabulary(value string) string {
	replacer := strings.NewReplacer(
		"checkpoints", "save points",
		"checkpoint", "save point",
		"snapshots", "save points",
		"snapshot", "save point",
		"worktrees", "workspaces",
		"worktree", "workspace",
		"current", "source",
		"latest", "newest",
		"HEAD", "source",
		"head", "source",
		"dirty", "unsaved",
		"fork", "copy",
		"commit", "save",
	)
	return replacer.Replace(value)
}

func init() {
	rootCmd.AddCommand(viewCmd)
}
