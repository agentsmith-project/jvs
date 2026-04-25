package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	conformanceProfile string
	conformanceVerbose bool
)

var conformanceCmd = &cobra.Command{
	Use:    "conformance",
	Short:  "Conformance test commands",
	Hidden: true,
}

var conformanceRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run conformance tests",
	Long: `Run conformance tests to verify implementation compliance.

This command runs the conformance test suite defined in the specification.
Tests validate that the implementation correctly follows the JVS specification.

Profiles:
  - dev: Development profile, fast execution (default)
  - full: Full test suite including slow tests
  - ci: CI profile with strict output formatting`,
	Run: func(cmd *cobra.Command, args []string) {
		// Find the repository root (where go.mod is)
		repoRoot, err := findRepoRoot()
		if err != nil {
			fmtErr("find repo root: %v", err)
			os.Exit(1)
		}

		// Build test command
		testArgs := []string{
			"test",
			"-v",
			"-tags=conformance",
			"./test/conformance/...",
		}

		// Add profile-specific flags
		switch conformanceProfile {
		case "full":
			testArgs = append(testArgs, "-run=.")
		case "ci":
			testArgs = append(testArgs, "-json")
		default: // dev
			testArgs = append(testArgs, "-short")
		}

		if conformanceVerbose {
			testArgs = append(testArgs, "-v")
		}

		// Run tests
		testCmd := exec.Command("go", testArgs...)
		testCmd.Dir = repoRoot
		testCmd.Stdout = os.Stdout
		testCmd.Stderr = os.Stderr
		testCmd.Env = append(os.Environ(), "JVS_CONFORMANCE_PROFILE="+conformanceProfile)

		fmt.Printf("Running conformance tests (profile: %s)...\n", conformanceProfile)
		fmt.Printf("Command: go %s\n\n", strings.Join(testArgs, " "))

		if err := testCmd.Run(); err != nil {
			fmtErr("conformance tests failed: %v", err)
			os.Exit(1)
		}

		fmt.Println("\nConformance tests passed.")
	},
}

var conformanceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List conformance tests",
	Long: `List all conformance tests defined in the specification.

Shows test IDs, descriptions, and specification references.`,
	Run: func(cmd *cobra.Command, args []string) {
		repoRoot, err := findRepoRoot()
		if err != nil {
			fmtErr("find repo root: %v", err)
			os.Exit(1)
		}

		// List test files
		testDir := filepath.Join(repoRoot, "test", "conformance")
		entries, err := os.ReadDir(testDir)
		if err != nil {
			fmtErr("read conformance test directory: %v", err)
			os.Exit(1)
		}

		fmt.Println("Conformance Tests:")
		fmt.Println("==================")
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), "_test.go") {
				fmt.Printf("  - %s\n", strings.TrimSuffix(entry.Name(), "_test.go"))
			}
		}
		fmt.Println("\nRun 'jvs conformance run' to execute all tests.")
	},
}

func findRepoRoot() (string, error) {
	// Start from current directory and walk up looking for go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("go.mod not found")
}

func init() {
	conformanceRunCmd.Flags().StringVarP(&conformanceProfile, "profile", "p", "dev", "test profile (dev, full, ci)")
	conformanceRunCmd.Flags().BoolVarP(&conformanceVerbose, "verbose", "v", false, "verbose output")
	conformanceCmd.AddCommand(conformanceRunCmd)
	conformanceCmd.AddCommand(conformanceListCmd)
	rootCmd.AddCommand(conformanceCmd)
}
