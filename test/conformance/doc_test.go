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
		{"11_CONFORMANCE_TEST_PLAN.md", conformancePlan},
	} {
		for _, required := range []string{
			"CLI Help And Vocabulary",
			"JSON Envelope",
			"Setup And Status",
			"Save And History",
			"Restore Preview And Run",
			"Workspace Creation",
			"Doctor And Runtime Repair",
			"jvs save",
			"jvs history",
			"jvs view",
			"jvs restore",
			"jvs recovery",
			"jvs workspace new",
		} {
			if !strings.Contains(doc.body, required) {
				t.Fatalf("%s missing setup/targeting contract term %q", doc.name, required)
			}
		}
	}

	for _, doc := range []struct {
		name string
		body string
	}{
		{"PRODUCT_PLAN.md", productPlan},
		{"02_CLI_SPEC.md", cliSpec},
	} {
		for _, required := range []string{
			"save point",
			"jvs save",
			"jvs history",
			"jvs view",
			"jvs restore",
			"jvs recovery",
			"jvs workspace new",
			"effective_engine",
			"warnings",
		} {
			if !strings.Contains(doc.body, required) {
				t.Fatalf("%s missing current save point contract term %q", doc.name, required)
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
