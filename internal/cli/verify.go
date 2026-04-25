package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/verify"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/model"
)

var (
	verifyAll bool
)

var verifyCmd = &cobra.Command{
	Use:   "verify [<checkpoint-id>]",
	Short: "Verify checkpoint integrity",
	Long: `Verify checkpoint integrity.

Checks descriptor checksum and logical payload hash.

Examples:
  jvs verify                    # Verify all checkpoints
  jvs verify 1771589abc         # Verify specific checkpoint
  jvs verify --all              # Verify all checkpoints`,
	Args: func(cmd *cobra.Command, args []string) error {
		if verifyAll && len(args) > 0 {
			return errclass.ErrUsage.WithMessage("verify --all does not accept a checkpoint id")
		}
		return cobra.MaximumNArgs(1)(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()

		verifier := verify.NewVerifier(r.Root)

		if verifyAll || len(args) == 0 {
			results, err := verifier.VerifyAll(true)
			if err != nil {
				fmtErr("verify: %v", err)
				os.Exit(1)
			}

			tampered := false
			for _, res := range results {
				if res.TamperDetected {
					tampered = true
				}
			}

			if jsonOutput {
				outputJSON(publicVerifyResults(results))
				if tampered {
					os.Exit(1)
				}
				return
			}

			for _, res := range results {
				status := "OK"
				if res.TamperDetected {
					status = "TAMPERED"
				}
				errCode := ""
				if res.ErrorCode != "" {
					errCode = fmt.Sprintf("  [%s]", publicErrorCodeVocabulary(res.ErrorCode))
				}
				fmt.Printf("%s  %s%s\n", res.SnapshotID, status, errCode)
			}

			if tampered {
				os.Exit(1)
			}
		} else {
			snapshotID := model.SnapshotID(args[0])
			result, err := verifier.VerifySnapshot(snapshotID, true)
			if err != nil {
				fmtErr("verify: %v", err)
				os.Exit(1)
			}

			if jsonOutput {
				outputJSON(publicVerify(result))
				if result.TamperDetected {
					os.Exit(1)
				}
				return
			}

			fmt.Printf("Checkpoint: %s\n", result.SnapshotID)
			fmt.Printf("  Checksum: %v\n", result.ChecksumValid)
			fmt.Printf("  Payload hash: %v\n", result.PayloadHashValid)
			if result.TamperDetected {
				errCode := ""
				if result.ErrorCode != "" {
					errCode = fmt.Sprintf(" [%s]", publicErrorCodeVocabulary(result.ErrorCode))
				}
				fmt.Printf("  TAMPER DETECTED%s: %s\n", errCode, publicContractVocabulary(result.Error))
				os.Exit(1)
			}
		}
	},
}

func init() {
	verifyCmd.Flags().BoolVar(&verifyAll, "all", false, "verify all checkpoints")
	rootCmd.AddCommand(verifyCmd)
}
