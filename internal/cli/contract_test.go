package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contractEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	Command       string          `json:"command"`
	OK            bool            `json:"ok"`
	RepoRoot      *string         `json:"repo_root"`
	Workspace     *string         `json:"workspace"`
	Data          json.RawMessage `json:"data"`
	Error         *contractError  `json:"error"`
}

type contractError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

func TestCLIJSONEnvelope_InfoIsSingleObject(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoRoot := initLegacyContractRepo(t, "testrepo")
	require.NoError(t, os.Chdir(filepath.Join(repoRoot, "main")))

	cmd := createTestRootCmd()
	stdout, err := executeCommand(cmd, "--json", "info")
	require.NoError(t, err)

	env := decodeContractEnvelope(t, stdout)
	assert.Equal(t, 1, env.SchemaVersion)
	assert.Equal(t, "info", env.Command)
	assert.True(t, env.OK)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)
	assert.Nil(t, env.Error)

	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, repoRoot, data["repo_root"])
	assert.Contains(t, data, "format_version")
}

func TestCLITargetingRepoFlag_RejectsOutsideAnyRepo(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoRoot := initLegacyContractRepo(t, "testrepo")
	outside := filepath.Join(dir, "outside")
	require.NoError(t, os.Mkdir(outside, 0755))

	cases := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "info", command: "info", args: []string{"--json", "--repo", repoRoot, "info"}},
		{name: "status", command: "status", args: []string{"--json", "--repo", repoRoot, "status"}},
		{name: "workspace_list", command: "workspace list", args: []string{"--json", "--repo", repoRoot, "workspace", "list"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, outside, tc.args...)

			require.Equal(t, 1, exitCode)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeContractEnvelope(t, stdout)
			assert.False(t, env.OK)
			assert.Equal(t, tc.command, env.Command)
			assert.Nil(t, env.RepoRoot)
			assert.Nil(t, env.Workspace)
			require.NotNil(t, env.Error)
			assert.Equal(t, "E_NOT_REPO", env.Error.Code)
			assertPublicErrorOmitsLegacyVocabulary(t, env.Error)
			assert.JSONEq(t, `null`, string(env.Data))
		})
	}
}

func TestCLITargetingWorkspaceFlag_StatusFromRepoRoot(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoRoot := initLegacyContractRepo(t, "testrepo")
	require.NoError(t, os.Chdir(repoRoot))

	cmd := createTestRootCmd()
	stdout, err := executeCommand(cmd, "--json", "--workspace", "main", "status")
	require.NoError(t, err)

	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)

	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, "main", data["workspace"])
	assert.Equal(t, filepath.Join(repoRoot, "main"), data["folder"])
}

func TestCLITargetingRepoFlag_StatusInfersWorkspaceFromRealCWD(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoRoot := initLegacyContractRepo(t, "repoA")
	require.NoError(t, os.Chdir(filepath.Join(repoRoot, "main")))

	cmd := createTestRootCmd()
	stdout, err := executeCommand(cmd, "--json", "--repo", repoRoot, "status")
	require.NoError(t, err, stdout)

	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)

	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, "main", data["workspace"])
	assert.Equal(t, filepath.Join(repoRoot, "main"), data["folder"])
}

func TestCLITargetingRepoFlag_StatusAcceptsPathInsideSameRepoFromSubdir(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoRoot := initLegacyContractRepo(t, "repoA")
	subdir := filepath.Join(repoRoot, "main", "subdir")
	require.NoError(t, os.Mkdir(subdir, 0755))
	require.NoError(t, os.Chdir(subdir))

	cmd := createTestRootCmd()
	stdout, err := executeCommand(cmd, "--json", "--repo", filepath.Join(repoRoot, "main"), "status")
	require.NoError(t, err, stdout)

	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)
}

func TestCLITargetingRepoFlag_StatusAcceptsSymlinkedPathInsideSamePhysicalRepo(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoRoot := initLegacyContractRepo(t, "repoA")
	repoLink := filepath.Join(dir, "repo-link")
	require.NoError(t, os.Symlink(repoRoot, repoLink))
	require.NoError(t, os.Chdir(filepath.Join(repoRoot, "main")))

	cmd := createTestRootCmd()
	stdout, err := executeCommand(cmd, "--json", "--repo", filepath.Join(repoLink, "main"), "status")
	require.NoError(t, err, stdout)

	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)
}

func TestCLITargetingInferredWorkspaceMustBeRegistered(t *testing.T) {
	repoRoot, _ := setupTargetingContractRepo(t)
	ghostPath := filepath.Join(repoRoot, "worktrees", "ghost", "nested")
	require.NoError(t, os.MkdirAll(ghostPath, 0755))

	stdout, stderr, exitCode := runContractSubprocess(t, ghostPath, "--json", "info")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	assert.Nil(t, env.Workspace)

	stdout, stderr, exitCode = runContractSubprocess(t, ghostPath, "--json", "status")
	require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env = decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	assert.Nil(t, env.Workspace)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)
	assert.NotContains(t, stdout, "ghost")
}

func TestCLITargetingFakePayloadWithRegisteredNameIsNotWorkspace(t *testing.T) {
	repoRoot, _ := setupTargetingContractRepo(t)
	fakeMain := filepath.Join(repoRoot, "worktrees", "main", "nested")
	require.NoError(t, os.MkdirAll(fakeMain, 0755))

	stdout, stderr, exitCode := runContractSubprocess(t, fakeMain, "--json", "info")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Nil(t, env.Workspace)

	stdout, stderr, exitCode = runContractSubprocess(t, fakeMain, "--json", "status")
	require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env = decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "status", env.Command)
	assert.Nil(t, env.Workspace)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)
}

func TestCLITargetingMetadataDirRequiresExplicitWorkspace(t *testing.T) {
	repoRoot, mainPath := setupTargetingContractRepo(t)
	firstID := createTargetingContractCheckpoint(t, mainPath, "first")
	secondID := createTargetingContractCheckpoint(t, mainPath, "second")

	metadataCWDs := []string{
		filepath.Join(repoRoot, ".jvs"),
		filepath.Join(repoRoot, ".jvs", "worktrees", "main"),
	}
	for _, cwd := range metadataCWDs {
		t.Run(filepath.Base(cwd), func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				command string
				args    []string
			}{
				{name: "status", command: "status", args: []string{"--json", "status"}},
				{name: "workspace_path", command: "workspace path", args: []string{"--json", "workspace", "path"}},
				{name: "checkpoint_list", command: "checkpoint list", args: []string{"--json", "checkpoint", "list"}},
				{name: "diff", command: "diff", args: []string{"--json", "diff", firstID, secondID}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					stdout, stderr, exitCode := runContractSubprocess(t, cwd, tc.args...)
					require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
					assert.Empty(t, strings.TrimSpace(stderr))
					env := decodeContractEnvelope(t, stdout)
					assert.False(t, env.OK)
					assert.Equal(t, tc.command, env.Command)
					require.NotNil(t, env.RepoRoot)
					assert.Equal(t, repoRoot, *env.RepoRoot)
					assert.Nil(t, env.Workspace)
					require.NotNil(t, env.Error)
					assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)
				})
			}
		})
	}

	for _, tc := range []struct {
		name    string
		command string
		args    []string
	}{
		{name: "status", command: "status", args: []string{"--json", "--workspace", "main", "status"}},
		{name: "workspace_path", command: "workspace path", args: []string{"--json", "--workspace", "main", "workspace", "path"}},
		{name: "checkpoint_list", command: "checkpoint list", args: []string{"--json", "--workspace", "main", "checkpoint", "list"}},
		{name: "diff", command: "diff", args: []string{"--json", "--workspace", "main", "diff", firstID, secondID}},
	} {
		t.Run("explicit_"+tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, filepath.Join(repoRoot, ".jvs"), tc.args...)
			require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))
			env := decodeContractEnvelope(t, stdout)
			require.True(t, env.OK, stdout)
			assert.Equal(t, tc.command, env.Command)
			require.NotNil(t, env.Workspace)
			assert.Equal(t, "main", *env.Workspace)
		})
	}
}

func TestCLITargetingWorkspaceFlagOverridesCWDWorkspace(t *testing.T) {
	repoRoot, mainPath := setupTargetingContractRepo(t)
	createTargetingContractCheckpoint(t, mainPath, "base")
	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "--json", "fork", "feature")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)

	stdout, stderr, exitCode = runContractSubprocess(t, mainPath, "--json", "--workspace", "feature", "status")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "feature", *env.Workspace)
	data := decodeContractDataMap(t, stdout)
	assert.Equal(t, "feature", data["workspace"])
}

func TestCLITargetingCheckpointListIsWorkspaceScoped(t *testing.T) {
	_, mainPath := setupTargetingContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("main base"), 0644))
	mainID := createTargetingContractCheckpoint(t, mainPath, "main only")

	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "--json", "fork", "feature")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	featurePath := filepath.Join(filepath.Dir(mainPath), "worktrees", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "data.txt"), []byte("feature change"), 0644))
	featureID := createTargetingContractCheckpoint(t, featurePath, "feature only")

	stdout, stderr, exitCode = runContractSubprocess(t, mainPath, "--json", "checkpoint", "list")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	mainRecords := decodeTargetingCheckpointRecords(t, stdout)
	require.Len(t, mainRecords, 1, stdout)
	assert.Equal(t, mainID, mainRecords[0].CheckpointID)
	assert.Equal(t, "main", mainRecords[0].Workspace)

	stdout, stderr, exitCode = runContractSubprocess(t, featurePath, "--json", "checkpoint", "list")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	featureRecords := decodeTargetingCheckpointRecords(t, stdout)
	require.Len(t, featureRecords, 1, stdout)
	assert.Equal(t, featureID, featureRecords[0].CheckpointID)
	assert.Equal(t, "feature", featureRecords[0].Workspace)
}

func TestCLITargetingDiffRejectsCrossWorkspaceCheckpointRefs(t *testing.T) {
	_, mainPath := setupTargetingContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("main base"), 0644))
	mainID := createTargetingContractCheckpoint(t, mainPath, "main base")

	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "--json", "fork", "feature")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	featurePath := filepath.Join(filepath.Dir(mainPath), "worktrees", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "data.txt"), []byte("feature change"), 0644))
	featureID := createTargetingContractCheckpointWithArgs(t, featurePath, "feature tagged", "--tag", "feature-tag")

	for _, tc := range []struct {
		name string
		ref  string
	}{
		{name: "full_id", ref: featureID},
		{name: "short_id", ref: featureID[:12]},
		{name: "tag", ref: "feature-tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "--json", "diff", mainID, tc.ref)
			require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))
			env := decodeContractEnvelope(t, stdout)
			assert.False(t, env.OK)
			assert.Equal(t, "diff", env.Command)
			require.NotNil(t, env.Workspace)
			assert.Equal(t, "main", *env.Workspace)
			require.NotNil(t, env.Error)
			assert.Equal(t, "E_REF_NOT_FOUND", env.Error.Code)
			assert.NotContains(t, env.Error.Message, ".jvs")
			assert.NotContains(t, env.Error.Message, "worktrees")
		})
	}

	stdout, stderr, exitCode = runContractSubprocess(t, featurePath, "--json", "diff", featureID, "feature-tag")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
}

func TestCLITargetingRepoFlag_StatusRejectsWorkspaceFromDifferentRepo(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoA := initLegacyContractRepo(t, "repoA")
	repoB := initLegacyContractRepo(t, "repoB")
	repoBMain := filepath.Join(repoB, "main")
	stdout, stderr, exitCode := runContractSubprocess(t, repoBMain, "--json", "--repo", repoA, "--workspace", "main", "status")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoA, *env.RepoRoot)
	assert.Nil(t, env.Workspace)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_TARGET_MISMATCH", env.Error.Code)
	assert.Contains(t, env.Error.Message, "targeting mismatch")
	assert.Contains(t, env.Error.Message, repoA)
	assert.Contains(t, env.Error.Message, filepath.Join(dir, "repoB"))
	assertPublicErrorOmitsLegacyVocabulary(t, env.Error)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestCLITargetingMissingWorkspaceUsesPublicNotWorkspaceError(t *testing.T) {
	_, mainPath := setupTargetingContractRepo(t)

	for _, tc := range []struct {
		name    string
		command string
		args    []string
	}{
		{name: "path", command: "workspace path", args: []string{"--json", "workspace", "path", "missing"}},
		{name: "remove", command: "workspace remove", args: []string{"--json", "workspace", "remove", "missing"}},
		{name: "rename", command: "workspace rename", args: []string{"--json", "workspace", "rename", "missing", "renamed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, mainPath, tc.args...)
			require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))
			env := decodeContractEnvelope(t, stdout)
			assert.False(t, env.OK)
			assert.Equal(t, tc.command, env.Command)
			require.NotNil(t, env.Error)
			assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)
			assert.Contains(t, env.Error.Message, "workspace")
			assert.NotContains(t, env.Error.Message, ".jvs")
			assert.NotContains(t, env.Error.Message, "config")
			assertPublicErrorOmitsLegacyVocabulary(t, env.Error)
		})
	}
}

func TestCLITargetingCheckpointListRequiresWorkspace(t *testing.T) {
	repoRoot, mainPath := setupTargetingContractRepo(t)
	createTargetingContractCheckpoint(t, mainPath, "first")

	stdout, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "checkpoint", "list")
	require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "checkpoint list", env.Command)
	assert.Nil(t, env.Workspace)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)

	stdout, stderr, exitCode = runContractSubprocess(t, repoRoot, "--json", "--workspace", "main", "checkpoint", "list")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	env = decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)
}

func TestCLITargetingDiffRequiresWorkspaceBeforeResolvingFullCheckpointIDs(t *testing.T) {
	repoRoot, mainPath := setupTargetingContractRepo(t)
	firstID := createTargetingContractCheckpoint(t, mainPath, "first")
	secondID := createTargetingContractCheckpoint(t, mainPath, "second")

	stdout, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "diff", firstID, secondID)
	require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "diff", env.Command)
	assert.Nil(t, env.Workspace)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)

	for _, tc := range []struct {
		name string
		cwd  string
		args []string
	}{
		{name: "workspace_cwd", cwd: mainPath, args: []string{"--json", "diff", firstID, secondID}},
		{name: "workspace_flag", cwd: repoRoot, args: []string{"--json", "--workspace", "main", "diff", firstID, secondID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, tc.cwd, tc.args...)
			require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))
			env := decodeContractEnvelope(t, stdout)
			require.True(t, env.OK, stdout)
			require.NotNil(t, env.Workspace)
			assert.Equal(t, "main", *env.Workspace)
		})
	}
}

func TestCLITargetingRepoFlag_RejectsDifferentRepoWithDuplicatedRepoID(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoA := initLegacyContractRepo(t, "repoA")
	repoB := initLegacyContractRepo(t, "repoB")
	repoAID, err := os.ReadFile(filepath.Join(repoA, ".jvs", "repo_id"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoB, ".jvs", "repo_id"), repoAID, 0600))

	stdout, stderr, exitCode := runContractSubprocess(t, filepath.Join(repoB, "main"), "--json", "--repo", repoA, "--workspace", "main", "status")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_TARGET_MISMATCH", env.Error.Code)
	assert.Contains(t, env.Error.Message, "targeting mismatch")
	assert.Contains(t, env.Error.Message, repoA)
	assert.Contains(t, env.Error.Message, repoB)
	assertPublicErrorOmitsLegacyVocabulary(t, env.Error)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestCLIPathScopedSetupCommandsRemainRepoFree(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	outside := filepath.Join(dir, "outside")
	require.NoError(t, os.Mkdir(outside, 0755))
	require.NoError(t, os.Chdir(outside))
	unusedRepoFlag := filepath.Join(dir, "missing-repo")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "--repo", unusedRepoFlag, "init", "initrepo")
	require.NoError(t, err, stdout)

	sourceDir := filepath.Join(outside, "source")
	require.NoError(t, os.Mkdir(sourceDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "file.txt"), []byte("import me"), 0644))
	stdout, err = executeCommand(createTestRootCmd(), "--json", "--repo", unusedRepoFlag, "import", sourceDir, "importrepo")
	require.NoError(t, err, stdout)

	stdout, err = executeCommand(createTestRootCmd(), "--json", "--repo", unusedRepoFlag, "clone", "initrepo", "clonerepo")
	require.NoError(t, err, stdout)

	stdout, err = executeCommand(createTestRootCmd(), "--json", "--repo", unusedRepoFlag, "capability", outside)
	require.NoError(t, err, stdout)
}

func TestCLISetupJSONContract_InitAndCapability(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	require.NoError(t, os.Chdir(dir))

	initTarget := filepath.Join(dir, "initrepo")
	stdout, err := executeCommand(createTestRootCmd(), "--json", "init", initTarget)
	require.NoError(t, err, stdout)
	initData := assertSetupJSONContractForTest(t, stdout)
	assert.Equal(t, initTarget, initData["repo_root"])

	capabilityTarget := filepath.Join(dir, "capability-target")
	require.NoError(t, os.Mkdir(capabilityTarget, 0755))
	stdout, err = executeCommand(createTestRootCmd(), "--json", "capability", capabilityTarget)
	require.NoError(t, err, stdout)
	capabilityData := assertSetupJSONContractForTest(t, stdout)
	assert.Equal(t, capabilityTarget, capabilityData["target_path"])
	assert.IsType(t, false, capabilityData["write_probe"])
}

func TestCLIJSONErrorEnvelope_NotRepo(t *testing.T) {
	stdout, stderr, exitCode := runContractSubprocess(t, t.TempDir(), "--json", "info")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "info", env.Command)
	assert.Nil(t, env.RepoRoot)
	assert.Nil(t, env.Workspace)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_REPO", env.Error.Code)
	assert.Contains(t, env.Error.Message, "not a JVS repository")
	assert.Contains(t, env.Error.Hint, "jvs init")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestCLIJSONErrorEnvelope_StatusRequiresWorkspace(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoRoot := initLegacyContractRepo(t, "testrepo")
	stdout, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "status")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	assert.Nil(t, env.Workspace)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "not inside a workspace")
	assertPublicErrorOmitsLegacyVocabulary(t, env.Error)
}

func TestCLIJSONErrorEnvelope_ArgValidationReportsCommand(t *testing.T) {
	stdout, stderr, exitCode := runContractSubprocess(t, t.TempDir(), "--json", "diff")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "diff", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "diff requires two checkpoint refs")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestCLIJSONErrorEnvelope_UnknownCommandIsSingleObject(t *testing.T) {
	stdout, stderr, exitCode := runContractSubprocess(t, t.TempDir(), "--json", "histroy")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "histroy", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "unknown command")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestCLIJSONErrorEnvelope_CompletionRejectsScriptOutput(t *testing.T) {
	stdout, stderr, exitCode := runContractSubprocess(t, t.TempDir(), "--json", "completion", "bash")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))
	assert.NotContains(t, stdout, "bash completion for")
	assert.NotContains(t, stdout, "complete -")

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "completion", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "--json")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestCLIJSONEnvelope_HistoryUsesSavePointSchema(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repoPath, 0755))
	_, err := repo.InitAdoptedWorkspace(repoPath)
	require.NoError(t, err)

	stdout, stderr, exitCode := runContractSubprocess(t, repoPath, "--json", "history")

	require.Equal(t, 0, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))
	assert.NotContains(t, stdout, "snapshot_id")
	assert.NotContains(t, stdout, "worktree")
	assert.NotContains(t, stdout, "HEAD")

	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Equal(t, "history", env.Command)
	assert.Nil(t, env.Error)
	assert.Contains(t, string(env.Data), "save_points")
	assert.NotContains(t, string(env.Data), "checkpoint_id")
}

func TestDoctorRepairRuntimeRejectsPositionalArgs(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	require.NoError(t, os.Chdir(dir))
	initLegacyContractRepo(t, "testrepo")

	stdout, stderr, exitCode := runContractSubprocess(t, filepath.Join(repoPath, "main"), "--json", "doctor", "--repair-runtime", "clean_runtime_tmp")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "doctor", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestDoctorRepairRuntimeJSONIncludesRepairResults(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	require.NoError(t, os.Chdir(dir))
	initLegacyContractRepo(t, "testrepo")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs-tmp-orphan"), []byte("tmp"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, filepath.Join(repoPath, "main"), "--json", "doctor", "--repair-runtime")

	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	assert.Equal(t, true, data["healthy"])
	repairs, ok := data["repairs"].([]any)
	require.True(t, ok, "doctor repair JSON must include repairs: %s", stdout)
	require.NotEmpty(t, repairs)
	actions := map[string]bool{}
	for _, raw := range repairs {
		repair, ok := raw.(map[string]any)
		require.True(t, ok)
		action, _ := repair["action"].(string)
		actions[action] = true
	}
	assert.True(t, actions["clean_locks"])
	assert.True(t, actions["clean_runtime_tmp"])
	assert.True(t, actions["clean_runtime_operations"])
}

func TestDoctorHumanOutputUsesPublicVocabulary(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	require.NoError(t, os.Chdir(dir))
	initLegacyContractRepo(t, "testrepo")
	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, "main")))

	stdout, stderr, exitCode := runContractSubprocess(t, repoPath, "doctor")

	require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	lower := strings.ToLower(stdout)
	assert.Contains(t, lower, "workspace")
	assert.NotContains(t, lower, "worktree")
	assert.NotContains(t, lower, "snapshot")
	assert.NotContains(t, lower, "head")
}

func TestVerifyAllRejectsCheckpointArg(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	require.NoError(t, os.Chdir(dir))
	initLegacyContractRepo(t, "testrepo")

	stdout, stderr, exitCode := runContractSubprocess(t, filepath.Join(repoPath, "main"), "--json", "verify", "--all", "1708300800000-deadbeef")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "verify", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestVerifyAllIsCheckpointScopedAndDoctorStrictOwnsAuditChain(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	mainPath := filepath.Join(repoPath, "main")
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	require.NoError(t, os.Chdir(dir))
	initLegacyContractRepo(t, "testrepo")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("checkpoint payload"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "--json", "checkpoint", "audit contract")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	require.NoError(t, os.WriteFile(auditPath, []byte("{malformed audit record}\n"), 0644))

	stdout, stderr, exitCode = runContractSubprocess(t, mainPath, "--json", "verify", "--all")
	require.Equal(t, 0, exitCode, "verify --all stays checkpoint-scoped: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Equal(t, "verify", env.Command)
	var verifyResults []map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &verifyResults), stdout)
	require.NotEmpty(t, verifyResults)
	for _, result := range verifyResults {
		assert.NotContains(t, fmt.Sprint(result["error_code"]), "AUDIT")
	}

	stdout, stderr, exitCode = runContractSubprocess(t, mainPath, "--json", "doctor", "--strict")
	require.Equal(t, 1, exitCode, "doctor --strict must own audit-chain failures: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env = decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Equal(t, "doctor", env.Command)
	var doctorData map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &doctorData), stdout)
	assert.Equal(t, false, doctorData["healthy"])
	findings, ok := doctorData["findings"].([]any)
	require.True(t, ok, stdout)
	foundAudit := false
	for _, raw := range findings {
		finding, ok := raw.(map[string]any)
		require.True(t, ok)
		if finding["error_code"] == "E_AUDIT_RECORD_MALFORMED" {
			foundAudit = true
			break
		}
	}
	assert.True(t, foundAudit, "doctor --strict must report malformed audit chain: %s", stdout)
}

func TestVerifyPayloadMismatchHasErrorCodeAndNonzeroExit(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	mainPath := filepath.Join(repoPath, "main")
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	require.NoError(t, os.Chdir(dir))
	initLegacyContractRepo(t, "testrepo")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("before"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "--json", "checkpoint", "before")
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	checkpointData := decodeContractDataMap(t, stdout)
	checkpointID, _ := checkpointData["checkpoint_id"].(string)
	require.NotEmpty(t, checkpointID)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, "tampered.txt"), []byte("tampered"), 0644))

	stdout, stderr, exitCode = runContractSubprocess(t, mainPath, "--json", "verify", checkpointID)

	require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	assert.Equal(t, true, data["tamper_detected"])
	assert.Equal(t, "E_PAYLOAD_HASH_MISMATCH", data["error_code"])
	assert.NotContains(t, strings.ToLower(fmt.Sprint(data["error"])), "snapshot")
	assert.NotContains(t, strings.ToLower(fmt.Sprint(data["error"])), "worktree")
}

func TestCLIContractSubprocess(t *testing.T) {
	if os.Getenv("JVS_CONTRACT_SUBPROCESS") != "1" {
		t.Skip("contract subprocess helper")
	}

	cwd := os.Getenv("JVS_CONTRACT_CWD")
	if cwd != "" {
		if err := os.Chdir(cwd); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}

	var args []string
	if err := json.Unmarshal([]byte(os.Getenv("JVS_CONTRACT_ARGS")), &args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	cmd := createTestRootCmd()
	cmd.SetArgs(args)
	primeJSONOutputFromArgs(args)
	executedCmd, err := cmd.ExecuteC()
	if err != nil {
		reportCommandErrorForCommand(executedCmd, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func decodeContractEnvelope(t *testing.T, stdout string) contractEnvelope {
	t.Helper()

	dec := json.NewDecoder(strings.NewReader(stdout))
	var env contractEnvelope
	require.NoError(t, dec.Decode(&env), "stdout was not a JSON envelope: %q", stdout)

	var extra any
	err := dec.Decode(&extra)
	require.True(t, errors.Is(err, io.EOF), "stdout must contain exactly one JSON value: %q", stdout)

	return env
}

func decodeContractDataMap(t *testing.T, stdout string) map[string]any {
	t.Helper()

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	return data
}

func setupTargetingContractRepo(t *testing.T) (repoRoot string, mainPath string) {
	t.Helper()

	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWd))
	})

	repoRoot = initLegacyContractRepo(t, "testrepo")
	mainPath = filepath.Join(repoRoot, "main")
	return repoRoot, mainPath
}

func initLegacyContractRepo(t *testing.T, name string) string {
	t.Helper()

	r, err := repo.InitTarget(name)
	require.NoError(t, err)
	return r.Root
}

func createTargetingContractCheckpoint(t *testing.T, mainPath, note string) string {
	t.Helper()

	return createTargetingContractCheckpointWithArgs(t, mainPath, note)
}

func createTargetingContractCheckpointWithArgs(t *testing.T, workspacePath, note string, args ...string) string {
	t.Helper()

	allArgs := append([]string{"--json", "checkpoint", note}, args...)
	stdout, stderr, exitCode := runContractSubprocess(t, workspacePath, allArgs...)
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	data := decodeContractDataMap(t, stdout)
	checkpointID, _ := data["checkpoint_id"].(string)
	require.NotEmpty(t, checkpointID, stdout)
	return checkpointID
}

func decodeTargetingCheckpointRecords(t *testing.T, stdout string) []publicCheckpointRecord {
	t.Helper()
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	var records []publicCheckpointRecord
	require.NoError(t, json.Unmarshal(env.Data, &records), stdout)
	return records
}

func assertSetupJSONContractForTest(t *testing.T, stdout string) map[string]any {
	t.Helper()

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)

	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	require.NotEmpty(t, data, stdout)

	capabilities, ok := data["capabilities"].(map[string]any)
	require.True(t, ok, "setup JSON data.capabilities must be an object: %s", stdout)
	for _, field := range []string{"write", "juicefs", "reflink", "copy"} {
		_, ok := capabilities[field].(map[string]any)
		require.True(t, ok, "setup JSON data.capabilities.%s must be an object: %s", field, stdout)
	}

	effectiveEngine, ok := data["effective_engine"].(string)
	require.True(t, ok, "setup JSON data.effective_engine must be a string: %s", stdout)
	require.NotEmpty(t, effectiveEngine, stdout)

	_, ok = data["warnings"].([]any)
	require.True(t, ok, "setup JSON data.warnings must be an array: %s", stdout)

	return data
}

func assertPublicErrorOmitsLegacyVocabulary(t *testing.T, errData *contractError) {
	t.Helper()
	require.NotNil(t, errData)
	for _, value := range []string{errData.Code, errData.Message, errData.Hint} {
		lower := strings.ToLower(value)
		assert.NotContains(t, lower, "worktree")
		assert.NotContains(t, lower, "snapshot")
		assert.NotContains(t, lower, "history")
	}
}

func assertPublicErrorOmitsLegacyVocabularyExcept(t *testing.T, errData *contractError, allowedValues ...string) {
	t.Helper()
	require.NotNil(t, errData)
	for _, value := range []string{errData.Code, errData.Message, errData.Hint} {
		for _, allowed := range allowedValues {
			value = strings.ReplaceAll(value, allowed, "")
		}
		lower := strings.ToLower(value)
		assert.NotContains(t, lower, "worktree")
		assert.NotContains(t, lower, "snapshot")
		assert.NotContains(t, lower, "history")
	}
}

func runContractSubprocess(t *testing.T, cwd string, args ...string) (string, string, int) {
	t.Helper()

	argData, err := json.Marshal(args)
	require.NoError(t, err)

	cmd := exec.Command(os.Args[0], "-test.run=TestCLIContractSubprocess")
	cmd.Env = append(os.Environ(),
		"JVS_CONTRACT_SUBPROCESS=1",
		"JVS_CONTRACT_CWD="+cwd,
		"JVS_CONTRACT_ARGS="+string(argData),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}

	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr), "unexpected subprocess error: %v", err)
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}
