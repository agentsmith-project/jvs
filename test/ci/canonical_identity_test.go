package ci

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const canonicalModulePath = "github.com/agentsmith-project/jvs"

func legacyModulePath() string {
	legacyProject := "jvs-" + "project"
	return "github.com/" + legacyProject + "/jvs"
}

func legacyReleaseFacingFragments() []string {
	legacyProject := "jvs-" + "project"
	return []string{
		"github.com/" + legacyProject + "/jvs",
		legacyProject + "/jvs",
		legacyProject + ".org",
	}
}

func TestCanonicalModulePathAndGoImports(t *testing.T) {
	root := repoRoot(t)

	if module := goModulePath(t, root); module != canonicalModulePath {
		t.Errorf("go.mod module path = %q, want %q", module, canonicalModulePath)
	}

	var checkedGoFiles int
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			switch name {
			case ".git", "bin", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}

		checkedGoFiles++
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if importPath == legacyModulePath() || strings.HasPrefix(importPath, legacyModulePath()+"/") {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}
				t.Errorf("%s imports legacy module path %q", rel, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk Go files: %v", err)
	}
	if checkedGoFiles == 0 {
		t.Fatalf("expected to check at least one Go file")
	}
}

func TestReleaseWorkflowUsesCanonicalRepositoryIdentity(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	workflowText := string(data)
	for _, fragment := range legacyReleaseFacingFragments() {
		if strings.Contains(workflowText, fragment) {
			t.Fatalf("CI workflow contains legacy release-facing identity fragment %q", fragment)
		}
	}

	workflow := readWorkflow(t, root)
	jobs := requireMappingValue(t, workflow, "jobs")
	release := requireMappingValue(t, jobs, "release")
	notes := requireStepNamed(t, release, "Generate release notes")
	run := scalarValue(t, requireMappingValue(t, notes, "run"))
	for _, required := range []string{
		`CERTIFICATE_IDENTITY="https://github.com/${GITHUB_REPOSITORY}/.github/workflows/ci.yml@${GITHUB_REF}"`,
		"--certificate-identity=${CERTIFICATE_IDENTITY}",
		"signing certificate identity",
	} {
		requireContains(t, run, required)
	}
}
