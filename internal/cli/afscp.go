package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/afscp"
)

var (
	afscpHome              string
	afscpMessage           string
	afscpPurpose           string
	afscpSavePoint         string
	afscpTargetControlRoot string
	afscpTargetHome        string
)

var afscpCmd = &cobra.Command{
	Use:   "afscp",
	Short: "Internal AFSCP direct contract",
	Long: `Internal AFSCP direct contract for trusted control-plane callers.

Usage:
  jvs afscp --control-root <control_root_path> --home <payload_home_path> <command> --json`,
	Hidden: true,
}

var afscpSaveCmd = &cobra.Command{
	Use:    "save",
	Short:  "Create an internal direct save point",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		runAFSCPDirect(cmd.Context(), afscp.CommandSave, args, func(ctx context.Context, request afscp.Request) (any, error) {
			request.Message = afscpMessage
			request.Purpose = afscpPurpose
			return afscp.NewService().Save(ctx, request)
		})
	},
}

var afscpListCmd = &cobra.Command{
	Use:    "list",
	Short:  "List internal direct save points",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		runAFSCPDirect(cmd.Context(), afscp.CommandList, args, func(ctx context.Context, request afscp.Request) (any, error) {
			return afscp.NewService().List(ctx, request)
		})
	},
}

var afscpRestoreCmd = &cobra.Command{
	Use:    "restore",
	Short:  "Restore an internal direct save point",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		runAFSCPDirect(cmd.Context(), afscp.CommandRestore, args, func(ctx context.Context, request afscp.Request) (any, error) {
			request.SavePointID = afscpSavePoint
			return afscp.NewService().Restore(ctx, request)
		})
	},
}

var afscpCloneCmd = &cobra.Command{
	Use:    "clone",
	Short:  "Clone an internal direct save point into a new target",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		runAFSCPDirect(cmd.Context(), afscp.CommandClone, args, func(ctx context.Context, request afscp.Request) (any, error) {
			request.SavePointID = afscpSavePoint
			request.TargetSelector = afscp.Selector{ControlRoot: afscpTargetControlRoot, Home: afscpTargetHome}
			return afscp.NewService().Clone(ctx, request)
		})
	},
}

var afscpStatusCmd = &cobra.Command{
	Use:    "status",
	Short:  "Show internal direct metadata status",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		runAFSCPDirect(cmd.Context(), afscp.CommandStatus, args, func(ctx context.Context, request afscp.Request) (any, error) {
			return afscp.NewService().Status(ctx, request)
		})
	},
}

var afscpDoctorCmd = &cobra.Command{
	Use:    "doctor",
	Short:  "Check internal direct metadata health",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		runAFSCPDirect(cmd.Context(), afscp.CommandDoctor, args, func(ctx context.Context, request afscp.Request) (any, error) {
			return afscp.NewService().Doctor(ctx, request)
		})
	},
}

type afscpDirectRun func(context.Context, afscp.Request) (any, error)

func runAFSCPDirect(ctx context.Context, command afscp.Command, args []string, run afscpDirectRun) {
	if len(args) > 0 {
		exitWithAFSCPDirectError(command, afscp.NewError(afscp.ErrorCodeInvalidArgument, "direct command does not accept positional arguments", false))
		return
	}
	if !jsonOutput {
		fmt.Fprintln(os.Stderr, "afscp direct commands require --json")
		os.Exit(afscp.ExitInvalidArgument)
	}

	result, err := run(ctx, afscp.Request{
		Selector: afscp.Selector{
			ControlRoot: targetControlRoot,
			Home:        afscpHome,
		},
	})
	if err != nil {
		exitWithAFSCPDirectError(command, err)
		return
	}
	writeAFSCPDirectEnvelope(afscp.SuccessEnvelope(command, result))
}

func exitWithAFSCPDirectError(command afscp.Command, err error) {
	writeAFSCPDirectEnvelope(afscp.ErrorEnvelope(command, err))
	os.Exit(afscp.ExitCode(err))
}

func writeAFSCPDirectEnvelope(envelope afscp.Envelope) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(envelope)
}

func init() {
	afscpCmd.PersistentFlags().StringVar(&afscpHome, "home", "", "payload HOME root for the internal direct contract")
	afscpSaveCmd.Flags().StringVar(&afscpMessage, "message", "", "save point message")
	afscpSaveCmd.Flags().StringVar(&afscpPurpose, "purpose", "", "save point purpose")
	afscpRestoreCmd.Flags().StringVar(&afscpSavePoint, "save-point", "", "save point id to restore")
	afscpCloneCmd.Flags().StringVar(&afscpSavePoint, "save-point", "", "save point id to clone; defaults to history head")
	afscpCloneCmd.Flags().StringVar(&afscpTargetControlRoot, "target-control-root", "", "target external control data root")
	afscpCloneCmd.Flags().StringVar(&afscpTargetHome, "target-home", "", "target payload HOME root")

	afscpCmd.AddCommand(afscpSaveCmd)
	afscpCmd.AddCommand(afscpListCmd)
	afscpCmd.AddCommand(afscpRestoreCmd)
	afscpCmd.AddCommand(afscpCloneCmd)
	afscpCmd.AddCommand(afscpStatusCmd)
	afscpCmd.AddCommand(afscpDoctorCmd)
	rootCmd.AddCommand(afscpCmd)
}
