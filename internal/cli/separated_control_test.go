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
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeparatedControlInitJSONReportsAuthoritativeFields(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")

	stdout, err := executeCommand(createTestRootCmd(),
		"init",
		"--control-root", controlRoot,
		"--payload-root", payloadRoot,
		"--workspace", "main",
		"--json",
	)
	require.NoError(t, err, stdout)

	env, data := decodeSeparatedControlDataMap(t, stdout)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, controlRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)
	assertSeparatedControlAuthoritativeData(t, data, controlRoot, payloadRoot, "main")
	assert.Equal(t, "passed", data["doctor_strict"])
	assert.NoFileExists(t, filepath.Join(payloadRoot, jvsrepo.JVSDirName))
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
			assertSeparatedControlAuthoritativeData(t, data, controlRoot, payloadRoot, "main")
			assert.Equal(t, controlRoot, data["repo"])
			assert.Equal(t, payloadRoot, data["folder"])
			assert.Equal(t, "main", data["workspace"])
		})
	}
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

func TestSeparatedControlInitJSONRejectsMixedAndUnpairedRoots(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "positional folder with separated roots",
			args: []string{"init", "folder", "--control-root", "control", "--payload-root", "payload", "--workspace", "main", "--json"},
		},
		{
			name: "control root without payload root",
			args: []string{"init", "--control-root", "control", "--workspace", "main", "--json"},
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
	assertSeparatedControlAuthoritativeData(t, data, controlRoot, payloadRoot, "main")
	assert.Equal(t, "passed", data["doctor_strict"])
	assert.Equal(t, true, data["healthy"])
	assertSeparatedDoctorChecks(t, data, map[string]string{})
}

func TestSeparatedControlDoctorStrictJSONPayloadLocatorReportsDiagnosticChecks(t *testing.T) {
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
	assertSeparatedControlAuthoritativeData(t, data, controlRoot, payloadRoot, "main")
	assert.Equal(t, "failed", data["doctor_strict"])
	assert.Equal(t, false, data["healthy"])
	assertSeparatedDoctorChecks(t, data, map[string]string{
		"payload_locator": "E_PAYLOAD_LOCATOR_PRESENT",
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
			assertSeparatedControlAuthoritativeData(t, data, controlRoot, payloadRoot, "main")
			assert.Equal(t, "failed", data["doctor_strict"])
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
			assertSeparatedControlAuthoritativeData(t, data, controlRoot, payloadRoot, "main")
			assert.Equal(t, "failed", data["doctor_strict"])
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
		"--control-root", controlRoot,
		"--payload-root", payloadRoot,
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

func assertSeparatedControlAuthoritativeData(t *testing.T, data map[string]any, controlRoot, payloadRoot, workspace string) {
	t.Helper()

	assert.Equal(t, controlRoot, data["control_root"])
	assert.Equal(t, payloadRoot, data["payload_root"])
	assert.Equal(t, "separated_control", data["repo_mode"])
	assert.Equal(t, workspace, data["workspace_name"])
	assert.Equal(t, true, data["separated_control"])
	assert.Equal(t, true, data["boundary_validated"])
	assert.Equal(t, false, data["locator_authoritative"])
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
		"payload_locator",
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
