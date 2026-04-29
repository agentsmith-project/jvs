package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

var (
	historyLimit      int
	historyNoteFilter string
	historyAll        bool
	historyPath       string
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show save point history",
	Long: `Show save points for the active workspace.

Examples:
  jvs history
  jvs history --path notes.md
  jvs history -n 10
  jvs history --grep "baseline"

After finding a candidate, open it with:
  jvs view <save> <path>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}
		if historyPathFlagChanged(cmd) {
			if err := validateHistoryPathFlagCombination(cmd); err != nil {
				return historyPathError(err)
			}
			path, err := normalizeHistoryPath(historyPath)
			if err != nil {
				return historyPathError(err)
			}
			result, err := findHistoryPathCandidates(r.Root, workspaceName, path)
			if err != nil {
				return historyPathError(err)
			}
			if jsonOutput {
				return outputJSON(result)
			}
			printHistoryPathCandidates(result)
			return nil
		}

		cfg, savePoints, err := loadSavePointHistory(r.Root, workspaceName)
		if err != nil {
			return err
		}

		if jsonOutput {
			return outputJSON(publicSavePointHistory(workspaceName, savePoints, cfg.LatestSnapshotID, cfg.StartedFromSnapshotID))
		}

		if len(savePoints) == 0 {
			fmt.Println("No save points yet.")
			if cfg.StartedFromSnapshotID != "" {
				fmt.Printf("Workspace started from %s.\n", color.SnapshotID(cfg.StartedFromSnapshotID.String()))
			}
			return nil
		}

		if cfg.StartedFromSnapshotID != "" {
			fmt.Printf("Workspace started from %s.\n", color.SnapshotID(cfg.StartedFromSnapshotID.String()))
		}
		if historyAll {
			fmt.Println("Save points:")
		} else {
			fmt.Printf("Save points for workspace %s:\n", workspaceName)
		}
		for _, desc := range savePoints {
			message := desc.Note
			if message == "" {
				message = color.Dim("(no message)")
			}
			fmt.Printf("%s  %s  %s\n",
				color.SnapshotID(desc.SnapshotID.ShortID()),
				color.Dim(desc.CreatedAt.Format("2006-01-02 15:04")),
				message,
			)
		}
		return nil
	},
}

type publicHistoryPathResult struct {
	Folder       string                       `json:"folder"`
	Workspace    string                       `json:"workspace"`
	Path         string                       `json:"path"`
	Candidates   []publicHistoryPathCandidate `json:"candidates"`
	NextCommands []string                     `json:"next_commands"`
}

type publicHistoryPathCandidate struct {
	SavePointID string    `json:"save_point_id"`
	Message     string    `json:"message,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func historyPathFlagChanged(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("path")
	return flag != nil && flag.Changed
}

func validateHistoryPathFlagCombination(cmd *cobra.Command) error {
	var unsupported []string
	for _, name := range []string{"all", "grep", "limit"} {
		flag := cmd.Flags().Lookup(name)
		if flag != nil && flag.Changed {
			unsupported = append(unsupported, "--"+name)
		}
	}
	if len(unsupported) == 0 {
		return nil
	}
	return fmt.Errorf("history --path only searches path candidates in this workspace for now; remove %s", strings.Join(unsupported, ", "))
}

func normalizeHistoryPath(raw string) (string, error) {
	path, err := normalizeViewPath(raw)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("path inside save point must be a workspace-relative path")
	}
	return path, nil
}

func findHistoryPathCandidates(repoRoot, workspaceName, path string) (publicHistoryPathResult, error) {
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return publicHistoryPathResult{}, fmt.Errorf("load workspace: %w", err)
	}
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return publicHistoryPathResult{}, fmt.Errorf("workspace folder: %w", err)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return publicHistoryPathResult{}, fmt.Errorf("workspace managed boundary: %w", err)
	}

	descs, err := loadCurrentWorkspaceSavePointLineage(repoRoot, cfg)
	if err != nil {
		return publicHistoryPathResult{}, err
	}
	candidates := make([]publicHistoryPathCandidate, 0)
	for _, desc := range descs {
		matches, err := savePointContainsHistoryPath(repoRoot, desc, path, boundary)
		if err != nil {
			return publicHistoryPathResult{}, err
		}
		if matches {
			candidates = append(candidates, publicHistoryPathCandidate{
				SavePointID: string(desc.SnapshotID),
				Message:     desc.Note,
				CreatedAt:   desc.CreatedAt,
			})
		}
	}
	result := publicHistoryPathResult{
		Folder:       folder,
		Workspace:    workspaceName,
		Path:         path,
		Candidates:   candidates,
		NextCommands: historyPathNextCommands(path, candidates),
	}
	return result, nil
}

func loadCurrentWorkspaceSavePointLineage(repoRoot string, cfg *model.WorktreeConfig) ([]*model.Descriptor, error) {
	if cfg.LatestSnapshotID == "" {
		return []*model.Descriptor{}, nil
	}
	var savePoints []*model.Descriptor
	currentID := &cfg.LatestSnapshotID
	for currentID != nil {
		desc, err := snapshot.LoadDescriptor(repoRoot, *currentID)
		if err != nil {
			return nil, fmt.Errorf("load save point: %w", err)
		}
		savePoints = append(savePoints, desc)
		currentID = desc.ParentID
	}
	return savePoints, nil
}

func savePointContainsHistoryPath(repoRoot string, desc *model.Descriptor, path string, boundary repo.WorktreePayloadBoundary) (bool, error) {
	if desc == nil {
		return false, nil
	}
	state, issue := snapshot.InspectPublishState(repoRoot, desc.SnapshotID, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})
	if issue != nil {
		return false, snapshot.PublishStateIssueError(issue)
	}

	tempParent, err := os.MkdirTemp("", "jvs-history-path-*")
	if err != nil {
		return false, fmt.Errorf("create temporary history path area: %w", err)
	}
	defer func() {
		_ = restoreWriteBits(tempParent)
		_ = os.RemoveAll(tempParent)
	}()

	payloadRoot := filepath.Join(tempParent, "payload")
	opts := snapshotpayload.OptionsFromDescriptor(state.Descriptor)
	if err := snapshotpayload.MaterializeToNew(state.SnapshotDir, payloadRoot, opts, func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	}); err != nil {
		return false, fmt.Errorf("read save point payload: %w", err)
	}
	if err := repo.ValidateManagedPayloadOnly(boundary, payloadRoot); err != nil {
		return false, err
	}

	if err := pathutil.ValidateNoSymlinkParents(payloadRoot, path); err != nil {
		return false, fmt.Errorf("path inside save point must not traverse symlinks: %w", err)
	}
	target := filepath.Join(payloadRoot, filepath.FromSlash(path))
	_, err = os.Lstat(target)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat path inside save point: %w", err)
}

func historyPathNextCommands(path string, candidates []publicHistoryPathCandidate) []string {
	if len(candidates) == 0 {
		return []string{}
	}
	savePointID := candidates[0].SavePointID
	return []string{
		fmt.Sprintf("jvs view %s -- %s", savePointID, shellQuoteArg(path)),
	}
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	for _, r := range arg {
		if isShellSafeRune(r) {
			continue
		}
		return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return arg
}

func isShellSafeRune(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	switch r {
	case '_', '-', '.', '/', ':', '@', '%', '+', '=':
		return true
	default:
		return false
	}
}

func printHistoryPathCandidates(result publicHistoryPathResult) {
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	fmt.Printf("Candidates for path: %s\n", result.Path)
	if len(result.Candidates) == 0 {
		fmt.Println("No candidates found.")
		fmt.Println("No files were changed.")
		return
	}
	for _, candidate := range result.Candidates {
		message := candidate.Message
		if message == "" {
			message = color.Dim("(no message)")
		}
		fmt.Printf("%s  %s  %s\n",
			color.SnapshotID(candidate.SavePointID),
			color.Dim(candidate.CreatedAt.Format("2006-01-02 15:04")),
			message,
		)
	}
	fmt.Println("Next:")
	for _, command := range result.NextCommands {
		fmt.Printf("  %s\n", command)
	}
	fmt.Println("No files were changed.")
}

func historyPathError(err error) error {
	if err == nil {
		return nil
	}
	message := publicSavePointVocabulary(err.Error())
	if !strings.Contains(message, "No files were changed.") {
		message += ". No files were changed."
	}
	return fmt.Errorf("%s", message)
}

func loadSavePointHistory(repoRoot, workspaceName string) (*model.WorktreeConfig, []*model.Descriptor, error) {
	cfg, err := worktree.NewManager(repoRoot).Get(workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("load workspace: %w", err)
	}

	if historyAll {
		savePoints, err := snapshot.Find(repoRoot, snapshot.FilterOptions{
			NoteContains: historyNoteFilter,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("list save points: %w", err)
		}
		return cfg, limitSavePoints(savePoints), nil
	}

	if cfg.LatestSnapshotID == "" {
		return cfg, []*model.Descriptor{}, nil
	}

	var savePoints []*model.Descriptor
	currentID := &cfg.LatestSnapshotID
	for currentID != nil && (historyLimit == 0 || len(savePoints) < historyLimit) {
		desc, err := snapshot.LoadDescriptor(repoRoot, *currentID)
		if err != nil {
			return nil, nil, fmt.Errorf("load save point: %w", err)
		}
		if historyNoteFilter != "" && !strings.Contains(desc.Note, historyNoteFilter) {
			currentID = desc.ParentID
			continue
		}
		savePoints = append(savePoints, desc)
		currentID = desc.ParentID
	}
	return cfg, savePoints, nil
}

func limitSavePoints(savePoints []*model.Descriptor) []*model.Descriptor {
	if historyLimit <= 0 || len(savePoints) <= historyLimit {
		return savePoints
	}
	return savePoints[:historyLimit]
}

func init() {
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "n", 0, "limit number of save points (0 = all)")
	historyCmd.Flags().StringVarP(&historyNoteFilter, "grep", "g", "", "filter by message substring")
	historyCmd.Flags().StringVar(&historyPath, "path", "", "find save points that contain a workspace-relative path")
	historyCmd.Flags().BoolVar(&historyAll, "all", false, "show save points from all workspaces")
	rootCmd.AddCommand(historyCmd)
}
