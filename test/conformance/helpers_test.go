//go:build conformance

package conformance

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	jvsBinary           string
	conformanceRepoRoot string
)

func TestMain(m *testing.M) {
	var err error
	conformanceRepoRoot, err = findConformanceRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find repository root: %v\n", err)
		os.Exit(1)
	}

	binDir, err := os.MkdirTemp("", "jvs-conformance-bin-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create conformance bin dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(binDir)

	jvsBinary = filepath.Join(binDir, "jvs")
	build := exec.Command("go", "build", "-o", jvsBinary, "./cmd/jvs")
	build.Dir = conformanceRepoRoot
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build current jvs binary for conformance: %v\n%s", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func findConformanceRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExistsNoTest(filepath.Join(cwd, "go.mod")) && fileExistsNoTest(filepath.Join(cwd, "cmd", "jvs")) {
			return cwd, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", fmt.Errorf("go.mod and cmd/jvs not found above %s", cwd)
		}
		cwd = parent
	}
}

func fileExistsNoTest(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func initTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	if stdout, stderr, code := runJVS(t, dir, "init", repoPath); code != 0 {
		t.Fatalf("init test repo failed: stdout=%s stderr=%s", stdout, stderr)
	}
	return repoPath, func() {}
}

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
	if err == nil {
		return stdout, stderr, 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return stdout, stderr, exitErr.ExitCode()
	}
	return stdout, stderr, 1
}

func runJVSInRepo(t *testing.T, repoPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runJVS(t, repoPath, args...)
}

func createFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for filename, content := range files {
		path := filepath.Join(root, filename)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create directory for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func readFile(t *testing.T, root, filename string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filename))
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	return string(content)
}
