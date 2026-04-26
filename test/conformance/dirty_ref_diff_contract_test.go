//go:build conformance

package conformance

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type contractCommandResult struct {
	stdout string
	stderr string
	code   int
	env    contractSmokeEnvelope
}

func TestContract_DirtySafetyMatrixAndJSONEnvelope(t *testing.T) {
	t.Run("restore_default_rejects_and_explicit_modes_succeed", func(t *testing.T) {
		repoPath, cleanup := initTestRepo(t)
		defer cleanup()
		mainPath := filepath.Join(repoPath, "main")
		dataPath := filepath.Join(mainPath, "data.txt")
		extraPath := filepath.Join(mainPath, "scratch.txt")

		writeContractFile(t, dataPath, "v1")
		first := createContractCheckpoint(t, repoPath, "first")
		writeContractFile(t, dataPath, "v2")
		second := createContractCheckpoint(t, repoPath, "second")
		writeContractFile(t, dataPath, "dirty")
		writeContractFile(t, extraPath, "uncheckpointed")

		result := runContractJSON(t, repoPath, "restore", first)
		requireContractJSONFailure(t, result, "restore")
		requireContractErrorContains(t, result, "dirty")
		requireContractFileContent(t, dataPath, "dirty")
		requireContractFileContent(t, extraPath, "uncheckpointed")

		result = runContractJSON(t, repoPath, "restore", first, "--discard-dirty")
		requireContractJSONSuccess(t, result, "restore")
		requireContractDataString(t, result, "checkpoint_id", first)
		requireContractFileContent(t, dataPath, "v1")
		if fileExists(t, extraPath) {
			t.Fatalf("--discard-dirty restore left uncheckpointed file %s", extraPath)
		}

		result = runContractJSON(t, repoPath, "restore", "latest", "--discard-dirty")
		requireContractJSONSuccess(t, result, "restore")
		requireContractDataString(t, result, "checkpoint_id", second)
		writeContractFile(t, dataPath, "dirty-included")
		result = runContractJSON(t, repoPath, "restore", first, "--include-working")
		requireContractJSONSuccess(t, result, "restore")
		requireContractDataString(t, result, "checkpoint_id", first)
		requireContractFileContent(t, dataPath, "v1")

		status := readWorkspaceStatus(t, repoPath)
		if status.Current != first {
			t.Fatalf("restore --include-working current = %s, want %s", status.Current, first)
		}
		if status.Latest == second {
			t.Fatalf("restore --include-working did not create a new latest checkpoint")
		}
		if status.Dirty {
			t.Fatalf("workspace should be clean after restore --include-working")
		}
		historyOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
		requireContractRawJSONOK(t, historyOut, stderr, code, "checkpoint list")
		if got := len(extractAllSnapshotIDs(historyOut)); got != 3 {
			t.Fatalf("restore --include-working should leave 3 checkpoints, got %d\n%s", got, historyOut)
		}
	})

	t.Run("fork_default_rejects_and_explicit_modes_succeed", func(t *testing.T) {
		repoPath, cleanup := initTestRepo(t)
		defer cleanup()
		mainPath := filepath.Join(repoPath, "main")
		dataPath := filepath.Join(mainPath, "data.txt")

		writeContractFile(t, dataPath, "clean")
		createContractCheckpoint(t, repoPath, "base")
		writeContractFile(t, dataPath, "dirty")

		result := runContractJSON(t, repoPath, "fork", "discard-branch")
		requireContractJSONFailure(t, result, "fork")
		requireContractErrorContains(t, result, "dirty")
		if fileExists(t, filepath.Join(repoPath, "worktrees", "discard-branch")) {
			t.Fatalf("fork created workspace despite dirty rejection")
		}

		result = runContractJSON(t, repoPath, "fork", "discard-branch", "--discard-dirty")
		requireContractJSONSuccess(t, result, "fork")
		requireContractDataString(t, result, "workspace", "discard-branch")
		requireContractFileContent(t, filepath.Join(repoPath, "worktrees", "discard-branch", "data.txt"), "clean")

		writeContractFile(t, dataPath, "included")
		result = runContractJSON(t, repoPath, "fork", "include-branch", "--include-working")
		requireContractJSONSuccess(t, result, "fork")
		requireContractDataString(t, result, "workspace", "include-branch")
		requireContractFileContent(t, filepath.Join(repoPath, "worktrees", "include-branch", "data.txt"), "included")
		status := readWorkspaceStatus(t, repoPath)
		if status.Dirty {
			t.Fatalf("source workspace should be clean after fork --include-working")
		}
	})

	t.Run("workspace_remove_rejects_dirty_by_default", func(t *testing.T) {
		repoPath, cleanup := initTestRepo(t)
		defer cleanup()
		mainPath := filepath.Join(repoPath, "main")

		writeContractFile(t, filepath.Join(mainPath, "data.txt"), "base")
		createContractCheckpoint(t, repoPath, "base")
		stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "fork", "feature")
		requireContractRawJSONOK(t, stdout, stderr, code, "fork")
		featureFile := filepath.Join(repoPath, "worktrees", "feature", "data.txt")
		writeContractFile(t, featureFile, "dirty")

		result := runContractJSON(t, repoPath, "workspace", "remove", "feature")
		requireContractJSONFailure(t, result, "workspace remove")
		requireContractErrorContains(t, result, "dirty")
		requireContractFileContent(t, featureFile, "dirty")
	})
}

func TestContract_ReadOnlyCommandsWorkInDirtyWorkspace(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()
	mainPath := filepath.Join(repoPath, "main")

	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "base")
	createContractCheckpoint(t, repoPath, "base")
	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "dirty")

	cases := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "status", command: "status", args: []string{"status"}},
		{name: "info", command: "info", args: []string{"info"}},
		{name: "checkpoint_list", command: "checkpoint list", args: []string{"checkpoint", "list"}},
		{name: "workspace_list", command: "workspace list", args: []string{"workspace", "list"}},
		{name: "workspace_path", command: "workspace path", args: []string{"workspace", "path"}},
		{name: "diff_stat", command: "diff", args: []string{"diff", "current", "latest", "--stat"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := runContractJSON(t, repoPath, tc.args...)
			requireContractJSONSuccess(t, result, tc.command)
		})
	}

	status := readWorkspaceStatus(t, repoPath)
	if !status.Dirty {
		t.Fatalf("read-only commands must not clean dirty workspace")
	}
}

func TestContract_RefRulesReservedMissingAmbiguousAndNotes(t *testing.T) {
	t.Run("reserved_tags_and_workspace_names_are_rejected", func(t *testing.T) {
		for _, reserved := range []string{"current", "latest", "dirty"} {
			t.Run("tag_"+reserved, func(t *testing.T) {
				repoPath, cleanup := initTestRepo(t)
				defer cleanup()
				writeContractFile(t, filepath.Join(repoPath, "main", "data.txt"), reserved)

				result := runContractJSON(t, repoPath, "checkpoint", "reserved tag", "--tag", reserved)
				requireContractJSONFailure(t, result, "checkpoint")
				requireContractErrorCode(t, result, "E_REF_RESERVED")
				requireContractErrorContains(t, result, "reserved")
			})

			t.Run("fork_name_"+reserved, func(t *testing.T) {
				repoPath, cleanup := initTestRepo(t)
				defer cleanup()
				writeContractFile(t, filepath.Join(repoPath, "main", "data.txt"), "base")
				createContractCheckpoint(t, repoPath, "base")

				result := runContractJSON(t, repoPath, "fork", reserved)
				requireContractJSONFailure(t, result, "fork")
				requireContractErrorCode(t, result, "E_NAME_INVALID")
				requireContractErrorContains(t, result, "reserved")
			})

			t.Run("rename_name_"+reserved, func(t *testing.T) {
				repoPath, cleanup := initTestRepo(t)
				defer cleanup()

				result := runContractJSON(t, repoPath, "workspace", "rename", "main", reserved)
				requireContractJSONFailure(t, result, "workspace rename")
				requireContractErrorCode(t, result, "E_NAME_INVALID")
				requireContractErrorContains(t, result, "reserved")
			})
		}
	})

	t.Run("missing_ambiguous_and_note_refs_have_stable_codes", func(t *testing.T) {
		repoPath, cleanup := initTestRepo(t)
		defer cleanup()
		mainPath := filepath.Join(repoPath, "main")

		writeContractFile(t, filepath.Join(mainPath, "data.txt"), "one")
		first := createContractCheckpointWithTag(t, repoPath, "note-only-ref", "dup")
		writeContractFile(t, filepath.Join(mainPath, "data.txt"), "two")
		second := createContractCheckpointWithTag(t, repoPath, "second", "dup")

		result := runContractJSON(t, repoPath, "restore", "missing-ref")
		requireContractJSONFailure(t, result, "restore")
		requireContractErrorCode(t, result, "E_REF_NOT_FOUND")

		prefix := commonContractPrefix(first, second)
		if prefix == "" || prefix == first || prefix == second {
			t.Fatalf("could not derive ambiguous prefix from %s and %s", first, second)
		}
		result = runContractJSON(t, repoPath, "restore", prefix)
		requireContractJSONFailure(t, result, "restore")
		requireContractErrorCode(t, result, "E_REF_AMBIGUOUS")

		result = runContractJSON(t, repoPath, "restore", "dup")
		requireContractJSONFailure(t, result, "restore")
		requireContractErrorCode(t, result, "E_REF_AMBIGUOUS")

		result = runContractJSON(t, repoPath, "restore", "note-only-ref")
		requireContractJSONFailure(t, result, "restore")
		requireContractErrorCode(t, result, "E_REF_NOT_FOUND")
	})
}

func TestContract_RefResolutionReportsDescriptorCorruption(t *testing.T) {
	for _, tc := range []struct {
		name string
		ref  func(string) string
	}{
		{
			name: "full_checkpoint_id",
			ref:  func(id string) string { return id },
		},
		{
			name: "short_checkpoint_id",
			ref:  func(id string) string { return id[:8] },
		},
		{
			name: "tag_alias",
			ref:  func(string) string { return "broken-tag" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoPath, cleanup := initTestRepo(t)
			defer cleanup()
			writeContractFile(t, filepath.Join(repoPath, "main", "data.txt"), "base")
			checkpointID := createContractCheckpointWithTag(t, repoPath, "corrupt descriptor target", "broken-tag")
			corruptContractDescriptor(t, repoPath, checkpointID)

			ref := tc.ref(checkpointID)
			result := runContractJSON(t, repoPath, "diff", ref, ref)
			requireContractJSONFailure(t, result, "diff")
			requireContractErrorCode(t, result, "E_DESCRIPTOR_CORRUPT")
			requireContractErrorContains(t, result, "descriptor")
		})
	}
}

func TestContract_RefResolutionFailsClosedWhenUnreadableDescriptorCouldHideTag(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()
	mainPath := filepath.Join(repoPath, "main")

	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "good")
	createContractCheckpointWithTag(t, repoPath, "release target", "release")
	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "broken")
	broken := createContractCheckpointWithTag(t, repoPath, "unreadable descriptor", "other")
	corruptContractDescriptor(t, repoPath, broken)

	result := runContractJSON(t, repoPath, "diff", "release", "release")
	requireContractJSONFailure(t, result, "diff")
	requireContractErrorCode(t, result, "E_DESCRIPTOR_CORRUPT")
}

func TestContract_DiffRejectsDamagedPublishState(t *testing.T) {
	cases := []struct {
		name     string
		wantCode string
		mutate   func(t *testing.T, repoPath, checkpointID string)
	}{
		{
			name:     "missing_ready",
			wantCode: "E_READY_MISSING",
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				requireRemovePublishStatePath(t, filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, ".READY"))
			},
		},
		{
			name:     "malformed_ready",
			wantCode: "E_READY_INVALID",
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, ".READY"), []byte("{not json"), 0644); err != nil {
					t.Fatalf("write malformed READY: %v", err)
				}
			},
		},
		{
			name:     "descriptor_checksum_mismatch",
			wantCode: "E_DESCRIPTOR_CHECKSUM_MISMATCH",
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				const badChecksum = "bad-descriptor-checksum"
				mutatePublishStateDescriptorJSON(t, repoPath, checkpointID, func(desc map[string]any) {
					desc["descriptor_checksum"] = badChecksum
				})
				mutatePublishStateReadyJSON(t, repoPath, checkpointID, func(marker map[string]any) {
					marker["descriptor_checksum"] = badChecksum
				})
			},
		},
		{
			name:     "payload_mismatch",
			wantCode: "E_PAYLOAD_HASH_MISMATCH",
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, "tampered.txt"), []byte("tampered"), 0644); err != nil {
					t.Fatalf("tamper payload: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoPath, cleanup := initTestRepo(t)
			defer cleanup()
			writeContractFile(t, filepath.Join(repoPath, "main", "data.txt"), tc.name)
			checkpointID := createContractCheckpoint(t, repoPath, tc.name)
			tc.mutate(t, repoPath, checkpointID)

			result := runContractJSON(t, repoPath, "diff", checkpointID, checkpointID, "--stat")
			requireContractJSONFailure(t, result, "diff")
			requireContractErrorCode(t, result, tc.wantCode)
		})
	}
}

func TestContract_DiffRealContentAndStatSmoke(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()
	mainPath := filepath.Join(repoPath, "main")

	writeContractFile(t, filepath.Join(mainPath, "common.txt"), "old")
	writeContractFile(t, filepath.Join(mainPath, "removed.txt"), "removed")
	fromID := createContractCheckpointWithTag(t, repoPath, "base", "base")

	writeContractFile(t, filepath.Join(mainPath, "common.txt"), "new content")
	if err := os.Remove(filepath.Join(mainPath, "removed.txt")); err != nil {
		t.Fatal(err)
	}
	writeContractFile(t, filepath.Join(mainPath, "added.txt"), "added")
	toID := createContractCheckpointWithTag(t, repoPath, "target", "target")

	stdout, stderr, code := runJVSInRepo(t, repoPath, "diff", "base", "target")
	if code != 0 {
		t.Fatalf("human diff failed: stdout=%s stderr=%s", stdout, stderr)
	}
	for _, want := range []string{
		"Diff " + fromID[:8] + " -> " + toID[:8],
		"Added (1):",
		"+ added.txt",
		"Removed (1):",
		"- removed.txt",
		"Modified (1):",
		"~ common.txt",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("human diff missing %q:\n%s", want, stdout)
		}
	}

	stdout, stderr, code = runJVSInRepo(t, repoPath, "diff", fromID, toID, "--stat")
	if code != 0 {
		t.Fatalf("human diff --stat failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "Added: 1, Removed: 1, Modified: 1" {
		t.Fatalf("unexpected diff --stat output: %q", stdout)
	}

	result := runContractJSON(t, repoPath, "diff", "base", "target", "--stat")
	requireContractJSONSuccess(t, result, "diff")
	data := contractDataMap(t, result)
	if data["from_checkpoint"] != fromID || data["to_checkpoint"] != toID {
		t.Fatalf("diff JSON checkpoint ids mismatch: %#v", data)
	}
	for field, want := range map[string]float64{
		"total_added":    1,
		"total_removed":  1,
		"total_modified": 1,
	} {
		if data[field] != want {
			t.Fatalf("diff JSON %s = %#v, want %.0f\n%s", field, data[field], want, result.stdout)
		}
	}
	requireContractChangePath(t, data, "added", "added.txt")
	requireContractChangePath(t, data, "removed", "removed.txt")
	requireContractChangePath(t, data, "modified", "common.txt")
}

func createContractCheckpoint(t *testing.T, repoPath, note string) string {
	t.Helper()
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", note)
	requireContractRawJSONOK(t, stdout, stderr, code, "checkpoint")
	id, _ := decodeContractSmokeDataMap(t, stdout)["checkpoint_id"].(string)
	if id == "" {
		t.Fatalf("checkpoint output missing checkpoint_id: %s", stdout)
	}
	return id
}

func createContractCheckpointWithTag(t *testing.T, repoPath, note, tag string) string {
	t.Helper()
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", note, "--tag", tag)
	requireContractRawJSONOK(t, stdout, stderr, code, "checkpoint")
	id, _ := decodeContractSmokeDataMap(t, stdout)["checkpoint_id"].(string)
	if id == "" {
		t.Fatalf("checkpoint output missing checkpoint_id: %s", stdout)
	}
	return id
}

func corruptContractDescriptor(t *testing.T, repoPath, checkpointID string) {
	t.Helper()
	path := filepath.Join(repoPath, ".jvs", "descriptors", checkpointID+".json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("corrupt descriptor %s: %v", path, err)
	}
}

func runContractJSON(t *testing.T, repoPath string, args ...string) contractCommandResult {
	t.Helper()
	allArgs := append([]string{"--json"}, args...)
	stdout, stderr, code := runJVSInRepo(t, repoPath, allArgs...)
	env := requireContractSingleJSONEnvelope(t, stdout, stderr, code)
	return contractCommandResult{stdout: stdout, stderr: stderr, code: code, env: env}
}

func requireContractRawJSONOK(t *testing.T, stdout, stderr string, code int, command string) {
	t.Helper()
	result := contractCommandResult{
		stdout: stdout,
		stderr: stderr,
		code:   code,
		env:    requireContractSingleJSONEnvelope(t, stdout, stderr, code),
	}
	requireContractJSONSuccess(t, result, command)
}

func requireContractSingleJSONEnvelope(t *testing.T, stdout, stderr string, code int) contractSmokeEnvelope {
	t.Helper()
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("JSON command wrote stderr: %q\nstdout=%s", stderr, stdout)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		t.Fatalf("JSON command produced empty stdout with code %d", code)
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	var env contractSmokeEnvelope
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, stdout)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout contains more than one JSON value: %v\n%s", err, stdout)
	}
	if env.OK && code != 0 {
		t.Fatalf("JSON envelope ok=true but exit code=%d\n%s", code, stdout)
	}
	if !env.OK && code == 0 {
		t.Fatalf("JSON envelope ok=false but exit code=0\n%s", stdout)
	}
	if env.OK && env.Error != nil {
		t.Fatalf("JSON success envelope error must be null: %#v\n%s", env.Error, stdout)
	}
	if !env.OK {
		if string(env.Data) != "null" {
			t.Fatalf("JSON failure envelope data must be null: %s", env.Data)
		}
		if _, ok := env.Error.(map[string]any); !ok {
			t.Fatalf("JSON failure envelope error must be an object: %#v\n%s", env.Error, stdout)
		}
	}
	return env
}

func requireContractJSONSuccess(t *testing.T, result contractCommandResult, command string) {
	t.Helper()
	if result.code != 0 || !result.env.OK {
		t.Fatalf("%s JSON command failed: code=%d stdout=%s stderr=%s", command, result.code, result.stdout, result.stderr)
	}
	if result.env.Command != command {
		t.Fatalf("JSON envelope command=%q, want %q\n%s", result.env.Command, command, result.stdout)
	}
}

func requireContractJSONFailure(t *testing.T, result contractCommandResult, command string) {
	t.Helper()
	if result.code == 0 || result.env.OK {
		t.Fatalf("%s JSON command unexpectedly succeeded: code=%d stdout=%s stderr=%s", command, result.code, result.stdout, result.stderr)
	}
	if result.env.Command != command {
		t.Fatalf("JSON envelope command=%q, want %q\n%s", result.env.Command, command, result.stdout)
	}
}

func requireContractErrorCode(t *testing.T, result contractCommandResult, want string) {
	t.Helper()
	errData := contractErrorMap(t, result)
	if got, _ := errData["code"].(string); got != want {
		t.Fatalf("error code=%q, want %q\n%#v\n%s", got, want, errData, result.stdout)
	}
}

func requireContractErrorContains(t *testing.T, result contractCommandResult, want string) {
	t.Helper()
	errData := contractErrorMap(t, result)
	message, _ := errData["message"].(string)
	hint, _ := errData["hint"].(string)
	if !strings.Contains(strings.ToLower(message+" "+hint), strings.ToLower(want)) {
		t.Fatalf("error message/hint missing %q: %#v\n%s", want, errData, result.stdout)
	}
}

func requireContractDataString(t *testing.T, result contractCommandResult, field, want string) {
	t.Helper()
	data := contractDataMap(t, result)
	if got, _ := data[field].(string); got != want {
		t.Fatalf("data[%s]=%q, want %q\n%s", field, got, want, result.stdout)
	}
}

func contractDataMap(t *testing.T, result contractCommandResult) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(result.env.Data, &data); err != nil {
		t.Fatalf("decode JSON data object: %v\n%s", err, result.stdout)
	}
	return data
}

func contractErrorMap(t *testing.T, result contractCommandResult) map[string]any {
	t.Helper()
	errData, ok := result.env.Error.(map[string]any)
	if !ok {
		t.Fatalf("JSON error is not an object: %#v\n%s", result.env.Error, result.stdout)
	}
	return errData
}

func requireContractChangePath(t *testing.T, data map[string]any, field, path string) {
	t.Helper()
	changes, ok := data[field].([]any)
	if !ok {
		t.Fatalf("diff JSON field %s is not an array: %#v", field, data[field])
	}
	for _, change := range changes {
		record, ok := change.(map[string]any)
		if ok && record["path"] == path {
			return
		}
	}
	t.Fatalf("diff JSON field %s missing path %q: %#v", field, path, changes)
}

func writeContractFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func requireContractFileContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("%s content=%q, want %q", path, string(content), want)
	}
}

func commonContractPrefix(a, b string) string {
	n := minContractInt(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	if i == 0 {
		return ""
	}
	return a[:i]
}

func minContractInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
