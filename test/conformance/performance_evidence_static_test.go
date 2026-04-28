//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	backtickedBenchmarkNamePattern = regexp.MustCompile("`(Benchmark[A-Za-z0-9_]+(?:/[A-Za-z0-9_]+)?)`")
	benchmarkFunctionPattern       = regexp.MustCompile(`(?m)^func (Benchmark[A-Za-z0-9_]+)\(`)
	performanceEvidenceClaim       = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(?:o\(1\)|instant(?:ly)?|constant-time|constant overhead)([^A-Za-z0-9_]|$)`)
	performanceEvidenceScope       = regexp.MustCompile(`(?i)juicefs-clone|supported\s+[^A-Za-z0-9_]*juicefs|not\b.*(?:o\(1\)|instant(?:ly)?|constant-time|constant overhead)(?:[^A-Za-z0-9_]|$)`)
	doctorJSONJQFieldPattern       = regexp.MustCompile(`jq\s+['"]\.(?:data\.)?([A-Za-z0-9_]+)`)
	unsupportedJVSVerifyFlag       = regexp.MustCompile(`(^|[^A-Za-z0-9_-])--no-payload([^A-Za-z0-9_-]|$)`)
)

func TestDocs_PerformanceResultsEvidenceRecordsModuleGoDirective(t *testing.T) {
	results := readPerformanceEvidenceRepoFile(t, "docs/PERFORMANCE_RESULTS.md")
	goDirective := moduleGoDirective(t)

	if strings.Contains(results, "go1.23.6") {
		t.Fatalf("docs/PERFORMANCE_RESULTS.md must not keep the stale go1.23.6 baseline")
	}
	_ = goDirective
	for _, required := range []string{
		"## GA Result Boundaries",
		"Required engine coverage",
		"Required benchmark package commands",
		"## Public Interpretation",
		"`juicefs-clone`",
		"`reflink-copy`",
		"`copy`",
		"./internal/snapshot",
		"./internal/restore",
		"./internal/gc",
		"./internal/worktree",
	} {
		if !strings.Contains(results, required) {
			t.Fatalf("docs/PERFORMANCE_RESULTS.md must include current performance evidence term %q", required)
		}
	}
}

func TestDocs_BenchmarkEvidenceInventoryCoversWorkspaceCreation(t *testing.T) {
	benchmarks := readPerformanceEvidenceRepoFile(t, "docs/BENCHMARKS.md")
	goDirective := moduleGoDirective(t)

	if strings.Contains(benchmarks, "go1.23.6") {
		t.Fatalf("docs/BENCHMARKS.md must not keep the stale go1.23.6 baseline")
	}
	_ = goDirective
	for _, required := range []string{
		"## Package Benchmarks",
		"## Public Interpretation",
		"## Release Evidence Use",
		"./internal/snapshot",
		"./internal/restore",
		"./internal/gc",
		"./internal/worktree",
		"save point creation",
		"workspace creation from save point",
	} {
		if !strings.Contains(benchmarks, required) {
			t.Fatalf("docs/BENCHMARKS.md must include current benchmark evidence term %q", required)
		}
	}
	if strings.Contains(strings.ToLower(benchmarks), "workspace forking performance") {
		t.Fatalf("docs/BENCHMARKS.md must not list old workspace fork benchmark language as a future opportunity")
	}
}

func TestDocs_BenchmarkInventoryUsesRealGoBenchmarkIdentifiers(t *testing.T) {
	benchmarks := readPerformanceEvidenceRepoFile(t, "docs/BENCHMARKS.md")
	known := benchmarkFunctionNames(t)

	if strings.Contains(benchmarks, "Restore from historical checkpoint") {
		t.Fatalf("docs/BENCHMARKS.md must use a real Go benchmark identifier for historical restore evidence")
	}
	for _, match := range backtickedBenchmarkNamePattern.FindAllStringSubmatch(benchmarks, -1) {
		name := strings.Split(match[1], "/")[0]
		if !known[name] {
			t.Fatalf("docs/BENCHMARKS.md references benchmark %s, but no matching Go benchmark function exists", match[1])
		}
	}
}

func TestDocs_PerformanceEvidenceScopesO1Claims(t *testing.T) {
	for _, doc := range []string{
		"docs/PERFORMANCE_RESULTS.md",
		"docs/PERFORMANCE.md",
		"docs/BENCHMARKS.md",
	} {
		t.Run(doc, func(t *testing.T) {
			previousLine := ""
			for lineNo, line := range strings.Split(readPerformanceEvidenceRepoFile(t, doc), "\n") {
				if !performanceEvidenceClaim.MatchString(line) {
					previousLine = line
					continue
				}
				if performanceEvidenceScope.MatchString(line) {
					previousLine = line
					continue
				}
				lower := strings.ToLower(line)
				previousLower := strings.ToLower(previousLine)
				if strings.Contains(lower, "not ") || strings.Contains(lower, "forbid") ||
					strings.Contains(previousLower, "not ") || strings.Contains(previousLower, "forbid") {
					previousLine = line
					continue
				}
				t.Fatalf("%s:%d has an unscoped constant-time/O(1) claim:\n%s", doc, lineNo+1, line)
			}
		})
	}
}

func TestDocs_ReleaseFacingDocsAvoidUnsupportedJVSVerifyFlags(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if unsupportedJVSVerifyFlag.MatchString(line) {
					t.Fatalf("%s:%d documents unsupported public verify flag --no-payload:\n%s", doc, lineNo, line)
				}
			})
		})
	}
}

func TestDocs_DoctorJSONExamplesUsePublicFields(t *testing.T) {
	allowedFields := make(map[string]bool)
	for _, field := range jsonFieldsForStruct(t, "internal/cli/public_json.go", "publicDoctorResult") {
		allowedFields[field] = true
	}

	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if !strings.Contains(line, "jvs doctor") || !strings.Contains(line, "--json") {
					return
				}
				if strings.Contains(line, "grep") && strings.Contains(strings.ToLower(line), "engine") {
					t.Fatalf("%s:%d treats doctor JSON as engine/capability output; use status/info/capability JSON for engine fields:\n%s", doc, lineNo, line)
				}
				match := doctorJSONJQFieldPattern.FindStringSubmatch(line)
				if match == nil {
					return
				}
				field := match[1]
				if !allowedFields[field] {
					t.Fatalf("%s:%d documents unsupported doctor JSON data field %q; public doctor data fields are %v:\n%s", doc, lineNo, field, jsonFieldsForStruct(t, "internal/cli/public_json.go", "publicDoctorResult"), line)
				}
			})
		})
	}
}

func moduleGoDirective(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(readPerformanceEvidenceRepoFile(t, "go.mod"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	t.Fatalf("go.mod must contain a go directive")
	return ""
}

func benchmarkFunctionNames(t *testing.T) map[string]bool {
	t.Helper()
	known := make(map[string]bool)
	for _, path := range []string{
		"internal/snapshot/bench_test.go",
		"internal/restore/bench_test.go",
		"internal/gc/bench_test.go",
		"internal/worktree/bench_test.go",
	} {
		body := readPerformanceEvidenceRepoFile(t, path)
		for _, match := range benchmarkFunctionPattern.FindAllStringSubmatch(body, -1) {
			known[match[1]] = true
		}
	}
	if len(known) == 0 {
		t.Fatalf("no benchmark functions found")
	}
	return known
}

func readPerformanceEvidenceRepoFile(t *testing.T, path string) string {
	t.Helper()
	for _, candidate := range []string{
		path,
		filepath.Join("..", "..", path),
	} {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return string(data)
		}
	}
	t.Fatalf("read %s from repo root or conformance package dir", path)
	return ""
}
