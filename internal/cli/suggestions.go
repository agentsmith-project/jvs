package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/agentsmith-project/jvs/pkg/color"
)

// suggestInit provides a suggestion to initialize a repository.
func suggestInit() string {
	return "Run from a JVS workspace, or use " + notRepoExplicitSelectorCommand() + "."
}

func formatSuggestInitFor(file *os.File) string {
	return fmt.Sprintf("Run from a JVS workspace, or use %s.", color.CodeFor(file, notRepoExplicitSelectorCommand()))
}

func notRepoExplicitSelectorCommand() string {
	command := strings.TrimSpace(activeCommandName)
	if command == "" {
		command = "<command>"
	}
	return "jvs --control-root <control-root> --workspace main " + command
}

// formatNotInRepositoryError formats an error when not in a JVS repository.
func formatNotInRepositoryError() string {
	return formatNotInRepositoryErrorFor(os.Stderr)
}

func formatNotInRepositoryErrorFor(file *os.File) string {
	var sb strings.Builder

	sb.WriteString(color.ErrorFor(file, "not a JVS repository (or any parent)"))
	sb.WriteString("\n")
	sb.WriteString(color.DimFor(file, "  "+formatSuggestInitFor(file)))

	return sb.String()
}
