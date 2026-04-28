//go:build conformance

package conformance

import (
	"strings"
	"testing"
)

func TestStoryLocal_RestorePreviewFirstKeepsFilesAndHistoryUntilRun(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"notebook.py": "print('baseline')\n"})
	baseline := savePoint(t, repoPath, "notebook baseline")
	createFiles(t, repoPath, map[string]string{"notebook.py": "print('run result')\n"})
	run := savePoint(t, repoPath, "notebook run")

	stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", baseline)
	if code != 0 {
		t.Fatalf("restore preview failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Preview only. No files were changed.") || !strings.Contains(stdout, "History will not change.") {
		t.Fatalf("human preview should explain preview-first and history stability:\n%s", stdout)
	}
	planID := restorePlanIDFromHumanPreview(t, stdout)
	if got := readFile(t, repoPath, "notebook.py"); got != "print('run result')\n" {
		t.Fatalf("human preview changed file: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{run, baseline})

	stdout, stderr, code = runJVSInRepo(t, repoPath, "restore", "--run", planID)
	if code != 0 {
		t.Fatalf("restore run failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Restored save point: "+baseline) || !strings.Contains(stdout, "History was not changed.") {
		t.Fatalf("human restore run should report restored save point and stable history:\n%s", stdout)
	}
	if got := readFile(t, repoPath, "notebook.py"); got != "print('baseline')\n" {
		t.Fatalf("human restore run file = %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{run, baseline})
}
