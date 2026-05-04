package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/recovery"
	jvsrepo "github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeparatedControlInitJSONReportsFolderAndControlData(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")

	stdout, err := executeCommand(createTestRootCmd(),
		"init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "main",
		"--json",
	)
	require.NoError(t, err, stdout)

	env, data := decodeSeparatedControlDataMap(t, stdout)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, controlRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)
	assertExternalControlDataShape(t, data, controlRoot, payloadRoot, "main")
	assert.NoFileExists(t, filepath.Join(payloadRoot, jvsrepo.JVSDirName))
}

func TestSeparatedControlInitAdoptsExistingNonEmptyFolderAndCanSave(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", string(model.EngineCopy))
	base := setupSeparatedControlCLICWD(t)
	emptyBin := filepath.Join(base, "empty-bin")
	require.NoError(t, os.Mkdir(emptyBin, 0755))
	t.Setenv("PATH", emptyBin)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	require.NoError(t, os.MkdirAll(payloadRoot, 0755))
	userFile := filepath.Join(payloadRoot, "README.md")
	require.NoError(t, os.WriteFile(userFile, []byte("existing user file\n"), 0644))
	require.NoError(t, os.Chmod(userFile, 0640))
	originalMTime := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	require.NoError(t, os.Chtimes(userFile, originalMTime, originalMTime))
	before, err := os.Stat(userFile)
	require.NoError(t, err)

	stdout, err := executeCommand(createTestRootCmd(),
		"init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "main",
		"--json",
	)
	require.NoError(t, err, stdout)

	env, data := decodeSeparatedControlDataMap(t, stdout)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, controlRoot, *env.RepoRoot)
	assertExternalControlDataShape(t, data, controlRoot, payloadRoot, "main")
	assert.Equal(t, controlRoot, data["repo_root"])
	assert.NoFileExists(t, filepath.Join(payloadRoot, jvsrepo.JVSDirName))
	assert.FileExists(t, userFile)
	after, err := os.Stat(userFile)
	require.NoError(t, err)
	assert.Equal(t, before.Mode(), after.Mode())
	assert.Equal(t, before.ModTime(), after.ModTime())
	assertSeparatedInitSetupFields(t, data, payloadRoot)
	assertSeparatedWarningsInclude(t, data["warnings"], "juicefs command not found")

	statusOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"status",
	)
	require.NoError(t, err, statusOut)
	_, statusData := decodeSeparatedControlDataMap(t, statusOut)
	assert.Equal(t, true, statusData["unsaved_changes"])
	assert.Equal(t, "not_saved", statusData["files_state"])

	saveOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save",
		"-m", "baseline",
	)
	require.NoError(t, err, saveOut)
	_, saveData := decodeSeparatedControlDataMap(t, saveOut)
	savePointID, ok := saveData["save_point_id"].(string)
	require.True(t, ok, "save should expose save_point_id: %#v", saveData)
	require.NotEmpty(t, savePointID)
	savedFile := filepath.Join(controlRoot, ".jvs", "snapshots", savePointID, "README.md")
	savedContent, err := os.ReadFile(savedFile)
	require.NoError(t, err)
	assert.Equal(t, "existing user file\n", string(savedContent))

	statusOut, err = executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"status",
	)
	require.NoError(t, err, statusOut)
	_, statusData = decodeSeparatedControlDataMap(t, statusOut)
	assert.Equal(t, false, statusData["unsaved_changes"])
	assert.Equal(t, "matches_save_point", statusData["files_state"])
	assert.Equal(t, savePointID, statusData["newest_save_point"])
}

func TestSeparatedControlInitRejectsPayloadSymlinkEscapeBeforeControlData(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	outsideRoot := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(payloadRoot, 0755))
	require.NoError(t, os.MkdirAll(outsideRoot, 0755))
	outsideFile := filepath.Join(outsideRoot, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("outside\n"), 0644))
	if err := os.Symlink(outsideFile, filepath.Join(payloadRoot, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "main",
		"--json",
	)

	requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrPathBoundaryEscape.Code)
	assert.NoFileExists(t, filepath.Join(controlRoot, ".jvs"))
	assert.NoFileExists(t, controlRoot)
	assert.FileExists(t, outsideFile)
}

func TestSeparatedControlInitHumanUsesFolderAndControlDataLanguage(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	emptyBin := filepath.Join(base, "empty-bin")
	require.NoError(t, os.Mkdir(emptyBin, 0755))
	t.Setenv("PATH", emptyBin)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")

	stdout, err := executeCommand(createTestRootCmd(),
		"init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "main",
	)
	require.NoError(t, err, stdout)

	assert.Contains(t, stdout, "JVS is ready for this folder.")
	assert.Contains(t, stdout, "Folder: "+payloadRoot)
	assert.Contains(t, stdout, "Control data: "+controlRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Capabilities: write=")
	assert.Contains(t, stdout, "recommended=")
	assert.Contains(t, stdout, "Warning: juicefs command not found")
	assert.NotContains(t, stdout, "separated control")
	assert.NotContains(t, stdout, "Payload root")
	assert.NotContains(t, stdout, "Control root")
}

func TestSeparatedControlInitWithoutFolderUsesCurrentDirectory(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(t.TempDir(), "control")

	stdout, err := executeCommand(createTestRootCmd(),
		"init",
		"--control-root", controlRoot,
		"--workspace", "main",
		"--json",
	)
	require.NoError(t, err, stdout)

	_, data := decodeSeparatedControlDataMap(t, stdout)
	assertExternalControlDataShape(t, data, controlRoot, base, "main")
	assert.NoFileExists(t, filepath.Join(base, jvsrepo.JVSDirName))
}

func TestSeparatedControlStatusJSONUsesControlRootFromCleanAndOtherRepoCWD(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")

	cleanCWD := filepath.Join(base, "clean")
	require.NoError(t, os.MkdirAll(cleanCWD, 0755))
	otherRepo := initLegacyRepoForCLITest(t, filepath.Join(base, "other-repo"))

	for _, cwd := range []string{cleanCWD, otherRepo} {
		t.Run(filepath.Base(cwd), func(t *testing.T) {
			require.NoError(t, os.Chdir(cwd))
			stdout, err := executeCommand(createTestRootCmd(),
				"--json",
				"--control-root", controlRoot,
				"--workspace", "main",
				"status",
			)
			require.NoError(t, err, stdout)

			env, data := decodeSeparatedControlDataMap(t, stdout)
			require.NotNil(t, env.RepoRoot)
			assert.Equal(t, controlRoot, *env.RepoRoot)
			require.NotNil(t, env.Workspace)
			assert.Equal(t, "main", *env.Workspace)
			assertExternalControlDataShape(t, data, controlRoot, payloadRoot, "main")
			assert.NotContains(t, data, "repo")
			assert.Equal(t, payloadRoot, data["folder"])
			assert.Equal(t, "main", data["workspace"])
		})
	}
}

func TestSeparatedControlStatusHumanUsesControlDataInsteadOfRepo(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")

	stdout, err := executeCommand(createTestRootCmd(),
		"--control-root", controlRoot,
		"--workspace", "main",
		"status",
	)
	require.NoError(t, err, stdout)

	assert.Contains(t, stdout, "Control data: "+controlRoot)
	assert.Contains(t, stdout, "Folder: "+payloadRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.NotContains(t, stdout, "Repo: "+controlRoot)
}

func TestSeparatedControlRepoFlagRequiresControlRootWorkspaceSelector(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")

	cloneTarget := filepath.Join(base, "clone-target")
	for _, tc := range []struct {
		name        string
		commandHint string
		args        []string
	}{
		{
			name:        "status",
			commandHint: "status",
			args:        []string{"--json", "--repo", controlRoot, "status"},
		},
		{
			name:        "status with workspace",
			commandHint: "status",
			args:        []string{"--json", "--repo", controlRoot, "--workspace", "main", "status"},
		},
		{
			name:        "save",
			commandHint: "save",
			args:        []string{"--json", "--repo", controlRoot, "save", "-m", "blocked"},
		},
		{
			name:        "doctor",
			commandHint: "doctor",
			args:        []string{"--json", "--repo", controlRoot, "doctor", "--strict"},
		},
		{
			name:        "repo clone",
			commandHint: "repo clone",
			args:        []string{"--json", "--repo", controlRoot, "repo", "clone", cloneTarget, "--save-points", "main"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, base, tc.args...)
			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrExplicitTargetRequired.Code)
			assert.Contains(t, env.Error.Message, "control data is outside the folder")
			assert.NotContains(t, env.Error.Message, "separated-control")
			assertSeparatedSelectorHint(t, env, controlRoot, tc.commandHint)
		})
	}
	assert.NoDirExists(t, cloneTarget)
}

func TestSeparatedControlAmbientControlRootRequiresExplicitSelector(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")

	nestedCWD := filepath.Join(controlRoot, "nested")
	require.NoError(t, os.MkdirAll(nestedCWD, 0755))
	stdout, stderr, exitCode := runContractSubprocess(t, nestedCWD, "--json", "status")

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrExplicitTargetRequired.Code)
	assert.Contains(t, env.Error.Message, "control data is outside the folder")
	assert.NotContains(t, env.Error.Message, "separated-control")
	assertSeparatedSelectorHint(t, env, controlRoot, "status")
}

func TestSeparatedControlPayloadRootNakedStatusJSONHintDoesNotSuggestInit(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")

	stdout, stderr, exitCode := runContractSubprocess(t, payloadRoot, "--json", "status")

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrNotRepo.Code)
	require.NotNil(t, env.Error)
	assert.NotContains(t, env.Error.Hint, "jvs init")
	assert.NotContains(t, env.Error.Message, "jvs init")
}

func TestSeparatedControlMissingWorkspaceHintIncludesFullSelectorShape(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"status",
	)

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrExplicitTargetRequired.Code)
	assertSeparatedSelectorHint(t, env, controlRoot, "status")
}

func TestSeparatedControlRuntimeSelectorRejectsNonMainWorkspace(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))

	for _, tc := range []struct {
		name    string
		command string
		args    []string
	}{
		{
			name:    "status",
			command: "status",
			args:    []string{"--json", "--control-root", controlRoot, "--workspace", "feature", "status"},
		},
		{
			name:    "save",
			command: "save",
			args:    []string{"--json", "--control-root", controlRoot, "--workspace", "feature", "save", "-m", "blocked"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, base, tc.args...)
			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrWorkspaceMismatch.Code)
			assertSeparatedSelectorHint(t, env, controlRoot, tc.command)
		})
	}

	historyOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"history",
	)
	require.NoError(t, err, historyOut)
	_, history := decodeSeparatedControlDataMap(t, historyOut)
	assert.Empty(t, historySavePointIDsForSeparatedOpsTest(t, history, "save_points"))
}

func TestSeparatedControlStatusJSONPayloadLocatorErrorHasNullData(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, jvsrepo.JVSDirName), []byte("untrusted\n"), 0644))

	cleanCWD := filepath.Join(base, "clean")
	require.NoError(t, os.MkdirAll(cleanCWD, 0755))
	stdout, stderr, exitCode := runContractSubprocess(t, cleanCWD,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"status",
	)
	require.Equal(t, 1, exitCode, "status unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_PAYLOAD_LOCATOR_PRESENT", env.Error.Code)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestSeparatedControlStatusRejectsPayloadSymlinkEscapeBeforeBoundaryValidatedJSON(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	seedSeparatedControlMetadataSentinels(t, controlRoot)
	if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), filepath.Join(payloadRoot, "control-link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"status",
	)

	requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrPathBoundaryEscape.Code)
}

func TestSeparatedControlRecoveryStatusRejectsPayloadSymlinkEscape(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(planID string) []string
	}{
		{
			name: "list",
			args: func(string) []string {
				return []string{"recovery", "status"}
			},
		},
		{
			name: "detail",
			args: func(planID string) []string {
				return []string{"recovery", "status", planID}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := setupSeparatedControlCLICWD(t)
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
			seedSeparatedControlMetadataSentinels(t, controlRoot)
			seedSeparatedControlRecoveryPlanFixture(t, base, controlRoot, payloadRoot)
			if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), filepath.Join(payloadRoot, "control-link")); err != nil {
				t.Skipf("symlinks not supported: %v", err)
			}

			args := []string{"--json", "--control-root", controlRoot, "--workspace", "main"}
			args = append(args, tc.args("RP-separated-active")...)
			stdout, stderr, exitCode := runContractSubprocess(t, base, args...)

			requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrPathBoundaryEscape.Code)
		})
	}
}

func TestSeparatedControlRecoveryStatusRejectsPlanPayloadBindingDrift(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "list", args: []string{"recovery", "status"}},
		{name: "detail", args: []string{"recovery", "status", "RP-separated-active"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := setupSeparatedControlCLICWD(t)
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
			seedSeparatedControlRecoveryPlanFixture(t, base, controlRoot, payloadRoot)

			driftPayloadRoot := filepath.Join(base, "payload-drift")
			require.NoError(t, os.MkdirAll(driftPayloadRoot, 0755))
			cfg, err := jvsrepo.LoadWorktreeConfig(controlRoot, "main")
			require.NoError(t, err)
			cfg.RealPath = driftPayloadRoot
			require.NoError(t, jvsrepo.WriteWorktreeConfig(controlRoot, "main", cfg))

			args := []string{"--json", "--control-root", controlRoot, "--workspace", "main"}
			args = append(args, tc.args...)
			stdout, stderr, exitCode := runContractSubprocess(t, base, args...)

			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrPathBoundaryEscape.Code)
			assert.NotContains(t, env.Error.Message, "payload root")
			assert.Contains(t, env.Error.Message, "workspace folder")
			assert.Contains(t, env.Error.Message, "control data")
		})
	}
}

func TestSeparatedControlInitJSONRejectsHiddenPayloadAliasMisuse(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "positional folder with hidden payload-root alias",
			args: []string{"init", "folder", "--control-root", "control", "--payload-root", "payload", "--workspace", "main", "--json"},
		},
		{
			name: "payload root without control root",
			args: []string{"init", "--payload-root", "payload", "--workspace", "main", "--json"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := setupSeparatedControlCLICWD(t)
			stdout, stderr, exitCode := runContractSubprocess(t, base, tc.args...)
			require.Equal(t, 1, exitCode, "init unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeContractEnvelope(t, stdout)
			assert.False(t, env.OK)
			require.NotNil(t, env.Error)
			assert.Equal(t, "E_USAGE", env.Error.Code)
			assert.JSONEq(t, `null`, string(env.Data))
		})
	}
}

func TestSeparatedControlInitRejectsNonMainWorkspaceWithoutMutation(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "feature",
	)

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrWorkspaceMismatch.Code)
	assert.Contains(t, env.Error.Hint, "--workspace main")
	assert.NoDirExists(t, controlRoot)
	assert.NoDirExists(t, payloadRoot)
}

func TestSeparatedControlDoctorStrictJSONIncludesChecks(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")

	stdout, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"doctor",
		"--strict",
	)
	require.NoError(t, err, stdout)

	_, data := decodeSeparatedControlDataMap(t, stdout)
	assertExternalControlDataShape(t, data, controlRoot, payloadRoot, "main")
	assert.Equal(t, true, data["healthy"])
	assertSeparatedDoctorChecks(t, data, map[string]string{})
}

func TestSeparatedControlDoctorUnsupportedVariantsFailClosed(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")

	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "non strict json",
			args: []string{"--json", "--control-root", controlRoot, "--workspace", "main", "doctor"},
		},
		{
			name: "repair runtime",
			args: []string{"--json", "--control-root", controlRoot, "--workspace", "main", "doctor", "--strict", "--repair-runtime"},
		},
		{
			name: "repair list",
			args: []string{"--json", "--control-root", controlRoot, "--workspace", "main", "doctor", "--strict", "--repair-list"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, base, tc.args...)
			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrExplicitTargetRequired.Code)
			assert.Contains(t, env.Error.Message, "external control root")
			assert.NotContains(t, env.Error.Message, "separated-control")
			assertSeparatedSelectorHint(t, env, controlRoot, "doctor --strict --json")
		})
	}

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--control-root", controlRoot,
		"--workspace", "main",
		"doctor",
		"--strict",
	)
	require.Equal(t, 1, exitCode, "doctor unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, "doctor --strict --json")
	assert.Contains(t, stderr, "--control-root "+controlRoot)
	assert.Contains(t, stderr, "--workspace main")
}

func TestSeparatedControlDoctorStrictJSONWorkspaceControlMarkerReportsDiagnosticChecks(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, jvsrepo.JVSDirName), []byte("untrusted\n"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"doctor",
		"--strict",
	)
	require.Equal(t, 1, exitCode, "doctor unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK, stdout)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	assertExternalControlDataShape(t, data, controlRoot, payloadRoot, "main")
	assert.Equal(t, false, data["healthy"])
	assertSeparatedDoctorChecks(t, data, map[string]string{
		"workspace_control_marker": "E_PAYLOAD_LOCATOR_PRESENT",
	})
}

func TestSeparatedControlDoctorStrictJSONActiveOperationFixturesFail(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T, base, controlRoot, payloadRoot string)
	}{
		{
			name: "intent",
			seed: func(t *testing.T, base, controlRoot, payloadRoot string) {
				t.Helper()
				seedSeparatedControlIntentFixture(t, controlRoot)
			},
		},
		{
			name: "cleanup plan",
			seed: func(t *testing.T, base, controlRoot, payloadRoot string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("cleanup candidate\n"), 0644))
				saveOut, err := executeCommand(createTestRootCmd(),
					"--json",
					"--control-root", controlRoot,
					"--workspace", "main",
					"save", "-m", "cleanup source",
				)
				require.NoError(t, err, saveOut)
				_, err = executeCommand(createTestRootCmd(),
					"--json",
					"--control-root", controlRoot,
					"--workspace", "main",
					"cleanup", "preview",
				)
				require.NoError(t, err)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := setupSeparatedControlCLICWD(t)
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
			tc.seed(t, base, controlRoot, payloadRoot)

			stdout, stderr, exitCode := runContractSubprocess(t, base,
				"--json",
				"--control-root", controlRoot,
				"--workspace", "main",
				"doctor", "--strict",
			)
			require.Equal(t, 1, exitCode, "doctor unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeContractEnvelope(t, stdout)
			assert.True(t, env.OK, stdout)
			var data map[string]any
			require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
			assertExternalControlDataShape(t, data, controlRoot, payloadRoot, "main")
			assert.Equal(t, false, data["healthy"])
			assertSeparatedDoctorChecks(t, data, map[string]string{
				"active_operation": "E_ACTIVE_OPERATION_BLOCKING",
			})
		})
	}
}

func TestSeparatedControlDoctorStrictJSONRecoveryStateFixturesFail(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T, base, controlRoot, payloadRoot string)
	}{
		{
			name: "restore plan",
			seed: func(t *testing.T, base, controlRoot, payloadRoot string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))
				saveOut, err := executeCommand(createTestRootCmd(),
					"--json",
					"--control-root", controlRoot,
					"--workspace", "main",
					"save", "-m", "restore source",
				)
				require.NoError(t, err, saveOut)
				_, saveData := decodeSeparatedControlDataMap(t, saveOut)
				savePointID, _ := saveData["save_point_id"].(string)
				require.NotEmpty(t, savePointID)
				_, err = executeCommand(createTestRootCmd(),
					"--json",
					"--control-root", controlRoot,
					"--workspace", "main",
					"restore", savePointID,
				)
				require.NoError(t, err)
			},
		},
		{
			name: "active recovery plan",
			seed: func(t *testing.T, base, controlRoot, payloadRoot string) {
				t.Helper()
				seedSeparatedControlRecoveryPlanFixture(t, base, controlRoot, payloadRoot)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := setupSeparatedControlCLICWD(t)
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
			tc.seed(t, base, controlRoot, payloadRoot)

			stdout, stderr, exitCode := runContractSubprocess(t, base,
				"--json",
				"--control-root", controlRoot,
				"--workspace", "main",
				"doctor", "--strict",
			)
			require.Equal(t, 1, exitCode, "doctor unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeContractEnvelope(t, stdout)
			assert.True(t, env.OK, stdout)
			var data map[string]any
			require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
			assertExternalControlDataShape(t, data, controlRoot, payloadRoot, "main")
			assert.Equal(t, false, data["healthy"])
			assertSeparatedDoctorChecks(t, data, map[string]string{
				"recovery_state": "E_RECOVERY_BLOCKING",
			})
		})
	}
}

func setupSeparatedControlCLICWD(t *testing.T) string {
	t.Helper()

	base := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(base))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWd))
	})
	return base
}

func initSeparatedControlForCLITest(t *testing.T, controlRoot, payloadRoot, workspace string) {
	t.Helper()

	stdout, err := executeCommand(createTestRootCmd(),
		"init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", workspace,
		"--json",
	)
	require.NoError(t, err, stdout)
}

func decodeSeparatedControlDataMap(t *testing.T, stdout string) (contractEnvelope, map[string]any) {
	t.Helper()

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	return env, data
}

func assertExternalControlDataShape(t *testing.T, data map[string]any, controlRoot, folder, workspace string) {
	t.Helper()

	assert.Equal(t, controlRoot, data["control_root"])
	assert.Equal(t, folder, data["folder"])
	assert.Equal(t, workspace, data["workspace"])
	assertNoExternalControlImplementationFields(t, data)
}

func assertSeparatedInitSetupFields(t *testing.T, data map[string]any, folder string) {
	t.Helper()

	capabilities, ok := data["capabilities"].(map[string]any)
	require.True(t, ok, "init should expose capabilities: %#v", data["capabilities"])
	assert.Equal(t, folder, capabilities["target_path"])
	assert.Equal(t, true, capabilities["write_probe"])
	assert.Equal(t, capabilities["recommended_engine"], data["effective_engine"])
	assert.NotEmpty(t, data["effective_engine"])
	assert.NotEmpty(t, data["metadata_preservation"])
	assert.NotEmpty(t, data["performance_class"])
	require.IsType(t, []any{}, data["warnings"])
}

func assertSeparatedWarningsInclude(t *testing.T, raw any, want string) {
	t.Helper()

	warnings, ok := raw.([]any)
	require.True(t, ok, "warnings should be an array: %#v", raw)
	for _, item := range warnings {
		if item == want {
			return
		}
	}
	require.Contains(t, warnings, want)
}

func assertNoExternalControlImplementationFields(t *testing.T, data map[string]any) {
	t.Helper()

	for _, key := range []string{
		"repo_mode",
		"separated_control",
		"payload_root",
		"workspace_name",
		"locator_authoritative",
		"doctor_strict",
		"boundary_validated",
	} {
		assert.NotContains(t, data, key)
	}
}

func seedSeparatedControlIntentFixture(t *testing.T, controlRoot string) {
	t.Helper()

	intent := map[string]any{
		"snapshot_id":   "1708300800000-deadbeef",
		"worktree_name": "main",
		"started_at":    time.Now().UTC(),
		"engine":        "copy",
	}
	data, err := json.MarshalIndent(intent, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "intents", "1708300800000-deadbeef.json"), data, 0644))
}

func seedSeparatedControlRecoveryPlanFixture(t *testing.T, base, controlRoot, payloadRoot string) {
	t.Helper()

	r, err := jvsrepo.OpenControlRoot(controlRoot)
	require.NoError(t, err)
	now := time.Now().UTC()
	plan := &recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 "RP-separated-active",
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 payloadRoot,
		SourceSavePoint:        model.SnapshotID("1708300800000-deadbeef"),
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: "main", RealPath: payloadRoot},
		Backup:                 recovery.Backup{Path: filepath.Join(base, "backup"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		Phase:                  recovery.PhasePending,
		RecommendedNextCommand: "jvs recovery status RP-separated-active",
	}
	require.NoError(t, recovery.NewManager(controlRoot).Write(plan))
}

func assertSeparatedDoctorChecks(t *testing.T, data map[string]any, failed map[string]string) {
	t.Helper()

	raw, ok := data["checks"].([]any)
	require.True(t, ok, "checks should be an array: %#v", data["checks"])
	byName := map[string]map[string]any{}
	for _, item := range raw {
		check, ok := item.(map[string]any)
		require.True(t, ok, "check should be an object: %#v", item)
		name, ok := check["name"].(string)
		require.True(t, ok, "check name should be a string: %#v", check)
		byName[name] = check
	}

	for _, name := range []string{
		"root_overlap",
		"workspace_control_marker",
		"repo_identity",
		"workspace_binding",
		"path_boundary",
		"permissions",
		"active_operation",
		"recovery_state",
	} {
		check, ok := byName[name]
		require.True(t, ok, "missing doctor check %q in %#v", name, byName)
		require.NotEmpty(t, check["message"], "check %q should include a message", name)
		if code, failed := failed[name]; failed {
			assert.Equal(t, "failed", check["status"])
			assert.Equal(t, code, check["error_code"])
			continue
		}
		assert.Equal(t, "passed", check["status"])
		assert.Nil(t, check["error_code"])
	}
}

func requireSeparatedControlCLIJSONError(t *testing.T, stdout, stderr string, exitCode int, wantCode string) contractEnvelope {
	t.Helper()
	require.Equal(t, 1, exitCode, "command unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK, stdout)
	require.NotNil(t, env.Error)
	assert.Equal(t, wantCode, env.Error.Code)
	assert.JSONEq(t, `null`, string(env.Data))
	return env
}

func assertSeparatedSelectorHint(t *testing.T, env contractEnvelope, controlRoot, command string) {
	t.Helper()
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Hint, "--control-root "+controlRoot)
	assert.Contains(t, env.Error.Hint, "--workspace main")
	assert.Contains(t, env.Error.Hint, command)
}
