package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

var viewCapacityGate = capacitygate.Default()
var viewTransferPlanner transfer.EnginePlanner = engine.TransferPlanner{}

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
		ctx, err := resolveWorkspaceScoped()
		if err != nil {
			return err
		}

		savePointID, err := resolvePublicSavePointID(ctx.Repo.Root, args[0])
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

		result, err := openReadOnlySavePointView(ctx.Repo.Root, ctx.Workspace, savePointID, pathInside, ctx.Separated)
		if err != nil {
			return viewPointError(err)
		}
		if jsonOutput {
			return outputJSONWithSeparatedControl(result, ctx.Separated, separatedDoctorStrictNotRun)
		}

		printViewResult(result)
		return nil
	},
}

var viewCloseCmd = &cobra.Command{
	Use:   "close <view-id>",
	Short: "Close a read-only view",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("read-only view ID is required")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		result, err := closeReadOnlySavePointView(ctx.Repo.Root, args[0])
		if err != nil {
			return viewCloseError(err)
		}
		if jsonOutput {
			return outputJSONWithSeparatedControl(result, ctx.Separated, separatedDoctorStrictNotRun)
		}
		printViewCloseResult(result)
		return nil
	},
}

type publicViewResult struct {
	transfer.Data
	Folder                      string `json:"folder"`
	Workspace                   string `json:"workspace"`
	SavePoint                   string `json:"save_point"`
	PathInsideSavePoint         string `json:"path_inside_save_point,omitempty"`
	ViewID                      string `json:"view_id"`
	ViewPath                    string `json:"view_path"`
	ReadOnly                    bool   `json:"read_only"`
	NoWorkspaceOrHistoryChanged bool   `json:"no_workspace_or_history_changed"`
}

type publicViewCloseResult struct {
	Mode                        string  `json:"mode"`
	Status                      string  `json:"status"`
	ViewID                      string  `json:"view_id"`
	SavePoint                   *string `json:"save_point"`
	ViewPath                    string  `json:"view_path"`
	ViewPathRemoved             bool    `json:"view_path_removed"`
	NoWorkspaceOrHistoryChanged bool    `json:"no_workspace_or_history_changed"`
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

func openReadOnlySavePointView(repoRoot, workspaceName string, savePointID model.SnapshotID, pathInside string, separated *repo.SeparatedContext) (result publicViewResult, err error) {
	folder, err := workspaceFolder(repoRoot, workspaceName)
	if err != nil {
		return publicViewResult{}, err
	}
	if err := preflightSeparatedViewOpen(separated); err != nil {
		return publicViewResult{}, err
	}

	viewID := "view-" + string(model.NewSnapshotID())
	pinHandle, err := sourcepin.NewManager(repoRoot).CreateWithID(savePointID, viewID, "active read-only view")
	if err != nil {
		return publicViewResult{}, err
	}
	releasePinOnError := true
	defer func() {
		if releasePinOnError {
			if releaseErr := pinHandle.Release(); releaseErr != nil {
				if err != nil {
					err = fmt.Errorf("%w; additionally failed to release read-only view protection: %v", err, releaseErr)
				} else {
					err = fmt.Errorf("failed to release read-only view protection: %w", releaseErr)
				}
			}
		}
	}()

	state, issue := snapshot.InspectPublishState(repoRoot, savePointID, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        false,
	})
	if issue != nil {
		return publicViewResult{}, snapshot.PublishStateIssueError(issue)
	}

	viewRoot := filepath.Join(repoRoot, repo.JVSDirName, "views", viewID)
	payloadRoot := filepath.Join(viewRoot, "payload")
	sourceEstimate, err := snapshotpayload.EstimateMaterializationCapacity(state.SnapshotDir, snapshotpayload.OptionsFromDescriptor(state.Descriptor))
	if err != nil {
		return publicViewResult{}, err
	}
	if _, err := viewCapacityGate.Check(capacitygate.Request{
		Operation:       "view",
		Folder:          folder,
		Workspace:       workspaceName,
		SourceSavePoint: string(savePointID),
		Path:            pathInside,
		Components: []capacitygate.Component{
			{Name: "payload hash", Path: filepath.Join(os.TempDir(), "jvs-payload-hash-probe"), Bytes: sourceEstimate.PeakBytes},
			{Name: "view payload", Path: payloadRoot, Bytes: sourceEstimate.PeakBytes},
			{Name: "view metadata", Path: viewRoot, Bytes: metadataFloor},
		},
		FailureMessages: []string{"No view was opened.", "No files or history changed."},
	}); err != nil {
		return publicViewResult{}, err
	}
	state, issue = snapshot.InspectPublishState(repoRoot, savePointID, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})
	if issue != nil {
		return publicViewResult{}, snapshot.PublishStateIssueError(issue)
	}
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
	viewPath := payloadRoot
	if pathInside != "" {
		viewPath = filepath.Join(payloadRoot, filepath.FromSlash(pathInside))
	}
	intent := viewPrimaryTransferIntent(repoRoot, savePointID, state.SnapshotDir, payloadRoot, viewPath)
	plan, err := transfer.PlanIntent(viewTransferPlanner, intent)
	if err != nil {
		return publicViewResult{}, fmt.Errorf("plan view transfer: %w", err)
	}
	var runtimeResult *engine.CloneResult
	if err := snapshotpayload.MaterializeToNew(state.SnapshotDir, payloadRoot, opts, func(src, dst string) error {
		result, err := engine.CloneToNew(engine.NewEngine(plan.TransferEngine), src, dst)
		if result != nil {
			runtimeResult = result
		}
		return err
	}); err != nil {
		return publicViewResult{}, err
	}
	transferRecord := transfer.RecordFromPlanAndRuntime(intent, plan, runtimeResult)

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

	if pathInside != "" {
		if err := validateViewPath(payloadRoot, viewPath); err != nil {
			return publicViewResult{}, err
		}
	}
	if err := makeReadOnly(payloadRoot); err != nil {
		return publicViewResult{}, err
	}
	cleanupOnError = false
	releasePinOnError = false

	return publicViewResult{
		Data:                        transferDataFromRecord(&transferRecord),
		Folder:                      folder,
		Workspace:                   workspaceName,
		SavePoint:                   string(savePointID),
		PathInsideSavePoint:         pathInside,
		ViewID:                      viewID,
		ViewPath:                    viewPath,
		ReadOnly:                    true,
		NoWorkspaceOrHistoryChanged: true,
	}, nil
}

func preflightSeparatedViewOpen(ctx *repo.SeparatedContext) error {
	if ctx == nil {
		return nil
	}
	revalidated, err := revalidateSeparatedContext(ctx, ctx.PayloadRoot)
	if err != nil {
		return err
	}
	if err := validateSeparatedViewRuntimeBoundary(revalidated.ControlRoot); err != nil {
		return err
	}
	if _, err := repo.WorktreeManagedPayloadBoundary(revalidated.ControlRoot, revalidated.Workspace); err != nil {
		return err
	}
	return repo.ValidateSeparatedPayloadSymlinkBoundary(revalidated)
}

func validateSeparatedViewRuntimeBoundary(controlRoot string) error {
	cleanControlRoot := filepath.Clean(controlRoot)
	controlInfo, err := os.Lstat(cleanControlRoot)
	if err != nil {
		return errclass.ErrPathBoundaryEscape.WithMessagef("stat control root boundary: %v", err)
	}
	if controlInfo.Mode()&os.ModeSymlink != 0 || !controlInfo.IsDir() {
		return errclass.ErrPathBoundaryEscape.WithMessagef("control root is not a real directory: %s", cleanControlRoot)
	}
	controlPhysical, err := filepath.EvalSymlinks(cleanControlRoot)
	if err != nil {
		return errclass.ErrPathBoundaryEscape.WithMessagef("resolve control root boundary: %v", err)
	}

	jvsDir := filepath.Join(cleanControlRoot, repo.JVSDirName)
	viewsDir := filepath.Join(jvsDir, "views")
	for _, path := range []string{jvsDir, viewsDir} {
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) && path == viewsDir {
				return nil
			}
			return errclass.ErrPathBoundaryEscape.WithMessagef("stat control runtime boundary: %v", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errclass.ErrPathBoundaryEscape.WithMessagef("control runtime path is not a real directory: %s", path)
		}
		physical, err := filepath.EvalSymlinks(path)
		if err != nil {
			return errclass.ErrPathBoundaryEscape.WithMessagef("resolve control runtime boundary: %v", err)
		}
		if !viewPathContained(controlPhysical, physical) {
			return errclass.ErrPathBoundaryEscape.WithMessagef("control runtime path escapes control root: %s", path)
		}
	}
	return nil
}

func viewPathContained(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}

func viewPrimaryTransferIntent(repoRoot string, savePointID model.SnapshotID, sourcePath, payloadRoot, viewPath string) transfer.Intent {
	return transfer.Intent{
		TransferID:                 "view-primary",
		Operation:                  "view",
		Phase:                      "view_materialization",
		Primary:                    true,
		ResultKind:                 transfer.ResultKindFinal,
		PermissionScope:            transfer.PermissionScopeExecution,
		SourceRole:                 "save_point_payload",
		SourcePath:                 sourcePath,
		DestinationRole:            "view_directory",
		MaterializationDestination: payloadRoot,
		CapabilityProbePath:        filepath.Dir(payloadRoot),
		PublishedDestination:       viewPath,
		RequestedEngine:            requestedTransferEngine(repoRoot),
	}
}

func closeReadOnlySavePointView(repoRoot, rawViewID string) (publicViewCloseResult, error) {
	viewID, err := normalizeViewID(rawViewID)
	if err != nil {
		return publicViewCloseResult{}, err
	}
	viewRoot := filepath.Join(repoRoot, repo.JVSDirName, "views", viewID)
	viewPath := filepath.Join(viewRoot, "payload")

	pin, pinPresent, err := readViewPin(repoRoot, viewID)
	if err != nil {
		return publicViewCloseResult{}, err
	}
	viewPathRemoved, viewPresent, err := removeViewRootForClose(viewRoot)
	if err != nil {
		return publicViewCloseResult{}, err
	}
	if pinPresent {
		if err := sourcepin.NewManager(repoRoot).RemoveIfMatches(*pin); err != nil {
			return publicViewCloseResult{}, fmt.Errorf("read-only view folder was removed, but the view close record could not be cleared safely")
		}
	}

	status := "closed"
	if !pinPresent && !viewPresent {
		status = "already_closed"
	}
	var savePoint *string
	if pin != nil {
		value := string(pin.SnapshotID)
		savePoint = &value
	}
	return publicViewCloseResult{
		Mode:                        "close",
		Status:                      status,
		ViewID:                      viewID,
		SavePoint:                   savePoint,
		ViewPath:                    viewPath,
		ViewPathRemoved:             viewPathRemoved,
		NoWorkspaceOrHistoryChanged: true,
	}, nil
}

func normalizeViewID(raw string) (string, error) {
	viewID := strings.TrimSpace(raw)
	if viewID == "" || viewID != raw {
		return "", fmt.Errorf("read-only view ID must be a safe view ID")
	}
	if !strings.HasPrefix(viewID, "view-") {
		return "", fmt.Errorf("read-only view ID must be a safe view ID")
	}
	if err := pathutil.ValidateName(viewID); err != nil {
		return "", fmt.Errorf("read-only view ID must be a safe view ID")
	}
	return viewID, nil
}

func readViewPin(repoRoot, viewID string) (*model.Pin, bool, error) {
	pin, err := sourcepin.NewManager(repoRoot).Read(viewID)
	if err == nil {
		if !isActiveReadOnlyViewPin(viewID, pin) {
			return nil, false, fmt.Errorf("read-only view could not be confirmed safely")
		}
		return pin, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("read-only view could not be confirmed safely")
}

func isActiveReadOnlyViewPin(viewID string, pin *model.Pin) bool {
	return pin != nil &&
		pin.PinID == viewID &&
		strings.HasPrefix(viewID, "view-") &&
		pin.Reason == "active read-only view"
}

func removeViewRootForClose(viewRoot string) (removed bool, present bool, err error) {
	info, err := os.Lstat(viewRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return true, false, nil
		}
		return false, false, fmt.Errorf("read-only view folder could not be checked")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, true, fmt.Errorf("read-only view folder is not safe to close")
	}
	if err := restoreWriteBits(viewRoot); err != nil {
		return false, true, fmt.Errorf("read-only view folder could not be prepared for removal")
	}
	if err := os.RemoveAll(viewRoot); err != nil {
		return false, true, fmt.Errorf("read-only view folder could not be removed")
	}
	if _, err := os.Lstat(viewRoot); err == nil {
		return false, true, fmt.Errorf("read-only view folder could not be removed")
	} else if !os.IsNotExist(err) {
		return false, true, fmt.Errorf("read-only view folder could not be checked")
	}
	return true, true, nil
}

func workspaceFolder(repoRoot, workspaceName string) (string, error) {
	mgr := worktree.NewManager(repoRoot)
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return "", fmt.Errorf("workspace folder: %w", err)
	}
	return folder, nil
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
	viewsDir := filepath.Dir(viewRoot)
	viewsInfo, err := os.Lstat(viewsDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat view area: %w", err)
		}
		if err := os.Mkdir(viewsDir, 0700); err != nil {
			return fmt.Errorf("create view area: %w", err)
		}
	} else if viewsInfo.Mode()&os.ModeSymlink != 0 || !viewsInfo.IsDir() {
		return errclass.ErrPathBoundaryEscape.WithMessagef("view area is not a real directory: %s", viewsDir)
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
	if len(result.Transfers) > 0 {
		printPrimaryTransferSummary(&result.Transfers[0])
	}
	fmt.Println("No workspace or history changed.")
}

func printViewCloseResult(result publicViewCloseResult) {
	if result.Status == "already_closed" {
		fmt.Println("Read-only view already closed.")
	} else {
		fmt.Println("Closed read-only view.")
	}
	fmt.Printf("View: %s\n", result.ViewID)
	if result.SavePoint != nil {
		fmt.Printf("Save point: %s\n", color.SnapshotID(*result.SavePoint))
	}
	if result.ViewPath != "" {
		fmt.Printf("View path: %s\n", result.ViewPath)
	}
	if result.ViewPathRemoved {
		fmt.Println("View path removed: yes")
	} else {
		fmt.Println("View path removed: no")
	}
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
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return &errclass.JVSError{Code: jvsErr.Code, Message: message, Hint: viewPointVocabulary(jvsErr.Hint)}
	}
	return fmt.Errorf("%s", message)
}

func viewCloseError(err error) error {
	if err == nil {
		return nil
	}
	message := viewPointVocabulary(err.Error())
	if !strings.Contains(message, "No files or history changed.") {
		message += ". No files or history changed."
	}
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return &errclass.JVSError{Code: jvsErr.Code, Message: message, Hint: viewPointVocabulary(jvsErr.Hint)}
	}
	return fmt.Errorf("%s", message)
}

func viewPointVocabulary(value string) string {
	replacer := strings.NewReplacer(
		"checkpoints", "save points",
		"checkpoint", "save point",
		"snapshots", "save points",
		"snapshot", "save point",
		"active source pins", "read-only view protections",
		"active source pin", "read-only view protection",
		"gc control data", "JVS control data",
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
	viewCmd.AddCommand(viewCloseCmd)
	rootCmd.AddCommand(viewCmd)
}
