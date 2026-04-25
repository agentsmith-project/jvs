//go:build conformance

package conformance

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var checkpointListBareJQSelector = regexp.MustCompile(`jq\s+-r\s+['"]\.\[(?:\]|[0-9]+)`)

func TestDocs_PublicTerminologyContract(t *testing.T) {
	for _, doc := range stablePublicDocs() {
		t.Run(doc, func(t *testing.T) {
			path := repoFile(t, doc)
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open %s: %v", doc, err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineNo := 0
			internalSectionLevel := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Text()
				if level, ok := markdownHeadingLevel(line); ok {
					if internalSectionLevel > 0 && level <= internalSectionLevel {
						internalSectionLevel = 0
					}
					if markedInternalCompatibilityHeading(line) {
						internalSectionLevel = level
					}
				}
				if internalSectionLevel > 0 || allowedPublicDocCompatibilityLine(doc, line) {
					continue
				}
				normalizedLine := strings.ToLower(line)
				for _, forbidden := range publicDocForbiddenTerms() {
					if strings.Contains(normalizedLine, strings.ToLower(forbidden)) {
						t.Fatalf("%s:%d exposes legacy public term %q:\n%s", doc, lineNo, forbidden, line)
					}
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scan %s: %v", doc, err)
			}
		})
	}
}

func TestConformancePublicProfileUsesStableCommands(t *testing.T) {
	for _, dir := range []string{"test/conformance", "test/regression"} {
		entries, err := os.ReadDir(repoFile(t, dir))
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if strings.Contains(path, "compat") || strings.Contains(path, "legacy") {
				continue
			}
			assertTestFileUsesStableCommands(t, path)
		}
	}
}

func TestDocs_CheckpointListJSONExamplesUseEnvelope(t *testing.T) {
	for _, doc := range stablePublicDocs() {
		t.Run(doc, func(t *testing.T) {
			path := repoFile(t, doc)
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open %s: %v", doc, err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Text()
				if !strings.Contains(line, "jvs checkpoint list --json") || !strings.Contains(line, "jq") {
					continue
				}
				if checkpointListBareJQSelector.MatchString(line) {
					t.Fatalf("%s:%d treats checkpoint list JSON as a top-level array; select from .data instead:\n%s", doc, lineNo, line)
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scan %s: %v", doc, err)
			}
		})
	}
}

func stablePublicDocs() []string {
	return []string{
		"README.md",
		"docs/00_OVERVIEW.md",
		"docs/02_CLI_SPEC.md",
		"docs/11_CONFORMANCE_TEST_PLAN.md",
		"docs/12_RELEASE_POLICY.md",
		"docs/API_DOCUMENTATION.md",
		"docs/EXAMPLES.md",
		"docs/FAQ.md",
		"docs/PERFORMANCE.md",
		"docs/PERFORMANCE_RESULTS.md",
		"docs/BENCHMARKS.md",
		"docs/TROUBLESHOOTING.md",
		"docs/20_USER_SCENARIOS.md",
		"docs/agent_sandbox_quickstart.md",
		"docs/etl_pipeline_quickstart.md",
		"docs/game_dev_quickstart.md",
	}
}

func publicDocForbiddenTerms() []string {
	return []string{
		"jvs snapshot",
		"jvs worktree",
		"jvs history",
		"restore HEAD",
		"detached",
		"snapshots",
		"worktrees",
		"detached state",
		"snapshot_id",
		"worktree_name",
		"source_worktree",
		"head_snapshot",
		"latest_snapshot",
		"from_snapshot",
		"to_snapshot",
		"--paths",
		"--compress",
		"--format json",
		"--engine",
		"--keep-daily",
		"--keep-tagged",
		"--allow-protected",
		"jvs checkpoint list --tag",
		"jvs checkpoint list --grep",
		"jvs inspect",
	}
}

func markdownHeadingLevel(line string) (int, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(trimmed, "#") {
		return 0, false
	}
	level := 0
	for _, r := range trimmed {
		if r != '#' {
			break
		}
		level++
	}
	if level == 0 || level > 6 || len(trimmed) <= level || trimmed[level] != ' ' {
		return 0, false
	}
	return level, true
}

func markedInternalCompatibilityHeading(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{"legacy", "internal", "on-disk", "on disk", "compatibility"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func allowedPublicDocCompatibilityLine(doc, line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{".jvs/snapshots", ".jvs/worktrees"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if doc == "docs/BENCHMARKS.md" {
		for _, marker := range []string{"`benchmark", "internal/snapshot", "internal/restore", "internal/gc"} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	// This is the JuiceFS mount flag, not the old JVS checkpoint compression flag.
	if strings.Contains(line, "juicefs mount") && strings.Contains(line, "--compress") {
		return true
	}
	if strings.Contains(line, "HEAD") {
		for _, marker := range []string{"Git", "GitHub", "CI", "github.sha"} {
			if strings.Contains(line, marker) {
				return true
			}
		}
	}
	return false
}

func assertTestFileUsesStableCommands(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, repoFile(t, path), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	legacyCommands := map[string]string{
		"snapshot": "checkpoint",
		"history":  "checkpoint list",
		"worktree": "workspace or fork",
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isRunJVSCall(call.Fun) {
			return true
		}
		for _, arg := range call.Args {
			lit, ok := arg.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			if replacement, ok := legacyCommands[value]; ok {
				pos := fset.Position(lit.Pos())
				t.Fatalf("%s:%d public-profile test invokes legacy command %q; use %s", path, pos.Line, value, replacement)
			}
		}
		return true
	})
}

func isRunJVSCall(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	switch ident.Name {
	case "runJVS", "runJVSInRepo", "runJVSInWorktree":
		return true
	default:
		return false
	}
}

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	segments := append([]string{"..", ".."}, parts...)
	return filepath.Join(segments...)
}
