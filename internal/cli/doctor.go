package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/doctor"
)

var (
	doctorStrict     bool
	doctorRepair     bool
	doctorRepairList bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check repository health",
	Long: `Check repository health.

Runs diagnostic checks on the repository and reports any issues.
Use --strict to include full checkpoint integrity verification.
Use --repair-runtime to execute safe automatic repairs.`,
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()

		doc := doctor.NewDoctor(r.Root)

		// If --repair-list, show available repair actions
		if doctorRepairList {
			actions := doc.ListRepairActions()
			if jsonOutput {
				outputJSON(actions)
				return
			}
			fmt.Println("Available repair actions:")
			for _, a := range actions {
				safe := ""
				if a.AutoSafe {
					safe = " (safe)"
				}
				fmt.Printf("  %s%s: %s\n", a.ID, safe, a.Description)
			}
			return
		}

		// If --repair-runtime, execute safe repairs first
		if doctorRepair {
			results, err := doc.Repair([]string{"clean_tmp", "clean_intents"})
			if err != nil {
				fmtErr("repair: %v", err)
				os.Exit(1)
			}
			if !jsonOutput {
				for _, r := range results {
					fmt.Printf("Repair %s: %s\n", r.Action, r.Message)
				}
			}
		}

		result, err := doc.Check(doctorStrict)
		if err != nil {
			fmtErr("doctor: %v", err)
			os.Exit(1)
		}

		if jsonOutput {
			outputJSON(result)
			if !result.Healthy {
				os.Exit(1)
			}
			return
		}

		if len(result.Findings) == 0 {
			fmt.Println("Repository is healthy.")
			return
		}

		fmt.Printf("Findings (%d):\n", len(result.Findings))
		for _, f := range result.Findings {
			errCode := ""
			if f.ErrorCode != "" {
				errCode = fmt.Sprintf(" [%s]", f.ErrorCode)
			}
			fmt.Printf("  [%s] %s: %s%s\n", f.Severity, f.Category, f.Description, errCode)
		}

		if !result.Healthy {
			os.Exit(1)
		}
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorStrict, "strict", false, "include full integrity verification")
	doctorCmd.Flags().BoolVar(&doctorRepair, "repair-runtime", false, "execute safe automatic repairs")
	doctorCmd.Flags().BoolVar(&doctorRepairList, "repair-list", false, "list available repair actions")
	rootCmd.AddCommand(doctorCmd)
}
