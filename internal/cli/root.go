package cli

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/pkg/color"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/logging"
)

var (
	jsonOutput        bool
	debugOutput       bool
	noProgress        bool
	noColor           bool
	activeCommandName string
	resolvedRepoRoot  string
	resolvedWorkspace string
	jsonErrorEmitted  bool
	rootCmd           = &cobra.Command{
		Use:   "jvs",
		Short: "JVS - Juicy Versioned Workspaces",
		Long: `JVS is a checkpoint-first, filesystem-native workspace versioning system
built on JuiceFS. It provides atomic checkpoints, workspace navigation,
and explicit dirty-workspace protection.`,
		SilenceUsage:     true,
		SilenceErrors:    true,
		PersistentPreRun: cliPersistentPreRun,
	}
)

const cliJSONSchemaVersion = 1

type cliJSONEnvelope struct {
	SchemaVersion int           `json:"schema_version"`
	Command       string        `json:"command"`
	OK            bool          `json:"ok"`
	RepoRoot      *string       `json:"repo_root"`
	Workspace     *string       `json:"workspace"`
	Data          any           `json:"data"`
	Error         *cliJSONError `json:"error"`
}

type cliJSONError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&debugOutput, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&noProgress, "no-progress", false, "disable progress bars")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output (also respects NO_COLOR env var)")
	rootCmd.PersistentFlags().StringVar(&targetRepoPath, "repo", "", "target repository root or path inside a repository")
	rootCmd.PersistentFlags().StringVar(&targetWorkspaceName, "workspace", "", "target workspace name")
}

// Execute runs the root command.
func Execute() {
	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		reportCommandErrorForCommand(cmd, err)
		os.Exit(1)
	}
}

// progressEnabled returns whether progress bars should be shown.
func progressEnabled() bool {
	return !noProgress && !jsonOutput
}

// outputJSON prints v as JSON if --json flag is set, otherwise does nothing.
func outputJSON(v any) error {
	if !jsonOutput {
		return nil
	}
	envelope := newJSONEnvelope(true, v, nil)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope)
}

func outputJSONError(err error) error {
	if !jsonOutput {
		return nil
	}
	envelope := newJSONEnvelope(false, nil, err)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope)
}

func newJSONEnvelope(ok bool, data any, err error) cliJSONEnvelope {
	repoRoot := resolvedRepoRoot
	workspace := resolvedWorkspace
	if repoRoot == "" {
		repoRoot = inferStringField(data, "repo_root")
	}
	if workspace == "" {
		workspace = inferStringField(data, "workspace")
	}
	if !ok {
		data = nil
	}

	return cliJSONEnvelope{
		SchemaVersion: cliJSONSchemaVersion,
		Command:       activeCommandName,
		OK:            ok,
		RepoRoot:      stringPtrOrNil(repoRoot),
		Workspace:     stringPtrOrNil(workspace),
		Data:          data,
		Error:         jsonErrorFromError(err),
	}
}

func jsonErrorFromError(err error) *cliJSONError {
	if err == nil {
		return nil
	}

	code := errclass.ErrUsage.Code
	message := err.Error()
	hint := ""

	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		code = jvsErr.Code
		if jvsErr.Message != "" {
			message = jvsErr.Message
		}
		hint = jvsErr.Hint
	}

	return &cliJSONError{
		Code:    code,
		Message: message,
		Hint:    hint,
	}
}

func reportCommandErrorForCommand(cmd *cobra.Command, err error) {
	recordValidationCommand(cmd)
	cliErr := commandError(err)
	if jsonOutput {
		_ = outputJSONError(cliErr)
		return
	}
	printCLIError(cliErr)
}

func recordValidationCommand(cmd *cobra.Command) {
	if activeCommandName != "" || cmd == nil {
		return
	}
	beginCLICommand(cmd)
}

func commandError(err error) error {
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return err
	}
	return errclass.ErrUsage.WithMessage(err.Error())
}

func cliPersistentPreRun(cmd *cobra.Command, args []string) {
	beginCLICommand(cmd)

	// Configure color output first (before any output)
	color.Init(noColor)

	// Configure logging based on debug flag
	if debugOutput {
		logging.SetGlobal(logging.NewLogger(logging.LevelDebug))
	}
}

func beginCLICommand(cmd *cobra.Command) {
	activeCommandName = commandName(cmd)
	resolvedRepoRoot = ""
	resolvedWorkspace = ""
	jsonErrorEmitted = false
}

func commandName(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	var names []string
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		names = append([]string{c.Name()}, names...)
	}
	return strings.Join(names, " ")
}

func recordResolvedTarget(repoRoot, workspace string) {
	if repoRoot != "" {
		resolvedRepoRoot = repoRoot
	}
	if workspace != "" {
		resolvedWorkspace = workspace
	}
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	v := value
	return &v
}

func inferStringField(data any, key string) string {
	switch v := data.(type) {
	case map[string]any:
		if s, ok := v[key].(string); ok {
			return s
		}
	case map[string]string:
		return v[key]
	}
	return ""
}
