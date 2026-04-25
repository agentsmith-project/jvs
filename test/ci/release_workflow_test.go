package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseWorkflowRequiresReleaseGate(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, root)

	on := requireMappingValue(t, workflow, "on")
	push := requireMappingValue(t, on, "push")
	tags := requireMappingValue(t, push, "tags")
	if !nodeContainsScalar(tags, "v*") {
		t.Fatalf("push.tags must include v* release tags")
	}
	if mappingValue(on, "workflow_dispatch") == nil {
		t.Fatalf("workflow_dispatch must be available for manual release runs")
	}

	jobs := requireMappingValue(t, workflow, "jobs")
	releaseGate := requireMappingValue(t, jobs, "release-gate")
	releaseGateIf := scalarValue(t, requireMappingValue(t, releaseGate, "if"))
	requireContains(t, releaseGateIf, "startsWith(github.ref, 'refs/tags/v')")
	requireContains(t, releaseGateIf, "github.event_name == 'workflow_dispatch'")
	if !jobRuns(releaseGate, "make release-gate") {
		t.Fatalf("release-gate job must run make release-gate")
	}

	release := requireMappingValue(t, jobs, "release")
	needs := requireMappingValue(t, release, "needs")
	if !nodeContainsScalar(needs, "release-gate") {
		t.Fatalf("release job must need release-gate")
	}
	releaseIf := scalarValue(t, requireMappingValue(t, release, "if"))
	requireContains(t, releaseIf, "startsWith(github.ref, 'refs/tags/v')")
	requireContains(t, releaseIf, "github.event_name == 'workflow_dispatch'")
}

func TestManualReleaseBindsToTagRef(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, root)
	jobs := requireMappingValue(t, workflow, "jobs")

	releaseGate := requireMappingValue(t, jobs, "release-gate")
	validation := requireStepNamed(t, releaseGate, "Validate manual release tag")
	validationIf := scalarValue(t, requireMappingValue(t, validation, "if"))
	if validationIf != "github.event_name == 'workflow_dispatch'" {
		t.Fatalf("manual tag validation must only run for workflow_dispatch, got %q", validationIf)
	}
	env := requireMappingValue(t, validation, "env")
	releaseTag := scalarValue(t, requireMappingValue(t, env, "RELEASE_TAG"))
	if releaseTag != "${{ github.event.inputs.tag }}" {
		t.Fatalf("manual tag validation must bind RELEASE_TAG to the workflow input, got %q", releaseTag)
	}
	validationRun := scalarValue(t, requireMappingValue(t, validation, "run"))
	requireContains(t, validationRun, "git check-ref-format \"refs/tags/$RELEASE_TAG\"")
	requireContains(t, validationRun, "ls-remote --exit-code --refs")
	requireContains(t, validationRun, "refs/tags/$RELEASE_TAG")

	release := requireMappingValue(t, jobs, "release")
	releasePathJobs := scalarSequenceValues(t, requireMappingValue(t, release, "needs"))
	releasePathJobs = append(releasePathJobs, "release")
	for _, jobName := range releasePathJobs {
		job := requireMappingValue(t, jobs, jobName)
		checkout := requireStepUsing(t, job, "actions/checkout@")
		with := requireMappingValue(t, checkout, "with")
		ref := scalarValue(t, requireMappingValue(t, with, "ref"))
		requireContains(t, ref, "format('refs/tags/{0}', github.event.inputs.tag)")
	}

	publish := requireStepUsing(t, release, "softprops/action-gh-release@")
	with := requireMappingValue(t, publish, "with")
	tagName := scalarValue(t, requireMappingValue(t, with, "tag_name"))
	requireContains(t, tagName, "github.event.inputs.tag")
	if strings.Contains(tagName, "refs/tags/") {
		t.Fatalf("release action tag_name must publish the exact tag name, not a full ref: %q", tagName)
	}
}

func TestWorkflowSetupGoUsesModuleVersion(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, root)
	jobs := requireMappingValue(t, workflow, "jobs")

	setupGoSteps := 0
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		job := jobs.Content[i+1]
		steps := requireMappingValue(t, job, "steps")
		for _, step := range steps.Content {
			uses := mappingValue(step, "uses")
			if uses == nil || !strings.HasPrefix(uses.Value, "actions/setup-go@") {
				continue
			}
			setupGoSteps++
			with := requireMappingValue(t, step, "with")
			if mappingValue(with, "go-version") != nil {
				t.Fatalf("setup-go steps must use go-version-file instead of hard-coded go-version")
			}
			versionFile := requireMappingValue(t, with, "go-version-file")
			if versionFile.Value != "go.mod" {
				t.Fatalf("setup-go must read the Go version from go.mod, got %q", versionFile.Value)
			}
		}
	}
	if setupGoSteps == 0 {
		t.Fatalf("workflow must configure Go with actions/setup-go")
	}
}

func TestWorkflowUsesPinnedLintTools(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	workflowText := string(data)
	if strings.Contains(workflowText, "github.com/golangci/golangci-lint/cmd/golangci-lint@latest") {
		t.Fatalf("CI workflow must not install golangci-lint with @latest")
	}

	workflow := readWorkflow(t, root)
	jobs := requireMappingValue(t, workflow, "jobs")
	for _, jobName := range []string{"lint", "release-gate"} {
		job := requireMappingValue(t, jobs, jobName)
		if !jobRuns(job, "make tools") {
			t.Fatalf("%s job must install pinned tools through make tools", jobName)
		}
	}
}

func TestReleaseWorkflowNotesIncludeReadinessSections(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, root)
	jobs := requireMappingValue(t, workflow, "jobs")
	release := requireMappingValue(t, jobs, "release")
	notes := requireStepNamed(t, release, "Generate release notes")
	run := scalarValue(t, requireMappingValue(t, notes, "run"))

	for _, required := range []string{
		"## Known limitations",
		"## Risk labels",
		"## Migration notes",
	} {
		requireContains(t, run, required)
	}
}

func TestReleaseWorkflowNotesUseSigningGuideAndGASections(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, root)
	jobs := requireMappingValue(t, workflow, "jobs")
	release := requireMappingValue(t, jobs, "release")
	notes := requireStepNamed(t, release, "Generate release notes")
	run := scalarValue(t, requireMappingValue(t, notes, "run"))

	for _, required := range []string{
		"docs/SIGNING.md",
		"cosign verify-blob jvs-linux-amd64",
		"--signature jvs-linux-amd64.sig",
		"--certificate jvs-linux-amd64.pem",
		"SHA256SUMS.sig",
		"SHA256SUMS.pem",
		"--certificate-identity=https://github.com/jvs-project/jvs/.github/workflows/ci.yml@${GITHUB_REF}",
		"--certificate-oidc-issuer=https://token.actions.githubusercontent.com",
		"signing certificate identity",
		"workflow_dispatch",
		"refs/tags",
		"## Known limitations",
		"remote push/pull",
		"signing commands",
		"partial checkpoint contracts",
		"compression contracts",
		"merge/rebase",
		"complex retention policy flags",
		"## Risk labels",
		"integrity",
		"migration",
		"## Migration notes",
		"jvs doctor --strict --repair-runtime",
	} {
		requireContains(t, run, required)
	}
}

func TestMakefilePinsGolangCILintTooling(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(data)

	if !strings.Contains(makefile, "GOLANGCI_LINT_VERSION ?= v1.64.8") {
		t.Fatalf("Makefile must expose pinned GOLANGCI_LINT_VERSION v1.64.8")
	}
	if !strings.Contains(makefile, "GOLANGCI_LINT_PACKAGE := github.com/golangci/golangci-lint/cmd/golangci-lint") {
		t.Fatalf("Makefile must expose the golangci-lint package path")
	}
	if strings.Contains(makefile, "github.com/golangci/golangci-lint/cmd/golangci-lint@latest") {
		t.Fatalf("Makefile must not install golangci-lint with @latest")
	}
	if !strings.Contains(makeTargetLine(t, makefile, "tools"), "tools:") {
		t.Fatalf("Makefile must expose a tools target")
	}

	toolsCommands := makeTargetCommands(makefile, "tools")
	if !commandsContain(toolsCommands, "go install $(GOLANGCI_LINT_PACKAGE)@$(GOLANGCI_LINT_VERSION)") {
		t.Fatalf("tools target must install golangci-lint using the pinned Makefile version")
	}

	lintCommands := makeTargetCommands(makefile, "lint")
	if !commandsContain(lintCommands, "command -v golangci-lint") {
		t.Fatalf("lint target must first locate golangci-lint with command -v")
	}
	if !commandsContain(lintCommands, "go env GOPATH") {
		t.Fatalf("lint target must fall back to GOPATH/bin when golangci-lint is not on PATH")
	}
	if !commandsContain(lintCommands, "--version") || !commandsContain(lintCommands, "version $(GOLANGCI_LINT_VERSION)") {
		t.Fatalf("lint target must verify the golangci-lint binary version before running it")
	}
	if !commandsContain(lintCommands, "go install $(GOLANGCI_LINT_PACKAGE)@$(GOLANGCI_LINT_VERSION)") {
		t.Fatalf("lint target must install the pinned golangci-lint binary when no matching binary is available")
	}
}

func TestMakefileReleaseGateRunsCIContract(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(data)

	ciContractCommands := makeTargetCommands(makefile, "ci-contract")
	if !commandsContain(ciContractCommands, "go test -count=1 ./test/ci/...") {
		t.Fatalf("ci-contract target must run the CI workflow contract tests")
	}

	docsContractCommands := makeTargetCommands(makefile, "docs-contract")
	if !commandsContain(docsContractCommands, "-run 'TestDocs_|TestConformancePublicProfileUsesStableCommands'") {
		t.Fatalf("docs-contract target must run all docs contract tests")
	}

	releaseGateLine := makeTargetLine(t, makefile, "release-gate")
	deps := strings.Fields(strings.TrimSpace(strings.TrimPrefix(releaseGateLine, "release-gate:")))
	for _, want := range []string{"tools", "ci-contract", "test-race", "test-cover", "lint", "build", "conformance", "library", "regression", "fuzz"} {
		if !stringSliceContains(deps, want) {
			t.Fatalf("release-gate target dependencies %v must include %s", deps, want)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, ".github", "workflows", "ci.yml")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repository root from %s", dir)
		}
		dir = parent
	}
}

func readWorkflow(t *testing.T, root string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse CI workflow: %v", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("CI workflow root must be a mapping")
	}
	return doc.Content[0]
}

func requireMappingValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value := mappingValue(node, key)
	if value == nil {
		t.Fatalf("missing YAML mapping key %q", key)
	}
	return value
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func scalarValue(t *testing.T, node *yaml.Node) string {
	t.Helper()
	if node.Kind != yaml.ScalarNode {
		t.Fatalf("expected scalar YAML value, got kind %d", node.Kind)
	}
	return node.Value
}

func nodeContainsScalar(node *yaml.Node, want string) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value == want
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if item.Kind == yaml.ScalarNode && item.Value == want {
				return true
			}
		}
	}
	return false
}

func scalarSequenceValues(t *testing.T, node *yaml.Node) []string {
	t.Helper()
	if node.Kind != yaml.SequenceNode {
		t.Fatalf("expected sequence YAML value, got kind %d", node.Kind)
	}
	values := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			t.Fatalf("expected scalar sequence item, got kind %d", item.Kind)
		}
		values = append(values, item.Value)
	}
	return values
}

func jobRuns(job *yaml.Node, want string) bool {
	steps := mappingValue(job, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return false
	}
	for _, step := range steps.Content {
		run := mappingValue(step, "run")
		if run != nil && strings.Contains(run.Value, want) {
			return true
		}
	}
	return false
}

func requireStepNamed(t *testing.T, job *yaml.Node, name string) *yaml.Node {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	if steps.Kind != yaml.SequenceNode {
		t.Fatalf("job steps must be a sequence, got kind %d", steps.Kind)
	}
	for _, step := range steps.Content {
		stepName := mappingValue(step, "name")
		if stepName != nil && stepName.Value == name {
			return step
		}
	}
	t.Fatalf("missing workflow step named %q", name)
	return nil
}

func requireStepUsing(t *testing.T, job *yaml.Node, usesPrefix string) *yaml.Node {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	if steps.Kind != yaml.SequenceNode {
		t.Fatalf("job steps must be a sequence, got kind %d", steps.Kind)
	}
	for _, step := range steps.Content {
		uses := mappingValue(step, "uses")
		if uses != nil && strings.HasPrefix(uses.Value, usesPrefix) {
			return step
		}
	}
	t.Fatalf("missing workflow step using %q", usesPrefix)
	return nil
}

func makeTargetLine(t *testing.T, makefile, target string) string {
	t.Helper()
	prefix := target + ":"
	for _, line := range strings.Split(makefile, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("missing Makefile target %s", target)
	return ""
}

func makeTargetCommands(makefile, target string) []string {
	prefix := target + ":"
	lines := strings.Split(makefile, "\n")
	var commands []string
	inTarget := false
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, prefix):
			inTarget = true
		case inTarget && strings.HasPrefix(line, "\t"):
			commands = append(commands, strings.TrimSpace(line))
		case inTarget && line != "":
			return commands
		}
	}
	return commands
}

func commandsContain(commands []string, want string) bool {
	for _, command := range commands {
		if strings.Contains(command, want) {
			return true
		}
	}
	return false
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func requireContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("value %q must contain %q", got, want)
	}
}
