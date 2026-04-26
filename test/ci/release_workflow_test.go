package ci

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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

func TestTagAwareValidationJobsCheckoutFullGitMetadata(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, root)
	jobs := requireMappingValue(t, workflow, "jobs")

	tagAwareCommands := []string{"make conformance", "make release-gate"}
	coveredCommands := map[string]bool{}
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		jobName := jobs.Content[i].Value
		job := jobs.Content[i+1]
		for _, command := range tagAwareCommands {
			if !jobRuns(job, command) {
				continue
			}
			coveredCommands[command] = true
			checkout := requireStepUsing(t, job, "actions/checkout@")
			requireCheckoutFetchesFullGitMetadata(t, checkout, jobName, command)
		}
	}
	for _, command := range tagAwareCommands {
		if !coveredCommands[command] {
			t.Fatalf("CI workflow must include a job that runs %q", command)
		}
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

func TestReleaseWorkflowNotesIncludeRuntimeStateBoundary(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, root)
	jobs := requireMappingValue(t, workflow, "jobs")
	release := requireMappingValue(t, jobs, "release")
	notes := requireStepNamed(t, release, "Generate release notes")
	run := scalarValue(t, requireMappingValue(t, notes, "run"))

	for _, required := range []string{
		".jvs/locks/**",
		".jvs/intents/**",
		".jvs/gc/*.json",
		"jvs doctor --strict --repair-runtime",
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
		`CERTIFICATE_IDENTITY="https://github.com/${GITHUB_REPOSITORY}/.github/workflows/ci.yml@${GITHUB_REF}"`,
		"--certificate-identity=${CERTIFICATE_IDENTITY}",
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

func TestReleaseWorkflowVerifiesArtifactsBeforeUpload(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, root)
	jobs := requireMappingValue(t, workflow, "jobs")
	release := requireMappingValue(t, jobs, "release")

	createChecksums, createChecksumsIndex := requireStepIndexNamed(t, release, "Create checksums")
	_, signChecksumsIndex := requireStepIndexNamed(t, release, "Sign checksums file")
	verify, verifyIndex := requireStepIndexNamed(t, release, "Verify release artifacts")
	upload, uploadIndex := requireStepIndexNamed(t, release, "Upload artifacts to release")
	if !(createChecksumsIndex < signChecksumsIndex && signChecksumsIndex < verifyIndex && verifyIndex < uploadIndex) {
		t.Fatalf("Verify release artifacts must run after checksum creation/signing and before upload; got create=%d sign=%d verify=%d upload=%d", createChecksumsIndex, signChecksumsIndex, verifyIndex, uploadIndex)
	}
	requireStepDoesNotBypassFailures(t, verify, "Verify release artifacts")
	requireStepDoesNotBypassFailures(t, upload, "Upload artifacts to release")

	checksummedBinaries := releaseBinariesChecksummedBySha256sum(t, scalarValue(t, requireMappingValue(t, createChecksums, "run")))
	run := scalarValue(t, requireMappingValue(t, verify, "run"))
	verifiedArtifacts := releaseArtifactsVerifiedWithTestS(t, run)
	uploadedArtifacts := releaseArtifactsUploadedByReleaseAction(t, upload)
	requireSameStringSet(t, checksummedBinaries, expectedReleaseBinaries())
	requireSameStringSet(t, verifiedArtifacts, expectedReleaseArtifacts())
	requireSameStringSet(t, uploadedArtifacts, verifiedArtifacts)
	requireSha256sumStrictCheck(t, run)
}

func TestReleaseArtifactsVerifiedWithTestSRejectsWeakeningTokens(t *testing.T) {
	for _, body := range []string{
		`test -s "$artifact" || true`,
		`test -s "${artifact}" ; true`,
		`test -s "$artifact" extra-token`,
	} {
		run := releaseArtifactTestSLoop(body)
		if artifacts, ok := releaseArtifactsVerifiedWithTestSFromScript(run); ok {
			t.Fatalf("test -s contract accepted weakened command %q with artifacts %v", body, artifacts)
		}
	}

	for _, body := range []string{
		`test -s "$artifact"`,
		`test -s "${artifact}"`,
	} {
		artifacts, ok := releaseArtifactsVerifiedWithTestSFromScript(releaseArtifactTestSLoop(body))
		if !ok {
			t.Fatalf("test -s contract rejected strict command %q", body)
		}
		requireSameStringSet(t, artifacts, []string{"jvs-linux-amd64"})
	}
}

func TestReleaseStepSuccessGateRejectsBypassControls(t *testing.T) {
	for _, tc := range []struct {
		name string
		step string
	}{
		{
			name: "composed success condition",
			step: "name: Verify release artifacts\nif: ${{ success() || true }}\nrun: test\n",
		},
		{
			name: "custom condition",
			step: "name: Upload artifacts to release\nif: github.ref != ''\nuses: softprops/action-gh-release@v2\n",
		},
		{
			name: "continue false",
			step: "name: Verify release artifacts\ncontinue-on-error: false\nrun: test\n",
		},
		{
			name: "continue true",
			step: "name: Verify release artifacts\ncontinue-on-error: true\nrun: test\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &fatalRecorder{}
			requireStepDoesNotBypassFailures(recorder, workflowStepFromYAML(t, tc.step), tc.name)
			if !recorder.failed {
				t.Fatalf("release step success gate accepted bypass control:\n%s", tc.step)
			}
		})
	}
}

func TestReleaseStepSuccessGateAllowsDefaultAndStrictSuccess(t *testing.T) {
	for _, tc := range []struct {
		name string
		step string
	}{
		{
			name: "default",
			step: "name: Verify release artifacts\nrun: test\n",
		},
		{
			name: "success function",
			step: "name: Verify release artifacts\nif: success()\nrun: test\n",
		},
		{
			name: "wrapped success function",
			step: "name: Verify release artifacts\nif: ${{ success() }}\nrun: test\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &fatalRecorder{}
			requireStepDoesNotBypassFailures(recorder, workflowStepFromYAML(t, tc.step), tc.name)
			if recorder.failed {
				t.Fatalf("release step success gate rejected strict success control: %s", recorder.message)
			}
		})
	}
}

func TestReleaseSha256sumStrictCheckRejectsWeakeningFlags(t *testing.T) {
	for _, run := range []string{
		"sha256sum --check --ignore-missing --strict SHA256SUMS",
		"sha256sum --check --strict --ignore-missing SHA256SUMS",
		"sha256sum --check --strict SHA256SUMS || true",
	} {
		if sha256sumStrictCheckCommand(run) != nil {
			t.Fatalf("sha256sum strict contract accepted weak command: %q", run)
		}
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

func TestMakefileReleaseGateRunsFuzzOrdinaryTests(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(data)

	if !targetRunsOrdinaryGoTestsForPackage(makefile, "test", "./test/fuzz/...") {
		t.Fatalf("test target must include ordinary Go tests for ./test/fuzz/...")
	}
	if !targetOrDepsRunOrdinaryGoTestsForPackage(t, makefile, "release-gate", "./test/fuzz/...") {
		t.Fatalf("release-gate must run ordinary Go tests for ./test/fuzz/... through test or an explicit dependency")
	}

	fuzzCommands := makeTargetCommands(makefile, "fuzz")
	if !commandsContain(fuzzCommands, "-run='^$$'") {
		t.Fatalf("fuzz target must keep skipping ordinary Go tests while running fuzz targets")
	}
}

func TestMakefileFuzzTargetPinsMinimizationPolicy(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(data)

	minimizeDefinition := makeVariableDefinition(t, makefile, "FUZZMINIMIZETIME")
	if !strings.HasPrefix(minimizeDefinition, "FUZZMINIMIZETIME ?=") {
		t.Fatalf("FUZZMINIMIZETIME must be configurable with ?=, got %q", minimizeDefinition)
	}
	minimizeDefault, ok := makeVariableValue(makefile, "FUZZMINIMIZETIME")
	if !ok {
		t.Fatalf("Makefile must define FUZZMINIMIZETIME")
	}
	if minimizeDefault != "0" {
		t.Fatalf("FUZZMINIMIZETIME must default to 0 for stable release fuzz smoke runs, got %q", minimizeDefault)
	}

	fuzzTimeDefinition := makeVariableDefinition(t, makefile, "FUZZTIME")
	if !strings.HasPrefix(fuzzTimeDefinition, "FUZZTIME ?=") {
		t.Fatalf("FUZZTIME must remain configurable with ?=, got %q", fuzzTimeDefinition)
	}
	fuzzTimeDefault, ok := makeVariableValue(makefile, "FUZZTIME")
	if !ok {
		t.Fatalf("Makefile must define FUZZTIME")
	}
	if _, ok := releaseFuzzCountBudget(fuzzTimeDefault); !ok {
		t.Fatalf("FUZZTIME must default to a deterministic count budget like 100x for release fuzz smoke runs, not a wall-clock duration; got %q", fuzzTimeDefault)
	}

	fuzzCommands := makeTargetCommands(makefile, "fuzz")
	if !commandsContain(fuzzCommands, "-fuzztime=$(FUZZTIME)") {
		t.Fatalf("fuzz target must run with the configured FUZZTIME budget")
	}
	if !commandsContain(fuzzCommands, "-fuzzminimizetime=$(FUZZMINIMIZETIME)") {
		t.Fatalf("fuzz target must pass the configured FUZZMINIMIZETIME to avoid default minimization extending release smoke runs")
	}
}

func TestMakefileFuzzTargetCoversReleaseFuzzers(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(data)

	sourceTargets := fuzzTargetsFromSource(t, root)
	exclusions := releaseFuzzExclusions(t, makefile)
	for _, excluded := range exclusions {
		if !stringSliceContains(sourceTargets, excluded) {
			t.Fatalf("release fuzz exclusion %s does not match a fuzz target in test/fuzz/...", excluded)
		}
		reasonKey := "RELEASE_FUZZ_EXCLUDE_REASON_" + releaseFuzzReasonID(excluded)
		reason, ok := makeVariableValue(makefile, reasonKey)
		if !ok || strings.TrimSpace(reason) == "" {
			t.Fatalf("release fuzz exclusion %s must have an audited %s entry", excluded, reasonKey)
		}
	}

	releaseTargets := makeFuzzList(t, root)
	requireSameStringSet(t, releaseTargets, releaseBlockingFuzzTargets(sourceTargets, exclusions))

	packagePattern, ok := makeVariableValue(makefile, "RELEASE_FUZZ_PACKAGE_PATTERN")
	if !ok {
		t.Fatalf("Makefile must define RELEASE_FUZZ_PACKAGE_PATTERN")
	}
	if packagePattern != "./test/fuzz/..." {
		t.Fatalf("RELEASE_FUZZ_PACKAGE_PATTERN must cover test/fuzz recursively, got %q", packagePattern)
	}
	packages, ok := makeVariableValue(makefile, "RELEASE_FUZZ_PACKAGES")
	if !ok {
		t.Fatalf("Makefile must define RELEASE_FUZZ_PACKAGES")
	}
	if !strings.Contains(packages, "go list $(RELEASE_FUZZ_PACKAGE_PATTERN)") {
		t.Fatalf("RELEASE_FUZZ_PACKAGES must discover fuzz packages with go list, got %q", packages)
	}
	allTargets, ok := makeVariableValue(makefile, "RELEASE_FUZZ_ALL_TARGETS")
	if !ok {
		t.Fatalf("Makefile must define RELEASE_FUZZ_ALL_TARGETS")
	}
	if !strings.Contains(allTargets, "$(RELEASE_FUZZ_PACKAGES)") || !strings.Contains(allTargets, "go test -list '^Fuzz'") || !strings.Contains(allTargets, "$$pkg:") {
		t.Fatalf("RELEASE_FUZZ_ALL_TARGETS must discover package-qualified fuzz targets recursively, got %q", allTargets)
	}
	filteredTargets, ok := makeVariableValue(makefile, "RELEASE_FUZZ_TARGETS")
	if !ok {
		t.Fatalf("Makefile must define RELEASE_FUZZ_TARGETS")
	}
	if !strings.Contains(filteredTargets, "$(filter-out $(RELEASE_FUZZ_EXCLUDE_TARGETS),$(RELEASE_FUZZ_ALL_TARGETS))") {
		t.Fatalf("RELEASE_FUZZ_TARGETS must filter RELEASE_FUZZ_ALL_TARGETS through RELEASE_FUZZ_EXCLUDE_TARGETS, got %q", filteredTargets)
	}

	fuzzCommands := makeTargetCommands(makefile, "fuzz")
	if commandsContain(fuzzCommands, "for target in Fuzz") {
		t.Fatalf("fuzz target must not maintain a hand-written Fuzz target list")
	}
	if !commandsContain(fuzzCommands, "$(RELEASE_FUZZ_TARGETS)") {
		t.Fatalf("fuzz target must iterate over RELEASE_FUZZ_TARGETS")
	}
	if !commandsContain(fuzzCommands, `pkg="$${entry%:*}"`) || !commandsContain(fuzzCommands, `target="$${entry##*:}"`) {
		t.Fatalf("fuzz target must split package-qualified fuzz target entries")
	}
	if !commandsContain(fuzzCommands, "-fuzz=\"^$${target}$$\"") {
		t.Fatalf("fuzz target must run each target with an exact -fuzz regexp")
	}
	fuzzParallel, ok := makeVariableValue(makefile, "FUZZPARALLEL")
	if !ok || strings.TrimSpace(fuzzParallel) == "" {
		t.Fatalf("Makefile must define FUZZPARALLEL for stable release fuzz smoke runs")
	}
	if !commandsContain(fuzzCommands, "-parallel=$(FUZZPARALLEL)") {
		t.Fatalf("fuzz target must run with the configured FUZZPARALLEL worker count")
	}
	if !commandsContain(fuzzCommands, `fuzz_cache="$$(mktemp -d)"`) || !commandsContain(fuzzCommands, `-test.fuzzcachedir="$$fuzz_cache"`) {
		t.Fatalf("fuzz target must use an isolated fuzz cache for stable release smoke runs")
	}
	if !commandsContain(fuzzCommands, `"$$pkg"`) {
		t.Fatalf("fuzz target must run each fuzz target in its owning package")
	}
}

func TestMakefileReleaseFuzzExclusionsAreRepositoryOwned(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(data)

	for _, name := range []string{
		"RELEASE_FUZZ_PACKAGE_PATTERN",
		"RELEASE_FUZZ_PACKAGES",
		"RELEASE_FUZZ_ALL_TARGETS",
		"RELEASE_FUZZ_TARGETS",
		"RELEASE_FUZZ_EXCLUDE_TARGETS",
	} {
		definition := makeVariableDefinition(t, makefile, name)
		if !strings.HasPrefix(definition, "override "+name+" :=") && !strings.HasPrefix(definition, "override "+name+" =") {
			t.Fatalf("%s must be repository-owned with override, got %q", name, definition)
		}
		if strings.Contains(definition, "?=") {
			t.Fatalf("%s must not use ?= because command-line overrides would hide release fuzz targets", name)
		}
	}

	baseline := makeFuzzList(t, root)
	if len(baseline) == 0 {
		t.Fatalf("expected at least one release fuzz target")
	}
	rootPackage := goModulePath(t, root) + "/test/fuzz"
	for _, tc := range []struct {
		name string
		args []string
		env  []string
	}{
		{
			name: "package pattern command-line override",
			args: []string{"RELEASE_FUZZ_PACKAGE_PATTERN=./test/fuzz", "MAKEFLAGS="},
		},
		{
			name: "package list command-line override",
			args: []string{"RELEASE_FUZZ_PACKAGES=" + rootPackage, "MAKEFLAGS="},
		},
		{
			name: "all target list command-line override",
			args: []string{"RELEASE_FUZZ_ALL_TARGETS=" + baseline[0], "MAKEFLAGS="},
		},
		{
			name: "filtered target list command-line override",
			args: []string{"RELEASE_FUZZ_TARGETS=" + baseline[0], "MAKEFLAGS="},
		},
		{
			name: "exclusion command-line override",
			args: []string{"RELEASE_FUZZ_EXCLUDE_TARGETS=" + baseline[0], "MAKEFLAGS="},
		},
		{
			name: "environment override",
			env:  []string{"MAKEFLAGS=-e", "RELEASE_FUZZ_TARGETS=" + baseline[0]},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withOverride := makeFuzzListWithEnvAndArgs(t, root, tc.env, tc.args...)
			requireSameStringSet(t, withOverride, baseline)
		})
	}
}

func TestMakefileFuzzListFailsClosedWhenDiscoveryFails(t *testing.T) {
	root := repoRoot(t)
	result := runMakeFuzzList(t, root, []string{"GOFLAGS=-tags=release_fuzz_discovery_failure"})
	if result.err == nil {
		t.Fatalf("make -s fuzz-list must fail when release fuzz discovery fails; stdout:\n%s\nstderr:\n%s", result.stdout, result.stderr)
	}
	if targets := fuzzTargetsFromMakeOutput(result.stdout); len(targets) != 0 {
		t.Fatalf("fuzz-list must not print partial targets after discovery failure, got %v\nstderr:\n%s", targets, result.stderr)
	}
}

func TestMakefileRuntimeSuitesUseBuiltJVSBinary(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(data)

	for _, target := range []string{"conformance", "regression"} {
		deps := makeTargetDeps(t, makefile, target)
		if !stringSliceContains(deps, "build") {
			t.Fatalf("%s target dependencies %v must include build", target, deps)
		}
		commands := makeTargetCommands(makefile, target)
		if !commandsContain(commands, `PATH="$(CURDIR)/bin:$$PATH"`) {
			t.Fatalf("%s target must put $(CURDIR)/bin first on PATH when running tests", target)
		}
		if !commandsContain(commands, "go test -tags conformance") {
			t.Fatalf("%s target must run its Go tests with the conformance tag", target)
		}
	}

	integrationDeps := makeTargetDeps(t, makefile, "integration")
	for _, want := range []string{"build", "conformance"} {
		if !stringSliceContains(integrationDeps, want) {
			t.Fatalf("integration target dependencies %v must include %s", integrationDeps, want)
		}
	}

	releaseGateDeps := makeTargetDeps(t, makefile, "release-gate")
	for _, want := range []string{"build", "conformance", "regression"} {
		if !stringSliceContains(releaseGateDeps, want) {
			t.Fatalf("release-gate target dependencies %v must include %s", releaseGateDeps, want)
		}
	}
}

type testFailureReporter interface {
	Helper()
	Fatalf(string, ...any)
}

type fatalRecorder struct {
	failed  bool
	message string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.failed = true
	r.message = fmt.Sprintf(format, args...)
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

func workflowStepFromYAML(t *testing.T, data string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(data), &doc); err != nil {
		t.Fatalf("parse workflow step: %v", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("workflow step fixture must be a mapping")
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

func scalarValue(t testFailureReporter, node *yaml.Node) string {
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
	step, _ := requireStepIndexNamed(t, job, name)
	return step
}

func requireStepIndexNamed(t *testing.T, job *yaml.Node, name string) (*yaml.Node, int) {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	if steps.Kind != yaml.SequenceNode {
		t.Fatalf("job steps must be a sequence, got kind %d", steps.Kind)
	}
	for i, step := range steps.Content {
		stepName := mappingValue(step, "name")
		if stepName != nil && stepName.Value == name {
			return step, i
		}
	}
	t.Fatalf("missing workflow step named %q", name)
	return nil, -1
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

func requireCheckoutFetchesFullGitMetadata(t *testing.T, checkout *yaml.Node, jobName, command string) {
	t.Helper()
	with := requireMappingValue(t, checkout, "with")
	fetchDepth := mappingValue(with, "fetch-depth")
	if fetchDepth == nil {
		t.Fatalf("%s job runs %q, so its checkout must set fetch-depth: 0 to fetch full history and release tags", jobName, command)
	}
	if got := scalarValue(t, fetchDepth); got != "0" {
		t.Fatalf("%s job runs %q, so its checkout must use fetch-depth: 0 to fetch full history and release tags; got %q", jobName, command, got)
	}
	if fetchTags := mappingValue(with, "fetch-tags"); fetchTags != nil && scalarValue(t, fetchTags) == "false" {
		t.Fatalf("%s job runs %q, so its checkout must not disable tag fetching", jobName, command)
	}
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

func makeTargetDeps(t *testing.T, makefile, target string) []string {
	t.Helper()
	line := makeTargetLine(t, makefile, target)
	return strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, target+":")))
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

func releaseFuzzCountBudget(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasSuffix(value, "x") {
		return 0, false
	}
	count, err := strconv.Atoi(strings.TrimSuffix(value, "x"))
	return count, err == nil && count > 0
}

func targetOrDepsRunOrdinaryGoTestsForPackage(t *testing.T, makefile, target, pkg string) bool {
	t.Helper()
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(name string) bool {
		if seen[name] {
			return false
		}
		seen[name] = true
		if targetRunsOrdinaryGoTestsForPackage(makefile, name, pkg) {
			return true
		}
		for _, dep := range makeTargetDeps(t, makefile, name) {
			if visit(dep) {
				return true
			}
		}
		return false
	}
	return visit(target)
}

func targetRunsOrdinaryGoTestsForPackage(makefile, target, pkg string) bool {
	for _, command := range makeTargetCommands(makefile, target) {
		if !strings.Contains(command, "go test") || !strings.Contains(command, pkg) {
			continue
		}
		if strings.Contains(command, "-fuzz") {
			continue
		}
		return true
	}
	return false
}

func makeVariableDefinition(t *testing.T, makefile, name string) string {
	t.Helper()
	for _, line := range strings.Split(makefile, "\n") {
		trimmed := strings.TrimSpace(stripShellComment(line))
		definition := strings.TrimSpace(strings.TrimPrefix(trimmed, "override "))
		if !strings.HasPrefix(definition, name) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(definition, name))
		for _, op := range []string{":=", "?=", "="} {
			if strings.HasPrefix(rest, op) {
				return trimmed
			}
		}
	}
	t.Fatalf("Makefile must define %s", name)
	return ""
}

func makeVariableValue(makefile, name string) (string, bool) {
	for _, line := range strings.Split(makefile, "\n") {
		trimmed := strings.TrimSpace(stripShellComment(line))
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "override "))
		if !strings.HasPrefix(trimmed, name) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, name))
		for _, op := range []string{":=", "?=", "="} {
			if strings.HasPrefix(rest, op) {
				return strings.TrimSpace(strings.TrimPrefix(rest, op)), true
			}
		}
	}
	return "", false
}

func releaseFuzzExclusions(t *testing.T, makefile string) []string {
	t.Helper()
	exclusions, ok := makeVariableValue(makefile, "RELEASE_FUZZ_EXCLUDE_TARGETS")
	if !ok {
		t.Fatalf("Makefile must define RELEASE_FUZZ_EXCLUDE_TARGETS")
	}
	return strings.Fields(exclusions)
}

func makeFuzzList(t *testing.T, root string) []string {
	t.Helper()
	return makeFuzzListWithArgs(t, root)
}

func makeFuzzListWithArgs(t *testing.T, root string, args ...string) []string {
	t.Helper()
	return makeFuzzListWithEnvAndArgs(t, root, nil, args...)
}

func makeFuzzListWithEnvAndArgs(t *testing.T, root string, env []string, args ...string) []string {
	t.Helper()
	result := runMakeFuzzList(t, root, env, args...)
	if result.err != nil {
		t.Fatalf("make -s fuzz-list failed: %v\nstdout:\n%s\nstderr:\n%s", result.err, result.stdout, result.stderr)
	}
	return fuzzTargetsFromMakeOutput(result.stdout)
}

type makeRunResult struct {
	stdout string
	stderr string
	err    error
}

func runMakeFuzzList(t *testing.T, root string, env []string, args ...string) makeRunResult {
	t.Helper()
	makeArgs := append([]string{"-s", "fuzz-list"}, args...)
	cmd := exec.Command("make", makeArgs...)
	cmd.Dir = root
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return makeRunResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}

func fuzzTargetsFromMakeOutput(output string) []string {
	var targets []string
	for _, field := range strings.Fields(output) {
		if strings.HasPrefix(field, "Fuzz") || strings.Contains(field, ":Fuzz") {
			targets = append(targets, field)
		}
	}
	sort.Strings(targets)
	return targets
}

func fuzzTargetsFromSource(t *testing.T, root string) []string {
	t.Helper()
	fset := token.NewFileSet()
	modulePath := goModulePath(t, root)
	fuzzRoot := filepath.Join(root, "test", "fuzz")
	var targets []string
	err := filepath.Walk(fuzzRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		relDir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("rel package path for %s: %w", path, err)
		}
		pkgPath := modulePath + "/" + filepath.ToSlash(relDir)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && strings.HasPrefix(fn.Name.Name, "Fuzz") {
				targets = append(targets, pkgPath+":"+fn.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk fuzz tests: %v", err)
	}
	sort.Strings(targets)
	if len(targets) == 0 {
		t.Fatalf("expected at least one fuzz target in test/fuzz/...")
	}
	return targets
}

func goModulePath(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	t.Fatalf("go.mod must define a module path")
	return ""
}

func releaseFuzzReasonID(target string) string {
	var b strings.Builder
	for _, r := range target {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func releaseBlockingFuzzTargets(targets, exclusions []string) []string {
	excluded := make(map[string]bool, len(exclusions))
	for _, target := range exclusions {
		excluded[target] = true
	}
	var releaseTargets []string
	for _, target := range targets {
		if !excluded[target] {
			releaseTargets = append(releaseTargets, target)
		}
	}
	sort.Strings(releaseTargets)
	return releaseTargets
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

func requireStepDoesNotBypassFailures(t testFailureReporter, step *yaml.Node, name string) {
	t.Helper()
	if continueOnError := mappingValue(step, "continue-on-error"); continueOnError != nil {
		t.Fatalf("%s step must not define continue-on-error; release verification must fail closed", name)
	}
	if condition := mappingValue(step, "if"); condition != nil {
		value := scalarValue(t, condition)
		if normalizeGitHubActionsExpression(value) != "success()" {
			t.Fatalf("%s step must use the default success gate or exactly success(), got %q", name, value)
		}
	}
}

func normalizeGitHubActionsExpression(value string) string {
	expr := strings.TrimSpace(value)
	if strings.HasPrefix(expr, "${{") && strings.HasSuffix(expr, "}}") {
		expr = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expr, "${{"), "}}"))
	}
	return expr
}

func expectedReleaseBinaries() []string {
	return []string{
		"jvs-linux-amd64",
		"jvs-linux-arm64",
		"jvs-darwin-amd64",
		"jvs-darwin-arm64",
		"jvs-windows-amd64.exe",
	}
}

func expectedReleaseArtifacts() []string {
	binaries := expectedReleaseBinaries()
	artifacts := make([]string, 0, len(binaries)*3+3)
	for _, binary := range binaries {
		artifacts = append(artifacts, binary, binary+".sig", binary+".pem")
	}
	return append(artifacts, "SHA256SUMS", "SHA256SUMS.sig", "SHA256SUMS.pem")
}

func releaseBinariesChecksummedBySha256sum(t *testing.T, run string) []string {
	t.Helper()
	for _, line := range shellLogicalLines(run) {
		words := shellWords(line)
		if len(words) == 0 || words[0] != "sha256sum" {
			continue
		}
		redirectIndex := indexString(words, ">")
		if redirectIndex < 0 {
			t.Fatalf("Create checksums step must redirect sha256sum output to SHA256SUMS")
		}
		if redirectIndex != len(words)-2 || words[len(words)-1] != "SHA256SUMS" {
			t.Fatalf("Create checksums step must write exactly one SHA256SUMS file, got %q", line)
		}
		binaries := words[1:redirectIndex]
		if len(binaries) == 0 {
			t.Fatalf("Create checksums step must checksum release binaries")
		}
		for _, binary := range binaries {
			if strings.ContainsAny(binary, "*?[") || strings.Contains(binary, "$") {
				t.Fatalf("Create checksums step must list explicit release binaries, got %q", binary)
			}
			if strings.Contains(binary, "/") {
				t.Fatalf("Create checksums step must checksum files from bin/ by basename, got %q", binary)
			}
		}
		return append([]string(nil), binaries...)
	}
	t.Fatalf("Create checksums step must run sha256sum over explicit release binaries")
	return nil
}

func releaseArtifactsVerifiedWithTestS(t *testing.T, run string) []string {
	t.Helper()
	artifacts, ok := releaseArtifactsVerifiedWithTestSFromScript(run)
	if ok {
		return artifacts
	}
	t.Fatalf("Verify release artifacts step must test -s every artifact from a literal for loop")
	return nil
}

func releaseArtifactsVerifiedWithTestSFromScript(run string) ([]string, bool) {
	lines := shellLogicalLines(run)
	for i, line := range lines {
		loopVariable, artifacts, hasLoop := shellForInLiteralList(line)
		if !hasLoop {
			continue
		}
		if shellWordsStartWithTestSForVariable(shellWordsAfterDo(line), loopVariable) {
			return artifacts, true
		}
		for _, bodyLine := range lines[i+1:] {
			words := shellWords(bodyLine)
			if len(words) > 0 && words[0] == "done" {
				break
			}
			if shellWordsStartWithTestSForVariable(shellWordsAfterDo(bodyLine), loopVariable) {
				return artifacts, true
			}
		}
	}
	return nil, false
}

func releaseArtifactTestSLoop(body string) string {
	return "for artifact in jvs-linux-amd64\ndo\n  " + body + "\ndone\n"
}

func releaseArtifactsUploadedByReleaseAction(t *testing.T, step *yaml.Node) []string {
	t.Helper()
	with := requireMappingValue(t, step, "with")
	files := scalarValue(t, requireMappingValue(t, with, "files"))
	var artifacts []string
	for _, entry := range releaseUploadFileEntries(files) {
		if strings.ContainsAny(entry, "*?[") || strings.Contains(entry, "$") {
			t.Fatalf("release upload files must be an explicit bin/<artifact> list, got %q", entry)
		}
		if !strings.HasPrefix(entry, "bin/") {
			t.Fatalf("release upload file %q must live under bin/", entry)
		}
		artifact := strings.TrimPrefix(entry, "bin/")
		if artifact == "" || strings.Contains(artifact, "/") {
			t.Fatalf("release upload file %q must name a single bin artifact", entry)
		}
		artifacts = append(artifacts, artifact)
	}
	if len(artifacts) == 0 {
		t.Fatalf("release upload files must list artifacts explicitly")
	}
	return artifacts
}

func releaseUploadFileEntries(files string) []string {
	var entries []string
	for _, line := range strings.Split(files, "\n") {
		entry := strings.TrimSpace(line)
		if entry == "" {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func shellLogicalLines(script string) []string {
	var lines []string
	var logical strings.Builder
	for _, raw := range strings.Split(script, "\n") {
		line := strings.TrimSpace(stripShellComment(raw))
		if line == "" {
			continue
		}
		continued := strings.HasSuffix(line, "\\")
		if continued {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\\"))
		}
		if logical.Len() > 0 {
			logical.WriteByte(' ')
		}
		logical.WriteString(line)
		if !continued {
			lines = append(lines, logical.String())
			logical.Reset()
		}
	}
	if logical.Len() > 0 {
		lines = append(lines, logical.String())
	}
	return lines
}

func stripShellComment(line string) string {
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && !inSingleQuote {
			escaped = true
			continue
		}
		switch r {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '#':
			if !inSingleQuote && !inDoubleQuote {
				return line[:i]
			}
		}
	}
	return line
}

func shellWords(line string) []string {
	var words []string
	var word strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	flush := func() {
		if word.Len() == 0 {
			return
		}
		words = append(words, word.String())
		word.Reset()
	}
	for _, r := range line {
		if escaped {
			word.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && !inSingleQuote {
			escaped = true
			continue
		}
		switch r {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
				continue
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
				continue
			}
		case ' ', '\t', ';':
			if !inSingleQuote && !inDoubleQuote {
				flush()
				continue
			}
		}
		word.WriteRune(r)
	}
	flush()
	return words
}

func shellForInLiteralList(line string) (string, []string, bool) {
	words := shellWords(line)
	if len(words) < 4 || words[0] != "for" || words[2] != "in" {
		return "", nil, false
	}
	var artifacts []string
	for _, word := range words[3:] {
		if word == "do" {
			break
		}
		artifacts = append(artifacts, word)
	}
	return words[1], artifacts, len(artifacts) > 0
}

func shellWordsAfterDo(line string) []string {
	words := shellWords(line)
	for i, word := range words {
		if word == "do" {
			return words[i+1:]
		}
	}
	return words
}

func shellWordsStartWithTestSForVariable(words []string, variable string) bool {
	if len(words) != 3 {
		return false
	}
	return words[0] == "test" && words[1] == "-s" && shellVariableReference(words[2], variable)
}

func shellVariableReference(word, variable string) bool {
	return word == "$"+variable || word == "${"+variable+"}"
}

func requireSha256sumStrictCheck(t *testing.T, run string) {
	t.Helper()
	if sha256sumStrictCheckCommand(run) != nil {
		return
	}
	t.Fatalf("Verify release artifacts step must run exactly: sha256sum --check --strict SHA256SUMS")
}

func sha256sumStrictCheckCommand(run string) []string {
	for _, line := range shellLogicalLines(run) {
		words := shellWords(line)
		if len(words) == 4 && words[0] == "sha256sum" && words[1] == "--check" && words[2] == "--strict" && words[3] == "SHA256SUMS" {
			return words
		}
	}
	return nil
}

func indexString(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

func requireSameStringSet(t *testing.T, got, want []string) {
	t.Helper()
	gotCounts := stringCounts(got)
	wantCounts := stringCounts(want)
	if len(gotCounts) != len(wantCounts) || len(got) != len(want) {
		t.Fatalf("verified artifacts mismatch\nmissing: %v\nextra: %v\ngot: %v\nwant: %v", missingStrings(wantCounts, gotCounts), missingStrings(gotCounts, wantCounts), sortedStrings(got), sortedStrings(want))
	}
	for value, count := range wantCounts {
		if gotCounts[value] != count {
			t.Fatalf("verified artifacts mismatch\nmissing: %v\nextra: %v\ngot: %v\nwant: %v", missingStrings(wantCounts, gotCounts), missingStrings(gotCounts, wantCounts), sortedStrings(got), sortedStrings(want))
		}
	}
}

func stringCounts(values []string) map[string]int {
	counts := make(map[string]int, len(values))
	for _, value := range values {
		counts[value]++
	}
	return counts
}

func missingStrings(want, got map[string]int) []string {
	var missing []string
	for value, count := range want {
		for i := got[value]; i < count; i++ {
			missing = append(missing, value)
		}
	}
	sort.Strings(missing)
	return missing
}

func sortedStrings(values []string) []string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return sorted
}
