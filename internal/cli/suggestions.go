package cli

import (
	"fmt"
	"strings"

	"github.com/agentsmith-project/jvs/pkg/color"
)

// suggestInit provides a suggestion to initialize a repository.
func suggestInit() string {
	return fmt.Sprintf("Run %s to create a new repository.", color.Code("jvs init <name>"))
}

// formatNotInRepositoryError formats an error when not in a JVS repository.
func formatNotInRepositoryError() string {
	var sb strings.Builder

	sb.WriteString(color.Error("not a JVS repository (or any parent)"))
	sb.WriteString("\n")
	sb.WriteString(color.Dim("  " + suggestInit()))

	return sb.String()
}
