package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/agentsmith-project/jvs/pkg/color"
)

// suggestInit provides a suggestion to initialize a repository.
func suggestInit() string {
	return "Run jvs init <name> to create a new repository."
}

func formatSuggestInitFor(file *os.File) string {
	return fmt.Sprintf("Run %s to create a new repository.", color.CodeFor(file, "jvs init <name>"))
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
