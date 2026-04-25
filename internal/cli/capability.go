package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/engine"
)

var capabilityWriteProbe bool

var capabilityCmd = &cobra.Command{
	Use:   "capability <target-path>",
	Short: "Probe filesystem capabilities for a target path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		report, err := engine.ProbeCapabilities(args[0], capabilityWriteProbe)
		if err != nil {
			return err
		}
		if jsonOutput {
			return outputJSON(report)
		}

		fmt.Printf("Target: %s\n", report.TargetPath)
		if report.ProbePath != report.TargetPath {
			fmt.Printf("  Probe path: %s\n", report.ProbePath)
		}
		fmt.Printf("  JuiceFS: available=%t supported=%t\n", report.JuiceFS.Available, report.JuiceFS.Supported)
		fmt.Printf("  Reflink: supported=%t confidence=%s\n", report.Reflink.Supported, report.Reflink.Confidence)
		fmt.Printf("  Copy: supported=%t\n", report.Copy.Supported)
		fmt.Printf("  Recommended engine: %s\n", report.RecommendedEngine)
		for _, warning := range report.Warnings {
			fmt.Printf("  Warning: %s\n", warning)
		}
		return nil
	},
}

func init() {
	capabilityCmd.Flags().BoolVar(&capabilityWriteProbe, "write-probe", false, "create temporary files to confirm reflink support")
	rootCmd.AddCommand(capabilityCmd)
}
