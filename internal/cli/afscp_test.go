package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type afscpDirectEnvelope struct {
	Contract string            `json:"contract"`
	Command  string            `json:"command"`
	OK       bool              `json:"ok"`
	Status   string            `json:"status"`
	Data     json.RawMessage   `json:"data"`
	Error    *afscpDirectError `json:"error"`
}

type afscpDirectError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func TestAFSCPDirectExitCodesMapStableErrors(t *testing.T) {
	isolateContractCLIState(t)
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, exitCode := runContractSubprocess(
		t,
		base,
		"afscp",
		"--control-root", controlRoot,
		"status",
		"--json",
	)
	require.Equal(t, 2, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	env := decodeAFSCPDirectEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "JVS_INVALID_ARGUMENT", env.Error.Code)
	assert.NotContains(t, stdout, controlRoot)
	assert.NotContains(t, stdout, home)
	assert.Empty(t, strings.TrimSpace(stderr))

	stdout, stderr, exitCode = runContractSubprocess(
		t,
		base,
		"afscp",
		"--control-root", controlRoot,
		"--home", home,
		"save",
		"--message", "baseline",
		"--json",
	)
	require.Equal(t, 5, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	env = decodeAFSCPDirectEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "failed", env.Status)
	require.NotNil(t, env.Error)
	assert.Equal(t, "JVS_CLONE_UNAVAILABLE", env.Error.Code)
	assert.NotContains(t, stdout, controlRoot)
	assert.NotContains(t, stdout, home)
	assert.Empty(t, strings.TrimSpace(stderr))
}

func TestAFSCPDirectListStatusDoctorSkeletonsReturnJSON(t *testing.T) {
	isolateContractCLIState(t)
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))

	for _, command := range []string{"list", "status", "doctor"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(
				t,
				base,
				"afscp",
				"--control-root", controlRoot,
				"--home", home,
				command,
				"--json",
			)
			require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeAFSCPDirectEnvelope(t, stdout)
			assert.Equal(t, command, env.Command)
			assert.True(t, env.OK)
			assert.Equal(t, "succeeded", env.Status)
			assert.JSONEq(t, `null`, string(mustMarshalForAFSCPTest(t, env.Error)))
			assert.NotEqual(t, "null", strings.TrimSpace(string(env.Data)))
			assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)
		})
	}
}

func TestAFSCPDirectStatusAndDoctorAfterInitExposeStableShape(t *testing.T) {
	isolateContractCLIState(t)
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")

	initOut, initStderr, initExitCode := runContractSubprocess(
		t,
		base,
		"init",
		home,
		"--control-root", controlRoot,
		"--workspace", "main",
		"--json",
	)
	require.Equal(t, 0, initExitCode, "stdout=%s stderr=%s", initOut, initStderr)
	assert.Empty(t, strings.TrimSpace(initStderr))
	_, initData := decodeSeparatedControlDataMap(t, initOut)
	repoID, ok := initData["repo_id"].(string)
	require.True(t, ok, "init response should expose repo_id: %#v", initData)
	require.NotEmpty(t, repoID)

	for _, command := range []string{"status", "doctor"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(
				t,
				base,
				"afscp",
				"--control-root", controlRoot,
				"--home", home,
				command,
				"--json",
			)
			require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeAFSCPDirectEnvelope(t, stdout)
			assert.True(t, env.OK)
			assert.Equal(t, command, env.Command)
			var data map[string]any
			require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
			assert.Equal(t, repoID, data["repo_id"])
			assert.Equal(t, "uninitialized", data["metadata_state"])
			assert.Equal(t, "none", data["recovery"])
			if command == "status" {
				assert.Equal(t, "none", data["active_operation"])
			} else {
				assert.Equal(t, true, data["healthy"])
				assert.Equal(t, []any{}, data["findings"])
				assert.Equal(t, "clean", data["journal"])
			}
			assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)
		})
	}
}

func TestAFSCPDirectSaveAndListPublishMetadataJSON(t *testing.T) {
	isolateContractCLIState(t)
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
	installAFSCPFakeJuiceFSClone(t)

	stdout, stderr, exitCode := runContractSubprocess(
		t,
		base,
		"afscp",
		"--control-root", controlRoot,
		"--home", home,
		"save",
		"--message", "baseline",
		"--json",
	)
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	saveEnv := decodeAFSCPDirectEnvelope(t, stdout)
	assert.True(t, saveEnv.OK)
	assert.Equal(t, "save", saveEnv.Command)
	var saveData map[string]any
	require.NoError(t, json.Unmarshal(saveEnv.Data, &saveData), stdout)
	savePointID, ok := saveData["save_point_id"].(string)
	require.True(t, ok, "save response should expose save_point_id: %#v", saveData)
	require.NotEmpty(t, savePointID)
	assert.Equal(t, savePointID, saveData["history_head"])
	assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)

	stdout, stderr, exitCode = runContractSubprocess(
		t,
		base,
		"afscp",
		"--control-root", controlRoot,
		"--home", home,
		"list",
		"--json",
	)
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	listEnv := decodeAFSCPDirectEnvelope(t, stdout)
	assert.True(t, listEnv.OK)
	assert.Equal(t, "list", listEnv.Command)
	var listData map[string]any
	require.NoError(t, json.Unmarshal(listEnv.Data, &listData), stdout)
	assert.Equal(t, savePointID, listData["history_head"])
	require.Len(t, listData["save_points"], 1)
	assert.Equal(t, "ready", listData["metadata_state"])
	assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)
}

func TestAFSCPDirectSaveAcceptsSelectorFlagsBeforeAndAfterCommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(controlRoot, home string) []string
	}{
		{
			name: "command before selector flags",
			args: func(controlRoot, home string) []string {
				return []string{
					"afscp",
					"save",
					"--control-root", controlRoot,
					"--home", home,
					"--message", "baseline",
					"--json",
				}
			},
		},
		{
			name: "selector flags before command",
			args: func(controlRoot, home string) []string {
				return []string{
					"afscp",
					"--control-root", controlRoot,
					"--home", home,
					"save",
					"--message", "baseline",
					"--json",
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateContractCLIState(t)
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			home := filepath.Join(base, "home")
			require.NoError(t, os.Mkdir(controlRoot, 0755))
			require.NoError(t, os.Mkdir(home, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
			installAFSCPFakeJuiceFSClone(t)

			stdout, stderr, exitCode := runContractSubprocess(t, base, tc.args(controlRoot, home)...)
			require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))
			env := decodeAFSCPDirectEnvelope(t, stdout)
			assert.True(t, env.OK)
			assert.Equal(t, "save", env.Command)
			var saveData map[string]any
			require.NoError(t, json.Unmarshal(env.Data, &saveData), stdout)
			savePointID, ok := saveData["save_point_id"].(string)
			require.True(t, ok, "save response should expose save_point_id: %#v", saveData)
			assert.NotEmpty(t, savePointID)
			assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)
		})
	}
}

func TestAFSCPDirectCommandsRequireSelectorPairAndJSON(t *testing.T) {
	for _, command := range []string{"save", "list", "restore", "status", "doctor"} {
		t.Run(command, func(t *testing.T) {
			isolateContractCLIState(t)
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			home := filepath.Join(base, "home")
			require.NoError(t, os.Mkdir(controlRoot, 0755))
			require.NoError(t, os.Mkdir(home, 0755))

			args := afscpDirectCommandArgs(command, controlRoot, home, true)
			stdout, stderr, exitCode := runContractSubprocess(t, base, args...)
			assert.Equal(t, 2, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stdout))
			assert.Contains(t, stderr, "afscp direct commands require --json")

			missingControl := afscpDirectCommandArgs(command, "", home, false)
			stdout, stderr, exitCode = runContractSubprocess(t, base, missingControl...)
			assert.Equal(t, 2, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))
			assertAFSCPDirectInvalidSelectorEnvelope(t, stdout, command)
			assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)

			missingHome := afscpDirectCommandArgs(command, controlRoot, "", false)
			stdout, stderr, exitCode = runContractSubprocess(t, base, missingHome...)
			assert.Equal(t, 2, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))
			assertAFSCPDirectInvalidSelectorEnvelope(t, stdout, command)
			assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)
		})
	}
}

func TestAFSCPDirectMetadataCommandsRejectHomeContainingJVSMetadata(t *testing.T) {
	for _, command := range []string{"status", "doctor"} {
		t.Run(command, func(t *testing.T) {
			isolateContractCLIState(t)
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			home := filepath.Join(base, "home")
			require.NoError(t, os.Mkdir(controlRoot, 0755))
			require.NoError(t, os.MkdirAll(filepath.Join(home, ".jvs"), 0755))

			stdout, stderr, exitCode := runContractSubprocess(t, base, afscpDirectCommandArgs(command, controlRoot, home, false)...)
			require.Equal(t, 2, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))
			assertAFSCPDirectInvalidSelectorEnvelope(t, stdout, command)
			assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)
		})
	}
}

func TestAFSCPDirectRestoreReturnsJSONAndUpdatesHistoryHead(t *testing.T) {
	isolateContractCLIState(t)
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("saved"), 0644))
	installAFSCPFakeJuiceFSClone(t)

	stdout, stderr, exitCode := runContractSubprocess(
		t,
		base,
		"afscp",
		"--control-root", controlRoot,
		"--home", home,
		"save",
		"--message", "baseline",
		"--json",
	)
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	saveEnv := decodeAFSCPDirectEnvelope(t, stdout)
	var saveData map[string]any
	require.NoError(t, json.Unmarshal(saveEnv.Data, &saveData), stdout)
	savePointID, ok := saveData["save_point_id"].(string)
	require.True(t, ok, "save response should expose save_point_id: %#v", saveData)
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("dirty"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(home, "post-save.txt"), []byte("remove"), 0644))

	stdout, stderr, exitCode = runContractSubprocess(
		t,
		base,
		"afscp",
		"--control-root", controlRoot,
		"--home", home,
		"restore",
		"--save-point", savePointID,
		"--json",
	)
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	restoreEnv := decodeAFSCPDirectEnvelope(t, stdout)
	assert.True(t, restoreEnv.OK)
	assert.Equal(t, "restore", restoreEnv.Command)
	var restoreData map[string]any
	require.NoError(t, json.Unmarshal(restoreEnv.Data, &restoreData), stdout)
	assert.Equal(t, savePointID, restoreData["restored_save_point_id"])
	assert.Equal(t, savePointID, restoreData["new_head"])
	assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)
	assert.Equal(t, "saved", string(mustReadAFSCPTestFile(t, filepath.Join(home, "profile.txt"))))
	assert.NoFileExists(t, filepath.Join(home, "post-save.txt"))

	stdout, stderr, exitCode = runContractSubprocess(
		t,
		base,
		"afscp",
		"--control-root", controlRoot,
		"--home", home,
		"list",
		"--json",
	)
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	listEnv := decodeAFSCPDirectEnvelope(t, stdout)
	var listData map[string]any
	require.NoError(t, json.Unmarshal(listEnv.Data, &listData), stdout)
	assert.Equal(t, savePointID, listData["history_head"])
}

func TestAFSCPDirectGoldenJSONShapesOmitPathsRawCommandAndLegacyFields(t *testing.T) {
	isolateContractCLIState(t)
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("saved"), 0644))
	installAFSCPFakeJuiceFSClone(t)

	stdout, stderr, exitCode := runContractSubprocess(
		t,
		base,
		"afscp",
		"--control-root", controlRoot,
		"--home", home,
		"save",
		"--message", "baseline",
		"--json",
	)
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	assertAFSCPDirectEnvelopeShape(t, stdout, "save", []string{"save_point_id", "created_at", "message", "history_head"})
	saveEnv := decodeAFSCPDirectEnvelope(t, stdout)
	var saveData map[string]any
	require.NoError(t, json.Unmarshal(saveEnv.Data, &saveData), stdout)
	savePointID, ok := saveData["save_point_id"].(string)
	require.True(t, ok, "save response should expose save_point_id: %#v", saveData)
	assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)

	for _, command := range []string{"list", "status", "doctor"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, base, afscpDirectCommandArgs(command, controlRoot, home, false)...)
			require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))
			assertAFSCPDirectEnvelopeShape(t, stdout, command, afscpDirectExpectedDataKeys(command))
			assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)
		})
	}

	stdout, stderr, exitCode = runContractSubprocess(
		t,
		base,
		"afscp",
		"--control-root", controlRoot,
		"--home", home,
		"restore",
		"--save-point", savePointID,
		"--json",
	)
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	assertAFSCPDirectEnvelopeShape(t, stdout, "restore", []string{"restored_save_point_id", "previous_head", "new_head"})
	assertAFSCPDirectJSONDoesNotLeakSelector(t, stdout, controlRoot, home)
}

func TestAFSCPDirectHelpIsNotPublicUserSurface(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "--help")
	require.NoError(t, err)
	assertRootHelpOmitsWord(t, stdout, "afscp")

	stdout, err = executeCommand(createTestRootCmd(), "afscp", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "jvs afscp")
	assert.Contains(t, stdout, "internal")
}

func afscpDirectCommandArgs(command, controlRoot, home string, omitJSON bool) []string {
	args := []string{"afscp"}
	if controlRoot != "" {
		args = append(args, "--control-root", controlRoot)
	}
	if home != "" {
		args = append(args, "--home", home)
	}
	args = append(args, command)
	switch command {
	case "save":
		args = append(args, "--message", "baseline")
	case "restore":
		args = append(args, "--save-point", "missing-save-point")
	}
	if !omitJSON {
		args = append(args, "--json")
	}
	return args
}

func assertAFSCPDirectInvalidSelectorEnvelope(t *testing.T, stdout, command string) {
	t.Helper()

	env := decodeAFSCPDirectEnvelope(t, stdout)
	assert.Equal(t, command, env.Command)
	assert.False(t, env.OK)
	assert.Equal(t, "failed", env.Status)
	require.NotNil(t, env.Error)
	assert.Equal(t, "JVS_INVALID_ARGUMENT", env.Error.Code)
}

func assertAFSCPDirectEnvelopeShape(t *testing.T, stdout, command string, dataKeys []string) {
	t.Helper()

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw), stdout)
	assert.ElementsMatch(t, []string{"contract", "command", "ok", "status", "data", "error"}, mapKeys(raw))
	assert.NotContains(t, raw, "raw_command")
	assert.NotContains(t, raw, "argv")

	env := decodeAFSCPDirectEnvelope(t, stdout)
	assert.Equal(t, command, env.Command)
	assert.True(t, env.OK)
	assert.Equal(t, "succeeded", env.Status)
	assert.JSONEq(t, `null`, string(mustMarshalForAFSCPTest(t, env.Error)))

	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	assert.ElementsMatch(t, dataKeys, mapKeysAny(data))
	assertAFSCPDirectMapDoesNotContainLegacyFields(t, data)
}

func afscpDirectExpectedDataKeys(command string) []string {
	switch command {
	case "list":
		return []string{"history_head", "save_points", "metadata_state"}
	case "status":
		return []string{"repo_id", "history_head", "active_operation", "metadata_state", "recovery"}
	case "doctor":
		return []string{"repo_id", "healthy", "findings", "metadata_state", "journal", "recovery"}
	default:
		return nil
	}
}

func assertAFSCPDirectMapDoesNotContainLegacyFields(t *testing.T, data map[string]any) {
	t.Helper()

	for _, key := range mapKeysAny(data) {
		lowerKey := strings.ToLower(key)
		for _, forbidden := range []string{
			"path",
			"command",
			"raw_command",
			"argv",
			"payload_root_hash",
			"content_root_hash",
			"save_profile",
			"expected_folder_evidence",
			"restore_plan",
			"plan_id",
			"run_command",
			"capacity",
		} {
			assert.NotContains(t, lowerKey, forbidden)
		}
	}
}

func mapKeys(raw map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	return keys
}

func mapKeysAny(raw map[string]any) []string {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	return keys
}

func decodeAFSCPDirectEnvelope(t *testing.T, stdout string) afscpDirectEnvelope {
	t.Helper()

	var env afscpDirectEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &env), stdout)
	assert.Equal(t, "jvs.afscp.direct.v1", env.Contract)
	return env
}

func assertAFSCPDirectJSONDoesNotLeakSelector(t *testing.T, stdout, controlRoot, home string) {
	t.Helper()

	assert.NotContains(t, stdout, controlRoot)
	assert.NotContains(t, stdout, home)
	for _, forbidden := range []string{
		"raw_command",
		"argv",
		"payload_root_hash",
		"content_root_hash",
		"save_profile",
		"expected_folder_evidence",
	} {
		assert.NotContains(t, strings.ToLower(stdout), forbidden)
	}
}

func mustMarshalForAFSCPTest(t *testing.T, value any) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func installAFSCPFakeJuiceFSClone(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	juicefsPath := filepath.Join(binDir, "juicefs")
	script := `#!/bin/sh
set -eu
if [ "$#" -lt 3 ] || [ "$1" != "clone" ]; then
  printf 'unexpected juicefs args: %s\n' "$*" >&2
  exit 64
fi
/bin/mkdir -p "$3"
/bin/cp -a "$2"/. "$3"/
`
	require.NoError(t, os.WriteFile(juicefsPath, []byte(script), 0755))
	t.Setenv("PATH", binDir)
}

func mustReadAFSCPTestFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
