//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func jvsJSON(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"--json"}, args...)
	stdout, stderr, code := runJVS(t, cwd, fullArgs...)
	if code != 0 {
		t.Fatalf("jvs --json %s failed in %s\nstdout=%s\nstderr=%s", strings.Join(args, " "), cwd, stdout, stderr)
	}
	requirePureJSONEnvelope(t, stdout, stderr, true)
	return stdout
}

func jvsJSONData(t *testing.T, cwd string, args ...string) map[string]any {
	t.Helper()
	return decodeContractDataMap(t, jvsJSON(t, cwd, args...))
}

func jvsJSONFrom(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"--json"}, args...)
	stdout, stderr, code := runJVS(t, cwd, fullArgs...)
	if code != 0 {
		t.Fatalf("jvs --json %s failed in %s\nstdout=%s\nstderr=%s", strings.Join(args, " "), cwd, stdout, stderr)
	}
	requirePureJSONEnvelope(t, stdout, stderr, true)
	return stdout
}

func savePointFromCWD(t *testing.T, cwd, message string) string {
	t.Helper()
	data := jvsJSONData(t, cwd, "save", "-m", message)
	id, _ := data["save_point_id"].(string)
	if id == "" {
		t.Fatalf("save output missing save_point_id: %#v", data)
	}
	return id
}

func requireDifferentSavePoints(t *testing.T, left, right string) {
	t.Helper()
	if left == "" || right == "" || left == right {
		t.Fatalf("save point IDs must be non-empty and different: left=%q right=%q", left, right)
	}
}

func requireHistoryIDs(t *testing.T, repoPath string, want []string) {
	t.Helper()
	requireHistoryIDsInCWD(t, repoPath, want)
}

func requireHistoryIDsInCWD(t *testing.T, cwd string, want []string) {
	t.Helper()
	got := savePointIDsFromHistory(t, jvsJSONData(t, cwd, "history"))
	if !sameStringSlice(got, want) {
		t.Fatalf("history IDs in %s = %v, want %v", cwd, got, want)
	}
}

func requireHistoryGrepIDs(t *testing.T, cwd, grep string, want []string) {
	t.Helper()
	got := savePointIDsFromHistory(t, jvsJSONData(t, cwd, "history", "--grep", grep))
	if !sameStringSlice(got, want) {
		t.Fatalf("history --grep %q IDs in %s = %v, want %v", grep, cwd, got, want)
	}
}

func savePointIDsFromHistory(t *testing.T, history map[string]any) []string {
	t.Helper()
	raw, ok := history["save_points"].([]any)
	if !ok {
		t.Fatalf("history save_points should be an array: %#v", history["save_points"])
	}
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		record, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("history save point should be an object: %#v", item)
		}
		id, _ := record["save_point_id"].(string)
		if id == "" {
			t.Fatalf("history save point missing save_point_id: %#v", record)
		}
		ids = append(ids, id)
	}
	return ids
}

func requireCandidateSavePoint(t *testing.T, raw any, want string) {
	t.Helper()
	candidates, ok := raw.([]any)
	if !ok {
		t.Fatalf("candidates should be an array: %#v", raw)
	}
	for _, item := range candidates {
		record, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("candidate should be an object: %#v", item)
		}
		if record["save_point_id"] == want {
			return
		}
	}
	t.Fatalf("candidate save point %s not found in %#v", want, candidates)
}

func requirePublicPathSource(t *testing.T, raw any, targetPath, sourceSavePoint string) {
	t.Helper()
	sources, ok := raw.([]any)
	if !ok {
		t.Fatalf("path_sources should be an array: %#v", raw)
	}
	for _, item := range sources {
		record, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("path source should be an object: %#v", item)
		}
		if record["target_path"] == targetPath && record["source_save_point"] == sourceSavePoint {
			return
		}
	}
	t.Fatalf("path source %s from %s not found in %#v", targetPath, sourceSavePoint, sources)
}

func requireDiscardUnsavedOption(t *testing.T, raw any) {
	t.Helper()
	options, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("restore options should be an object: %#v", raw)
	}
	if options["discard_unsaved"] != true {
		t.Fatalf("restore preview should record discard_unsaved safety choice: %#v", options)
	}
}

func requireSaveFirstOption(t *testing.T, raw any) {
	t.Helper()
	options, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("restore options should be an object: %#v", raw)
	}
	if options["save_first"] != true {
		t.Fatalf("restore preview should record save_first safety choice: %#v", options)
	}
}

func requireNoRunnableRestorePlan(t *testing.T, data map[string]any) {
	t.Helper()
	if planID, _ := data["plan_id"].(string); planID != "" {
		t.Fatalf("dirty decision preview should not create a runnable restore plan before a safety choice: %#v", data)
	}
	if runCommand, _ := data["run_command"].(string); runCommand != "" {
		t.Fatalf("dirty decision preview should not print a run command before a safety choice: %#v", data)
	}
}

func requireManagedFilesImpactAtLeast(t *testing.T, data map[string]any, bucket string, minCount int) {
	t.Helper()
	managedFiles, ok := data["managed_files"].(map[string]any)
	if !ok {
		t.Fatalf("restore preview should expose managed_files impact: %#v", data["managed_files"])
	}
	summary, ok := managedFiles[bucket].(map[string]any)
	if !ok {
		t.Fatalf("restore preview managed_files.%s should be an object: %#v", bucket, managedFiles[bucket])
	}
	count, ok := summary["count"].(float64)
	if !ok {
		t.Fatalf("restore preview managed_files.%s.count should be numeric: %#v", bucket, summary["count"])
	}
	if int(count) < minCount {
		t.Fatalf("restore preview managed_files.%s.count = %d, want at least %d in %#v", bucket, int(count), minCount, data)
	}
}

func requireHistoryRecordMessage(t *testing.T, history map[string]any, savePointID, wantMessage string) {
	t.Helper()
	raw, ok := history["save_points"].([]any)
	if !ok {
		t.Fatalf("history save_points should be an array: %#v", history["save_points"])
	}
	for _, item := range raw {
		record, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("history save point should be an object: %#v", item)
		}
		if record["save_point_id"] == savePointID {
			if record["message"] != wantMessage {
				t.Fatalf("history save point %s message = %#v, want %q in %#v", savePointID, record["message"], wantMessage, record)
			}
			return
		}
	}
	t.Fatalf("history missing save point %s in %#v", savePointID, raw)
}

func requireJSONStringArrayContains(t *testing.T, raw any, want string) {
	t.Helper()
	if !stringSliceContains(jsonStringArray(t, raw), want) {
		t.Fatalf("JSON string array missing %q: %#v", want, raw)
	}
}

func requireJSONStringArrayNotContains(t *testing.T, raw any, want string) {
	t.Helper()
	if stringSliceContains(jsonStringArray(t, raw), want) {
		t.Fatalf("JSON string array should not contain %q: %#v", want, raw)
	}
}

func cleanupProtectionSavePointsForReason(t *testing.T, preview map[string]any, reason string) []string {
	t.Helper()
	groups, ok := preview["protection_groups"].([]any)
	if !ok {
		t.Fatalf("cleanup preview protection_groups should be an array: %#v", preview["protection_groups"])
	}
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("cleanup protection group should be an object: %#v", item)
		}
		if group["reason"] == reason {
			return jsonStringArray(t, group["save_points"])
		}
	}
	return nil
}

func requireCleanupProtectionContainsSavePoint(t *testing.T, preview map[string]any, reason, savePointID string) {
	t.Helper()
	savePoints := cleanupProtectionSavePointsForReason(t, preview, reason)
	if !stringSliceContains(savePoints, savePointID) {
		t.Fatalf("cleanup protection reason %q missing save point %s: %#v", reason, savePointID, preview["protection_groups"])
	}
}

func requireCleanupProtectionOmitsSavePoint(t *testing.T, preview map[string]any, reason, savePointID string) {
	t.Helper()
	savePoints := cleanupProtectionSavePointsForReason(t, preview, reason)
	if stringSliceContains(savePoints, savePointID) {
		t.Fatalf("cleanup protection reason %q should not include save point %s: %#v", reason, savePointID, preview["protection_groups"])
	}
}

func restorePlanIDFromHumanPreview(t *testing.T, stdout string) string {
	t.Helper()
	match := regexp.MustCompile("(?m)^Plan: ([A-Za-z0-9._:-]+)$").FindStringSubmatch(stdout)
	if len(match) != 2 {
		t.Fatalf("restore preview missing Plan line:\n%s", stdout)
	}
	return match[1]
}

func readAbsoluteFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func removePath(t *testing.T, root, name string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
}

func requirePathMissing(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, name)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s should not exist", name)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", name, err)
	}
}

func requireAbsolutePathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s should exist: %v", path, err)
	}
}

func requireAbsolutePathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s should not exist", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func jsonStringArray(t *testing.T, raw any) []string {
	t.Helper()
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("value should be a JSON string array: %#v", raw)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("array item should be a string: %#v", item)
		}
		out = append(out, value)
	}
	return out
}

func sameCleanPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
