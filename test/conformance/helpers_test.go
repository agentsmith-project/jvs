//go:build conformance

package conformance

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var jvsBinary string

func init() {
	// Find the jvs binary
	cwd, _ := os.Getwd()
	// Walk up to find bin/jvs
	for {
		binPath := filepath.Join(cwd, "bin", "jvs")
		if _, err := os.Stat(binPath); err == nil {
			jvsBinary = binPath
			return
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	// Fallback to PATH
	jvsBinary = "jvs"
}

// initTestRepo creates a temp repo and returns its path and cleanup function.
func initTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")

	runJVS(t, dir, "init", "testrepo")

	cleanup := func() {
		// Temp dir is auto-cleaned by testing package
	}
	return repoPath, cleanup
}

// runJVS executes the jvs binary with args in the given working directory.
func runJVS(t *testing.T, cwd string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(jvsBinary, args...)
	cmd.Dir = cwd
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	} else {
		exitCode = 0
	}
	return
}

// runJVSInRepo runs jvs from within the repo's main worktree.
func runJVSInRepo(t *testing.T, repoPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cwd := filepath.Join(repoPath, "main")
	return runJVS(t, cwd, args...)
}

// runJVSInWorktree runs jvs from within a specific worktree.
func runJVSInWorktree(t *testing.T, repoPath, worktreeName string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var cwd string
	if worktreeName == "main" {
		cwd = filepath.Join(repoPath, "main")
	} else {
		cwd = filepath.Join(repoPath, "worktrees", worktreeName)
	}
	return runJVS(t, cwd, args...)
}

// createFiles creates multiple files in a worktree.
func createFiles(t *testing.T, worktreePath string, files map[string]string) {
	t.Helper()
	for filename, content := range files {
		path := filepath.Join(worktreePath, filename)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", path, err)
		}
	}
}

// readFile reads file content from a worktree.
func readFile(t *testing.T, worktreePath, filename string) string {
	t.Helper()
	path := filepath.Join(worktreePath, filename)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	return string(content)
}

// fileExists checks if a file exists.
func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("failed to stat file %s: %v", path, err)
	return false
}

// extractJSONField extracts a specific field from JSON output.
func extractJSONField(jsonOutput, field string) string {
	// Look for "field": "value" pattern
	search := `"` + field + `": "`
	start := bytes.Index([]byte(jsonOutput), []byte(search))
	if start == -1 {
		// Try without quotes (for numbers)
		searchAlt := `"` + field + `": `
		start = bytes.Index([]byte(jsonOutput), []byte(searchAlt))
		if start == -1 {
			return ""
		}
		start += len(searchAlt)
		// Find end (comma, newline, or closing brace)
		end := bytes.IndexAny([]byte(jsonOutput[start:]), ",}\n")
		if end == -1 {
			return ""
		}
		return string(bytes.TrimSpace([]byte(jsonOutput[start : start+end])))
	}
	start += len(search)
	end := bytes.Index([]byte(jsonOutput[start:]), []byte(`"`))
	if end == -1 {
		return ""
	}
	return jsonOutput[start : start+end]
}

// getSnapshotCount returns the number of checkpoints in list JSON output.
func getSnapshotCount(historyJSON string) int {
	return bytes.Count([]byte(historyJSON), []byte(`"checkpoint_id"`))
}

type checkpointListJSONRecord struct {
	Tags []string `json:"tags"`
}

func getCheckpointCountByTag(t *testing.T, checkpointListJSON, tag string) int {
	t.Helper()
	var envelope cliJSONEnvelope
	if err := json.Unmarshal([]byte(checkpointListJSON), &envelope); err != nil {
		t.Fatalf("decode checkpoint list envelope: %v\n%s", err, checkpointListJSON)
	}
	var records []checkpointListJSONRecord
	if err := json.Unmarshal(envelope.Data, &records); err != nil {
		t.Fatalf("decode checkpoint list data: %v\n%s", err, checkpointListJSON)
	}
	count := 0
	for _, record := range records {
		for _, got := range record.Tags {
			if got == tag {
				count++
				break
			}
		}
	}
	return count
}

type cliJSONEnvelope struct {
	Data json.RawMessage `json:"data"`
}

type workspaceStatusJSON struct {
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	AtLatest bool   `json:"at_latest"`
	Dirty    bool   `json:"dirty"`
}

func readWorkspaceStatus(t *testing.T, repoPath string) workspaceStatusJSON {
	t.Helper()
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "status")
	if code != 0 {
		t.Fatalf("status failed: %s", stderr)
	}

	var envelope cliJSONEnvelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode status envelope: %v\n%s", err, stdout)
	}

	var status workspaceStatusJSON
	if err := json.Unmarshal(envelope.Data, &status); err != nil {
		t.Fatalf("decode status data: %v\n%s", err, stdout)
	}
	return status
}

func requireWorkspaceCurrentLatest(t *testing.T, repoPath, current, latest string, atLatest bool) {
	t.Helper()
	status := readWorkspaceStatus(t, repoPath)
	if status.Current != current {
		t.Fatalf("expected current %s, got %s", current, status.Current)
	}
	if status.Latest != latest {
		t.Fatalf("expected latest %s, got %s", latest, status.Latest)
	}
	if status.AtLatest != atLatest {
		t.Fatalf("expected at_latest=%t, got %t", atLatest, status.AtLatest)
	}
}

// extractSnapshotIDByTag extracts a checkpoint ID that has a specific tag.
func extractSnapshotIDByTag(historyJSON, tag string) string {
	lines := bytes.Split([]byte(historyJSON), []byte("\n"))
	var currentSnapshotID string
	for _, line := range lines {
		if bytes.Contains(line, []byte(`"checkpoint_id"`)) {
			parts := bytes.Split(line, []byte(`"`))
			for i, p := range parts {
				if string(p) == "checkpoint_id" && i+2 < len(parts) {
					currentSnapshotID = string(parts[i+2])
				}
			}
		}
		// Check for the tag in the same block
		if bytes.Contains(line, []byte(`"tags"`)) && currentSnapshotID != "" {
			// Look ahead for the tag
			if bytes.Contains(line, []byte(tag)) {
				return currentSnapshotID
			}
		}
	}
	return ""
}

// waitForSnapshotReady waits for .READY marker to appear for a snapshot.
func waitForSnapshotReady(t *testing.T, repoPath, snapshotID string) {
	t.Helper()
	readyPath := filepath.Join(repoPath, ".jvs", "snapshots", snapshotID+".READY")
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(readyPath); err == nil {
			return
		}
	}
	t.Fatalf("timeout waiting for .READY marker for snapshot %s", snapshotID)
}
