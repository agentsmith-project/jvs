package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/pkg/errclass"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for JVS.

To load completions for your shell:

Bash:
  # To load completions for each session, execute once:
  # Linux:
  jvs completion bash > /etc/bash_completion.d/jvs
  # macOS:
  jvs completion bash > /usr/local/etc/bash_completion.d/jvs

  # Or add to your ~/.bashrc or ~/.bash_profile:
  source <(jvs completion bash)

Zsh:
  # To load completions for each session, execute once:
  jvs completion zsh > "${fpath[1]}/_jvs"

  # Or add to your ~/.zshrc:
  source <(jvs completion zsh)

  # You may need to force rebuild the completion cache:
  rm -f ~/.zcompdump
  compinit

Fish:
  # To load completions for each session, execute once:
  jvs completion fish > ~/.config/fish/completions/jvs.fish

  # Or add to your ~/.config/fish/config.fish:
  jvs completion fish | source

PowerShell:
  # To load completions for each session, run:
  jvs completion powershell | Out-String | Invoke-Expression

  # Or add to your PowerShell profile:
  # (Microsoft.PowerShell_profile.ps1 or profile.ps1)
  jvs completion powershell | Out-String | Invoke-Expression`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput {
			return errclass.ErrUsage.
				WithMessage("--json is not supported for completion script output").
				WithHint("Run jvs completion <shell> without --json.")
		}

		shell := args[0]

		var err error
		switch shell {
		case "bash":
			err = cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			err = cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			err = cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			err = cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			err = fmt.Errorf("unsupported shell type: %s", shell)
		}

		if err != nil {
			return fmt.Errorf("failed to generate completion for %s: %w", shell, err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
