package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/logging"
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
		Use:              "jvs",
		Short:            "JVS - Juicy Versioned Workspaces",
		Long:             publicRootLong,
		SilenceUsage:     true,
		SilenceErrors:    true,
		PersistentPreRun: cliPersistentPreRun,
	}
)

const cliJSONSchemaVersion = 1

const publicRootLong = `JVS keeps save points for a folder.

Start with:
  jvs init
  jvs save -m "baseline"
  jvs history
  jvs view <save> [path]
  jvs restore <save>`

var publicRootCommandNames = map[string]bool{
	"completion": true,
	"doctor":     true,
	"help":       true,
	"history":    true,
	"init":       true,
	"recovery":   true,
	"restore":    true,
	"save":       true,
	"status":     true,
	"view":       true,
}

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
	installPublicRootHelpSurface(rootCmd)
}

func installPublicRootHelpSurface(cmd *cobra.Command) {
	cmd.SetHelpFunc(func(helpCmd *cobra.Command, args []string) {
		if helpCmd == helpCmd.Root() {
			configurePublicRootHelpSurface(helpCmd)
		}
		if long := strings.TrimSpace(helpCmd.Long); long != "" {
			fmt.Fprintln(os.Stdout, long)
			fmt.Fprintln(os.Stdout)
		}
		fmt.Fprint(os.Stdout, helpCmd.UsageString())
	})
}

func configurePublicRootHelpSurface(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	cmd.Long = publicRootLong
	for _, child := range cmd.Commands() {
		child.Hidden = !publicRootCommandNames[child.Name()]
	}
}

// Execute runs the root command.
func Execute() {
	configurePublicRootHelpSurface(rootCmd)
	primeJSONOutputFromArgs(os.Args[1:])
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
		code = publicErrorCodeVocabulary(jvsErr.Code)
		if jvsErr.Message != "" {
			message = jvsErr.Message
		}
		hint = jvsErr.Hint
	}

	return &cliJSONError{
		Code:    code,
		Message: publicCLIErrorMessageVocabulary(message),
		Hint:    publicCLIErrorMessageVocabulary(hint),
	}
}

func publicCLIErrorMessageVocabulary(value string) string {
	if activeCommandName == "view" || strings.HasPrefix(activeCommandName, "view ") {
		return viewPointVocabulary(value)
	}
	if activeCommandName == "save" {
		return publicSavePointVocabulary(value)
	}
	return publicCLIErrorVocabulary(value)
}

func publicCLIErrorVocabulary(value string) string {
	var out strings.Builder
	out.Grow(len(value))

	for i := 0; i < len(value); {
		switch value[i] {
		case '\'', '"':
			next := quotedSpanEnd(value, i)
			out.WriteString(value[i:next])
			i = next
		case '(':
			if next, ok := parenthesizedPathSpanEnd(value, i); ok {
				out.WriteString(value[i:next])
				i = next
				continue
			}
			out.WriteByte(value[i])
			i++
		default:
			if isASCIISpace(value[i]) {
				out.WriteByte(value[i])
				i++
				continue
			}
			if pathEnd, ok := publicCLIErrorPathValueEnd(value, i); ok {
				out.WriteString(value[i:pathEnd])
				i = pathEnd
				continue
			}
			next := nonSpaceSpanEnd(value, i)
			out.WriteString(publicCLIErrorVocabularySpan(value[i:next]))
			i = next
		}
	}

	return out.String()
}

func quotedSpanEnd(value string, start int) int {
	quote := value[start]
	for i := start + 1; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
			continue
		}
		if value[i] == quote {
			return i + 1
		}
	}
	return len(value)
}

func parenthesizedPathSpanEnd(value string, start int) (int, bool) {
	end, ok := parenthesizedSpanEnd(value, start)
	if !ok {
		return 0, false
	}
	inner := strings.TrimSpace(value[start+1 : end-1])
	if inner == "" || !isPathLikeParenthesizedValue(inner) {
		return 0, false
	}
	return end, true
}

func parenthesizedSpanEnd(value string, start int) (int, bool) {
	depth := 0
	for i := start; i < len(value); i++ {
		switch value[i] {
		case '\'', '"':
			if i > start {
				i = quotedSpanEnd(value, i) - 1
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func isPathLikeParenthesizedValue(value string) bool {
	firstSpanEnd := nonSpaceSpanEnd(value, 0)
	return firstSpanEnd > 0 && isPathLikeSpan(value[:firstSpanEnd])
}

func nonSpaceSpanEnd(value string, start int) int {
	for i := start; i < len(value); i++ {
		if isASCIISpace(value[i]) {
			return i
		}
	}
	return len(value)
}

func isPathLikeSpan(value string) bool {
	return strings.ContainsAny(value, `/\`)
}

func publicCLIErrorPathValueEnd(value string, start int) (int, bool) {
	firstSpanEnd := nonSpaceSpanEnd(value, start)
	if firstSpanEnd == start {
		return 0, false
	}

	hasPathPrefix := hasPublicCLIErrorPathValuePrefix(value, start)
	if !hasPathPrefix && !isPathLikeSpan(value[start:firstSpanEnd]) {
		return 0, false
	}
	if firstSpanEnd == len(value) {
		return firstSpanEnd, true
	}
	if end, ok := publicCLIErrorPathValueTerminator(value, firstSpanEnd); ok {
		return end, true
	}
	if hasPathPrefix {
		return len(value), true
	}
	return firstSpanEnd, true
}

func hasPublicCLIErrorPathValuePrefix(value string, start int) bool {
	prefix := value[:start]
	for _, suffix := range []string{
		"--repo is not inside a JVS repository: ",
		"--repo resolves to ",
		"belongs to ",
		"path escapes repo root: ",
		"path is not a directory: ",
		"repository root: ",
		"repo root: ",
		"source path: ",
		"target path: ",
		"destination path: ",
	} {
		if strings.HasSuffix(prefix, suffix) {
			return true
		}
	}
	return false
}

func publicCLIErrorPathValueTerminator(value string, start int) (int, bool) {
	for i := start; i < len(value); i++ {
		if value[i] == '\n' || value[i] == '\r' {
			return i, true
		}
		if i+1 < len(value) && value[i+1] == ' ' {
			switch value[i] {
			case ',', ';', ':':
				return i, true
			}
		}
	}
	return 0, false
}

func publicCLIErrorVocabularySpan(value string) string {
	var out strings.Builder
	out.Grow(len(value))

	for i := 0; i < len(value); {
		if !isPublicVocabularyTokenByte(value[i]) {
			out.WriteByte(value[i])
			i++
			continue
		}
		start := i
		for i < len(value) && isPublicVocabularyTokenByte(value[i]) {
			i++
		}
		token := value[start:i]
		if replacement, ok := publicCLIErrorVocabularyToken(token); ok {
			out.WriteString(replacement)
		} else {
			out.WriteString(token)
		}
	}

	return out.String()
}

func publicCLIErrorVocabularyToken(token string) (string, bool) {
	switch token {
	case "head_snapshot_id", "head_snapshot":
		return "current_checkpoint", true
	case "latest_snapshot_id", "latest_snapshot":
		return "latest_checkpoint", true
	case "base_snapshot_id", "base_snapshot":
		return "base_checkpoint", true
	case "from_snapshot":
		return "from_checkpoint", true
	case "to_snapshot":
		return "to_checkpoint", true
	case "snapshot_id":
		return "checkpoint_id", true
	case "worktree_id":
		return "workspace_id", true
	}

	if strings.Contains(token, "_") {
		if replacement, ok := publicCLIErrorIdentifierVocabularyToken(token); ok {
			return replacement, true
		}
	}

	replacement, ok := publicCLIErrorSimpleVocabularyToken(token)
	if !ok {
		return "", false
	}
	return applyVocabularyTokenCase(token, replacement), true
}

func publicCLIErrorIdentifierVocabularyToken(token string) (string, bool) {
	parts := strings.Split(token, "_")
	changed := false
	for i, part := range parts {
		replacement, ok := publicCLIErrorSimpleVocabularyToken(part)
		if !ok {
			continue
		}
		parts[i] = strings.ReplaceAll(applyVocabularyTokenCase(part, replacement), " ", "_")
		changed = true
	}
	if !changed {
		return "", false
	}
	return strings.Join(parts, "_"), true
}

func publicCLIErrorSimpleVocabularyToken(token string) (string, bool) {
	switch strings.ToLower(token) {
	case "history":
		return "checkpoint list", true
	case "snapshot":
		return "checkpoint", true
	case "snapshots":
		return "checkpoints", true
	case "worktree":
		return "workspace", true
	case "worktrees":
		return "workspaces", true
	default:
		return "", false
	}
}

func applyVocabularyTokenCase(original, replacement string) string {
	if original == strings.ToUpper(original) {
		return strings.ToUpper(replacement)
	}
	if len(original) > 0 && isASCIIUpper(original[0]) && original[1:] == strings.ToLower(original[1:]) {
		return strings.ToUpper(replacement[:1]) + replacement[1:]
	}
	return replacement
}

func isPublicVocabularyTokenByte(value byte) bool {
	return value == '_' || (value >= '0' && value <= '9') || (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z')
}

func isASCIISpace(value byte) bool {
	return value == ' ' || value == '\n' || value == '\r' || value == '\t' || value == '\v' || value == '\f'
}

func isASCIIUpper(value byte) bool {
	return value >= 'A' && value <= 'Z'
}

func reportCommandErrorForCommand(cmd *cobra.Command, err error) {
	recordValidationCommand(cmd, err)
	cliErr := commandError(err)
	if jsonOutput {
		_ = outputJSONError(cliErr)
		return
	}
	if printCommandErrorSpecificMessage(cliErr) {
		return
	}
	printCLIError(cliErr)
}

func recordValidationCommand(cmd *cobra.Command, err error) {
	if activeCommandName != "" || cmd == nil {
		return
	}
	if commandName(cmd) != "" {
		beginCLICommand(cmd)
		return
	}
	if name := unknownCommandName(err); name != "" {
		activeCommandName = name
		resolvedRepoRoot = ""
		resolvedWorkspace = ""
		jsonErrorEmitted = false
	}
}

func commandError(err error) error {
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return err
	}
	return errclass.ErrUsage.WithMessage(err.Error())
}

func printCommandErrorSpecificMessage(err error) bool {
	var jvsErr *errclass.JVSError
	if !errors.As(err, &jvsErr) {
		return false
	}
	if jvsErr.Code != errclass.ErrNotRepo.Code || jvsErr.Message == "" {
		return false
	}
	if isGenericNotRepoMessage(jvsErr.Message) {
		return false
	}
	printHumanError(jvsErr.Message, jvsErr.Hint)
	return true
}

func isGenericNotRepoMessage(message string) bool {
	return message == "" || message == "not a JVS repository (or any parent)"
}

func primeJSONOutputFromArgs(args []string) {
	for _, arg := range args {
		if arg == "--" {
			return
		}
		switch {
		case arg == "--json":
			jsonOutput = true
		case strings.HasPrefix(arg, "--json="):
			value := strings.TrimPrefix(arg, "--json=")
			jsonOutput = value == "1" || strings.EqualFold(value, "true")
		}
	}
}

func unknownCommandName(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	const prefix = "unknown command "
	idx := strings.Index(msg, prefix)
	if idx < 0 {
		return ""
	}
	rest := msg[idx+len(prefix):]
	if strings.HasPrefix(rest, "\"") {
		rest = rest[1:]
		if end := strings.Index(rest, "\""); end >= 0 {
			return rest[:end]
		}
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
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
