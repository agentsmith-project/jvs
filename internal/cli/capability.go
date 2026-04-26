package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/errclass"
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
		if capabilityWriteProbe && !report.Write.Supported {
			detail := strings.Join(report.Write.Warnings, "; ")
			if detail == "" {
				detail = "write probe did not confirm writable target"
			}
			return errclass.ErrUsage.WithMessagef("target path is not writable: %s", detail)
		}
		if jsonOutput {
			output := map[string]any{
				"target_path":        report.TargetPath,
				"probe_path":         report.ProbePath,
				"write_probe":        report.WriteProbe,
				"write":              report.Write,
				"juicefs":            report.JuiceFS,
				"reflink":            report.Reflink,
				"copy":               report.Copy,
				"recommended_engine": report.RecommendedEngine,
			}
			applySetupJSONFields(output, report, report.RecommendedEngine, report.Warnings)
			return outputJSON(output)
		}

		fmt.Printf("Target: %s\n", report.TargetPath)
		if report.ProbePath != report.TargetPath {
			fmt.Printf("  Probe path: %s\n", report.ProbePath)
		}
		fmt.Printf("  Write: supported=%t confidence=%s\n", report.Write.Supported, report.Write.Confidence)
		fmt.Printf("  JuiceFS: available=%t supported=%t\n", report.JuiceFS.Available, report.JuiceFS.Supported)
		fmt.Printf("  Reflink: supported=%t confidence=%s\n", report.Reflink.Supported, report.Reflink.Confidence)
		fmt.Printf("  Copy: supported=%t confidence=%s\n", report.Copy.Supported, report.Copy.Confidence)
		fmt.Printf("  Recommended engine: %s\n", report.RecommendedEngine)
		for _, warning := range report.Warnings {
			fmt.Printf("  Warning: %s\n", warning)
		}
		return nil
	},
}

func init() {
	capabilityCmd.Flags().BoolVar(&capabilityWriteProbe, "write-probe", false, "create temporary files to confirm write, remove, and reflink support")
	rootCmd.AddCommand(capabilityCmd)
}
