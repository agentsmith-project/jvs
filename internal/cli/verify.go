package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/verify"
	"github.com/jvs-project/jvs/pkg/model"
)

var (
	verifyAll bool
)

var verifyCmd = &cobra.Command{
	Use:   "verify [<snapshot-id>]",
	Short: "Verify snapshot integrity",
	Long: `Verify snapshot integrity.

Checks descriptor checksum and logical payload hash.

Examples:
  jvs verify                    # Verify all snapshots
  jvs verify 1771589abc         # Verify specific snapshot
  jvs verify --all              # Verify all snapshots`,
	Args: cobra.MaximumNArgs(1),
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
				outputJSON(results)
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
				fmt.Printf("%s  %s\n", res.SnapshotID, status)
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
				outputJSON(result)
				if result.TamperDetected {
					os.Exit(1)
				}
				return
			}

			fmt.Printf("Snapshot: %s\n", result.SnapshotID)
			fmt.Printf("  Checksum: %v\n", result.ChecksumValid)
			fmt.Printf("  Payload hash: %v\n", result.PayloadHashValid)
			if result.TamperDetected {
				fmt.Printf("  TAMPER DETECTED: %s\n", result.Error)
				os.Exit(1)
			}
		}
	},
}

func init() {
	verifyCmd.Flags().BoolVar(&verifyAll, "all", false, "verify all snapshots")
	rootCmd.AddCommand(verifyCmd)
}
