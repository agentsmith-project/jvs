package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/gc"
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
			return outputJSON(publicCleanup(plan))
		}

		fmt.Printf("Plan ID: %s\n", plan.PlanID)
		fmt.Printf("  Protected by history: %d save points\n", plan.ProtectedByLineage)
		fmt.Printf("  Reclaimable: %d save points\n", len(plan.ToDelete))
		fmt.Printf("  Estimated reclaim: %d bytes\n", plan.DeletableBytesEstimate)
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
			if jsonOutput {
				return err
			}
			return fmt.Errorf("%v", err)
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
