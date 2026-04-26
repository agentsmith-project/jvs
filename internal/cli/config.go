package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:    "config <command>",
	Short:  "Manage JVS configuration",
	Hidden: true,
	Long: `Manage JVS configuration stored in .jvs/config.yaml.

Configuration options:
  default_engine    - Default snapshot engine (juicefs-clone, reflink-copy, copy, auto)
  default_tags      - Tags automatically added to each snapshot (list)
  output_format     - Default output format (text, json)
  progress_enabled  - Enable progress bars (true, false)

Available commands:
  show              - Show current configuration
  set <key> <value> - Set a configuration value
  get <key>         - Get a configuration value`,
	DisableFlagsInUseLine: true,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  "Show the current JVS configuration from .jvs/config.yaml.",
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()
		cfg, err := config.Load(r.Root)
		if err != nil {
			fmtErr("load config: %v", err)
			os.Exit(1)
		}

		if jsonOutput {
			outputJSON(cfg)
			return
		}

		// Display config in a readable format
		fmt.Println("# JVS Configuration")
		fmt.Printf("# Location: %s/.jvs/config.yaml\n\n", r.Root)

		if cfg.DefaultEngine != "" {
			fmt.Printf("default_engine: %s\n", cfg.DefaultEngine)
		} else {
			fmt.Println("default_engine: (not set)")
		}

		if len(cfg.DefaultTags) > 0 {
			fmt.Printf("default_tags:\n")
			for _, tag := range cfg.DefaultTags {
				fmt.Printf("  - %s\n", tag)
			}
		} else {
			fmt.Println("default_tags: (not set)")
		}

		if cfg.OutputFormat != "" {
			fmt.Printf("output_format: %s\n", cfg.OutputFormat)
		} else {
			fmt.Println("output_format: (not set)")
		}

		if cfg.ProgressEnabled != nil {
			fmt.Printf("progress_enabled: %v\n", *cfg.ProgressEnabled)
		} else {
			fmt.Println("progress_enabled: (auto-detect)")
		}
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value in .jvs/config.yaml.

Examples:
  jvs config set default_engine juicefs-clone
  jvs config set default_tags "[\"auto\",\"dev\"]"
  jvs config set output_format json
  jvs config set progress_enabled true

Available keys:
  default_engine    - Default snapshot engine (juicefs-clone, reflink-copy, copy, auto)
  default_tags      - Tags automatically added to each snapshot (YAML list)
  output_format     - Default output format (text, json)
  progress_enabled  - Enable progress bars (true, false)`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()

		key := args[0]
		value := args[1]

		err := repo.WithMutationLock(r.Root, "config set", func() error {
			cfg, err := config.Load(r.Root)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if err := cfg.Set(key, value); err != nil {
				return fmt.Errorf("set config: %w", err)
			}
			if err := config.Save(r.Root, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			return nil
		})
		if err != nil {
			fmtErr("%v", err)
			os.Exit(1)
		}

		fmt.Printf("Set %s = %s\n", key, value)
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get a configuration value from .jvs/config.yaml.

Examples:
  jvs config get default_engine
  jvs config get default_tags
  jvs config get output_format

Available keys:
  default_engine    - Default snapshot engine
  default_tags      - Default tags (YAML list)
  output_format     - Default output format
  progress_enabled  - Progress bar setting`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()
		cfg, err := config.Load(r.Root)
		if err != nil {
			fmtErr("load config: %v", err)
			os.Exit(1)
		}

		key := args[0]
		value, err := cfg.Get(key)
		if err != nil {
			fmtErr("get config: %v", err)
			os.Exit(1)
		}

		if value == "" {
			fmt.Printf("%s (not set)\n", key)
		} else {
			// Trim trailing newlines for cleaner output
			value = strings.TrimRight(value, "\n")
			fmt.Println(value)
		}
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	rootCmd.AddCommand(configCmd)
}
