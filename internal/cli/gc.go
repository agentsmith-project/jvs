package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/gc"
	"github.com/jvs-project/jvs/pkg/progress"
)

var (
	gcPlanID string
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Garbage collection",
}

var gcPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Create a GC plan",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}

		collector := gc.NewCollector(r.Root)
		plan, err := collector.Plan()
		if err != nil {
			return fmt.Errorf("create gc plan: %w", err)
		}

		if jsonOutput {
			return outputJSON(publicGC(plan))
		}

		fmt.Printf("Plan ID: %s\n", plan.PlanID)
		fmt.Printf("  Protected by lineage: %d checkpoints\n", plan.ProtectedByLineage)
		fmt.Printf("  To delete: %d checkpoints\n", len(plan.ToDelete))
		fmt.Printf("  Estimated reclaim: %d bytes\n", plan.DeletableBytesEstimate)
		fmt.Println()
		fmt.Printf("Run: jvs gc run --plan-id %s\n", plan.PlanID)
		return nil
	},
}

var gcRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute a GC plan",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}

		if gcPlanID == "" {
			return fmt.Errorf("--plan-id is required")
		}

		collector := gc.NewCollector(r.Root)

		// Add progress callback if enabled
		if progressEnabled() {
			// First get the plan to know total
			plan, err := collector.LoadPlan(gcPlanID)
			if err != nil {
				return fmt.Errorf("load plan: %w", err)
			}

			total := len(plan.ToDelete)
			if total > 0 {
				term := progress.NewTerminal("GC", total, true)
				cb := term.Callback()
				collector.SetProgressCallback(cb)
			}
		}

		if err := collector.Run(gcPlanID); err != nil {
			if jsonOutput {
				return err
			}
			return fmt.Errorf("%v", err)
		}

		if jsonOutput {
			return outputJSON(map[string]string{"plan_id": gcPlanID, "status": "completed"})
		}
		fmt.Println("GC completed successfully.")
		return nil
	},
}

func init() {
	gcRunCmd.Flags().StringVar(&gcPlanID, "plan-id", "", "plan ID to execute")
	gcCmd.AddCommand(gcPlanCmd)
	gcCmd.AddCommand(gcRunCmd)
	rootCmd.AddCommand(gcCmd)
}
