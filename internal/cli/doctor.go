package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
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
Use --strict to include full save point integrity verification.
Use --repair-runtime to execute safe automatic repairs.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return errclass.ErrUsage.WithMessage("doctor does not accept positional arguments")
	},
	Run: func(cmd *cobra.Command, args []string) {
		if targetControlRoot != "" {
			if strings.TrimSpace(targetWorkspaceName) == "" {
				exitWithCLIError(separatedControlRootRequiresWorkspaceError(targetControlRoot))
			}
			if doctorStrict && jsonOutput && !doctorRepair && !doctorRepairList {
				runSeparatedStrictDoctorJSON()
				return
			}
			exitWithCLIError(separatedDoctorStrictJSONRequiredError(targetControlRoot))
		}

		r, err := discoverRequiredRepoForDoctor()
		if err != nil {
			exitWithCLIError(err)
		}

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
		var repairs []doctor.RepairResult
		if doctorRepair {
			results, err := doc.Repair(doctor.RuntimeRepairActionIDs())
			if err != nil {
				fmtErr("repair: %v", err)
				os.Exit(1)
			}
			repairs = results
			if !jsonOutput {
				for _, r := range results {
					fmt.Printf("Repair %s: %s\n", r.Action, publicContractVocabulary(r.Message))
				}
			}
		}

		result, err := doc.Check(doctorStrict)
		if err != nil {
			fmtErr("doctor: %v", err)
			os.Exit(1)
		}

		if jsonOutput {
			outputJSON(publicDoctorWithRepairs(result, repairs))
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
				errCode = fmt.Sprintf(" [%s]", publicErrorCodeVocabulary(f.ErrorCode))
			}
			fmt.Printf("  [%s] %s: %s%s\n", f.Severity, publicContractVocabulary(f.Category), publicContractVocabulary(f.Description), errCode)
		}

		if !result.Healthy {
			os.Exit(1)
		}
	},
}

func runSeparatedStrictDoctorJSON() {
	if targetRepoPath != "" {
		exitWithCLIError(errclass.ErrUsage.WithMessage("--control-root cannot be combined with --repo"))
	}
	if targetWorkspaceName == "" {
		exitWithCLIError(separatedControlRootRequiresWorkspaceError(targetControlRoot))
	}
	result, err := doctor.CheckSeparatedStrict(repo.SeparatedContextRequest{
		ControlRoot: targetControlRoot,
		Workspace:   targetWorkspaceName,
	})
	if err != nil {
		exitWithCLIError(err)
	}
	recordResolvedTarget(result.ControlRoot, result.Workspace)
	outputJSON(result)
	if !result.Healthy {
		os.Exit(1)
	}
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorStrict, "strict", false, "include full save point integrity verification")
	doctorCmd.Flags().BoolVar(&doctorRepair, "repair-runtime", false, "execute safe automatic repairs")
	doctorCmd.Flags().BoolVar(&doctorRepairList, "repair-list", false, "list available repair actions")
	rootCmd.AddCommand(doctorCmd)
}
