package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/workspacepath"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/model"
)

type workspaceStatus struct {
	separatedControlJSONFields

	Repo                 string                     `json:"repo"`
	Folder               string                     `json:"folder"`
	Workspace            string                     `json:"workspace"`
	NewestSavePoint      *string                    `json:"newest_save_point"`
	HistoryHead          *string                    `json:"history_head"`
	ContentSource        *string                    `json:"content_source"`
	StartedFromSavePoint *string                    `json:"started_from_save_point,omitempty"`
	UnsavedChanges       bool                       `json:"unsaved_changes"`
	FilesState           string                     `json:"files_state"`
	PathSources          []publicRestoredPathSource `json:"path_sources,omitempty"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show folder save point status",
	Long: `Show the active folder, workspace, newest save point, file source, and
whether the folder has unsaved changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveWorkspaceScoped()
		if err != nil {
			return err
		}
		if err := validateAndRefreshSeparatedPayloadBoundary(ctx); err != nil {
			return err
		}
		status, err := buildWorkspaceStatus(ctx.Repo.Root, ctx.Workspace)
		if err != nil {
			return err
		}
		if ctx.Separated != nil {
			status.separatedControlJSONFields = separatedControlFields(ctx.Separated, separatedDoctorStrictNotRun)
		}
		if jsonOutput {
			return outputJSON(status)
		}

		printWorkspaceStatus(status)
		return nil
	},
}

func printWorkspaceStatus(status workspaceStatus) {
	fmt.Printf("Repo: %s\n", status.Repo)
	fmt.Printf("Folder: %s\n", status.Folder)
	fmt.Printf("Workspace: %s\n", status.Workspace)
	fmt.Printf("Content source: %s\n", formatStatusSavePoint(status.ContentSource))
	fmt.Printf("Newest save point: %s\n", formatStatusSavePoint(status.NewestSavePoint))
	fmt.Printf("History head: %s\n", formatStatusSavePoint(status.HistoryHead))
	if status.StartedFromSavePoint != nil {
		fmt.Printf("Started from save point: %s\n", formatStatusSavePoint(status.StartedFromSavePoint))
	}
	switch status.FilesState {
	case "not_saved":
		fmt.Println("Not saved yet.")
	case "matches_save_point":
		fmt.Printf("Files match save point: %s\n", formatStatusSavePoint(status.ContentSource))
	case "changed_since_save_point":
		fmt.Printf("Files changed since save point: %s\n", formatStatusSavePoint(status.ContentSource))
	case "restored_then_changed":
		fmt.Printf("Files were last restored from: %s\n", formatStatusSavePoint(status.ContentSource))
	case "started_from_save_point":
		fmt.Printf("Files match save point: %s\n", formatStatusSavePoint(status.ContentSource))
	}
	if status.UnsavedChanges {
		fmt.Println("Unsaved changes: yes")
	} else {
		fmt.Println("Unsaved changes: no")
	}
	if len(status.PathSources) > 0 {
		fmt.Println("Restored paths:")
		for _, source := range status.PathSources {
			suffix := ""
			if source.Status == model.PathSourceModifiedAfterRestore {
				suffix = " (modified after restore)"
			}
			fmt.Printf("  %s from save point %s%s\n", source.TargetPath, formatStatusSavePoint(stringPtrOrNil(source.SourceSavePoint)), suffix)
		}
	}
}

func buildWorkspaceStatus(repoRoot, workspaceName string) (workspaceStatus, error) {
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return workspaceStatus{}, fmt.Errorf("load workspace: %w", err)
	}
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return workspaceStatus{}, fmt.Errorf("workspace folder: %w", err)
	}
	unsavedChanges, err := workspaceDirty(repoRoot, workspaceName)
	if err != nil {
		return workspaceStatus{}, err
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return workspaceStatus{}, fmt.Errorf("workspace path: %w", err)
	}
	projectedPathSources, err := workspacepath.ReconcilePathSources(repoRoot, boundary, cfg.PathSources)
	if err != nil {
		return workspaceStatus{}, fmt.Errorf("reconcile restored paths: %w", err)
	}

	historyHead := statusStringPointer(cfg.LatestSnapshotID)
	contentSource := statusStringPointer(cfg.HeadSnapshotID)
	startedFrom := statusStringPointer(cfg.StartedFromSnapshotID)
	return workspaceStatus{
		Repo:                 repoRoot,
		Folder:               folder,
		Workspace:            workspaceName,
		NewestSavePoint:      historyHead,
		HistoryHead:          historyHead,
		ContentSource:        contentSource,
		StartedFromSavePoint: startedFrom,
		UnsavedChanges:       unsavedChanges,
		FilesState:           filesState(historyHead, contentSource, startedFrom, unsavedChanges),
		PathSources:          publicRestoredPathSources(projectedPathSources.RestoredPaths()),
	}, nil
}

func filesState(historyHead, contentSource, startedFrom *string, unsavedChanges bool) string {
	if contentSource == nil {
		return "not_saved"
	}
	if historyHead == nil && startedFrom != nil && sameStatusSavePoint(contentSource, startedFrom) {
		return "started_from_save_point"
	}
	if unsavedChanges && !sameStatusSavePoint(historyHead, contentSource) {
		return "restored_then_changed"
	}
	if unsavedChanges {
		return "changed_since_save_point"
	}
	return "matches_save_point"
}

func sameStatusSavePoint(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func statusStringPointer(id model.SnapshotID) *string {
	if id == "" {
		return nil
	}
	value := string(id)
	return &value
}

func formatStatusSavePoint(ref *string) string {
	if ref == nil || *ref == "" {
		return "none"
	}
	return color.SnapshotID(model.SnapshotID(*ref).String())
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
