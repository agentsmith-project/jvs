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
