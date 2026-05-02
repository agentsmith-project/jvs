package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/progress"
)

var (
	cleanupPlanID string
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up unneeded save point storage",
	Long: `Clean up unneeded save point storage.

Cleanup starts with a preview plan. Run the listed plan ID to remove
reclaimable save point storage.`,
}

var cleanupPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview cleanup work",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}

		collector := gc.NewCollector(r.Root)
		plan, err := collector.Plan()
		if err != nil {
			return fmt.Errorf("create cleanup plan: %w", err)
		}

		if jsonOutput {
			record, err := publicCleanup(plan)
			if err != nil {
				return err
			}
			return outputJSON(record)
		}

		fmt.Printf("Plan ID: %s\n", plan.PlanID)
		fmt.Println("Protected save points:")
		printCleanupProtectionGroups(plan.ProtectionGroups)
		fmt.Printf("Reclaimable: %d save points\n", len(plan.ToDelete))
		fmt.Printf("Estimated reclaim: %d bytes\n", plan.DeletableBytesEstimate)
		fmt.Println()
		fmt.Printf("Run: jvs cleanup run --plan-id %s\n", plan.PlanID)
		return nil
	},
}

var cleanupRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a cleanup plan",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}

		if cleanupPlanID == "" {
			return fmt.Errorf("--plan-id is required")
		}

		collector := gc.NewCollector(r.Root)

		// Add progress callback if enabled
		if progressEnabled() {
			// First get the plan to know total
			plan, err := collector.LoadPlan(cleanupPlanID)
			if err != nil {
				return fmt.Errorf("load plan: %w", err)
			}

			total := len(plan.ToDelete)
			if total > 0 {
				term := progress.NewTerminal("Cleanup", total, true)
				cb := term.Callback()
				collector.SetProgressCallback(cb)
			}
		}

		if err := collector.Run(cleanupPlanID); err != nil {
			return err
		}

		if jsonOutput {
			return outputJSON(map[string]string{"plan_id": cleanupPlanID, "status": "completed"})
		}
		fmt.Println("Cleanup completed successfully.")
		return nil
	},
}

func init() {
	cleanupRunCmd.Flags().StringVar(&cleanupPlanID, "plan-id", "", "plan ID to execute")
	cleanupCmd.AddCommand(cleanupPreviewCmd)
	cleanupCmd.AddCommand(cleanupRunCmd)
	rootCmd.AddCommand(cleanupCmd)
}

func printCleanupProtectionGroups(groups []model.GCProtectionGroup) {
	if len(groups) == 0 {
		fmt.Println("  none")
		return
	}
	for _, group := range groups {
		fmt.Printf("  %s: %d save point%s\n", cleanupProtectionReasonLabel(group.Reason), group.Count, pluralS(group.Count))
		for _, id := range group.SavePoints {
			fmt.Printf("    %s\n", id)
		}
	}
}

func cleanupProtectionReasonLabel(reason string) string {
	switch reason {
	case model.GCProtectionReasonHistory:
		return "workspace history"
	case model.GCProtectionReasonOpenView:
		return "open views"
	case model.GCProtectionReasonActiveRecovery:
		return "active recovery plans"
	case model.GCProtectionReasonActiveOperation:
		return "active operations"
	case model.GCProtectionReasonImportedCloneHistory:
		return "imported clone history"
	default:
		return reason
	}
}

func pluralS(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
