//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Conformance tests validate JVS implementation against spec requirements.
// Run with: go test -tags conformance -v ./test/conformance/...

func TestDocs_SetupAndTargetingContract(t *testing.T) {
	productPlan := readConformanceDoc(t, "PRODUCT_PLAN.md")
	cliSpec := readConformanceDoc(t, "02_CLI_SPEC.md")
	conformancePlan := readConformanceDoc(t, "11_CONFORMANCE_TEST_PLAN.md")

	for _, doc := range []struct {
		name string
		body string
	}{
		{"PRODUCT_PLAN.md", productPlan},
		{"02_CLI_SPEC.md", cliSpec},
		{"11_CONFORMANCE_TEST_PLAN.md", conformancePlan},
	} {
		for _, required := range []string{
			"path-scoped setup commands",
			"repo-free",
			"--repo",
			"assertion",
			"workspace list",
			"workspace rename",
			"workspace remove",
			"workspace path",
			"clone",
			"optimized_transfer",
			"effective_engine",
			"transfer_engine",
			"degraded_reasons",
			"warnings",
			"capabilities",
		} {
			if !strings.Contains(doc.body, required) {
				t.Fatalf("%s missing setup/targeting contract term %q", doc.name, required)
			}
		}
	}

	for _, forbidden := range []string{
		"Repo-scoped commands: `init`, `import`, `clone`, `capability`",
		"Workspace-scoped commands: `status`, `checkpoint`, `restore`, `fork`,\n`workspace path`, `workspace remove`, and `workspace rename`",
	} {
		if strings.Contains(productPlan, forbidden) {
			t.Fatalf("PRODUCT_PLAN.md still contains stale command scoping: %q", forbidden)
		}
	}
}

func readConformanceDoc(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
