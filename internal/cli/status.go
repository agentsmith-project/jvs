package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/color"
	"github.com/jvs-project/jvs/pkg/model"
)

type workspaceStatus struct {
	Current       string   `json:"current"`
	Latest        string   `json:"latest"`
	Dirty         bool     `json:"dirty"`
	AtLatest      bool     `json:"at_latest"`
	Workspace     string   `json:"workspace"`
	Repo          string   `json:"repo"`
	Engine        string   `json:"engine"`
	RecoveryHints []string `json:"recovery_hints"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show workspace status",
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}
		status, err := buildWorkspaceStatus(r.Root, workspaceName)
		if err != nil {
			return err
		}
		if jsonOutput {
			return outputJSON(status)
		}

		fmt.Printf("Workspace: %s\n", status.Workspace)
		fmt.Printf("Repo: %s\n", color.Dim(status.Repo))
		fmt.Printf("Current: %s\n", formatStatusRef(status.Current))
		fmt.Printf("Latest: %s\n", formatStatusRef(status.Latest))
		fmt.Printf("Dirty: %t\n", status.Dirty)
		fmt.Printf("At latest: %t\n", status.AtLatest)
		fmt.Printf("Engine: %s\n", status.Engine)
		if len(status.RecoveryHints) > 0 {
			fmt.Println()
			for _, hint := range status.RecoveryHints {
				fmt.Printf("- %s\n", hint)
			}
		}
		return nil
	},
}

func buildWorkspaceStatus(repoRoot, workspaceName string) (workspaceStatus, error) {
	cfg, err := worktree.NewManager(repoRoot).Get(workspaceName)
	if err != nil {
		return workspaceStatus{}, fmt.Errorf("load workspace: %w", err)
	}
	dirty, err := workspaceDirty(repoRoot, workspaceName)
	if err != nil {
		return workspaceStatus{}, err
	}

	atLatest := cfg.HeadSnapshotID != "" && cfg.HeadSnapshotID == cfg.LatestSnapshotID && !dirty
	return workspaceStatus{
		Current:       string(cfg.HeadSnapshotID),
		Latest:        string(cfg.LatestSnapshotID),
		Dirty:         dirty,
		AtLatest:      atLatest,
		Workspace:     workspaceName,
		Repo:          repoRoot,
		Engine:        string(detectEngine(repoRoot)),
		RecoveryHints: statusRecoveryHints(cfg.HeadSnapshotID, cfg.LatestSnapshotID, dirty),
	}, nil
}

func statusForRestore(repoRoot, workspaceName string) (map[string]any, error) {
	status, err := buildWorkspaceStatus(repoRoot, workspaceName)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"current":       status.Current,
		"latest":        status.Latest,
		"dirty":         status.Dirty,
		"at_latest":     status.AtLatest,
		"checkpoint_id": status.Current,
		"status":        "restored",
	}, nil
}

func formatStatusRef(ref string) string {
	if ref == "" {
		return color.Dim("(none)")
	}
	return color.SnapshotID(model.SnapshotID(ref).String())
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
