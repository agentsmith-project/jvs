//go:build conformance

package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type contractSmokeEnvelope struct {
	SchemaVersion int                 `json:"schema_version"`
	Command       string              `json:"command"`
	OK            bool                `json:"ok"`
	RepoRoot      *string             `json:"repo_root"`
	Workspace     *string             `json:"workspace"`
	Data          json.RawMessage     `json:"data"`
	Error         *contractSmokeError `json:"error"`
}

type contractSmokeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

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

	if binary := os.Getenv("JVS_BINARY_UNDER_TEST"); binary != "" {
		if !filepath.IsAbs(binary) {
			fmt.Fprintf(os.Stderr, "JVS_BINARY_UNDER_TEST must be an absolute path, got %q\n", binary)
			os.Exit(1)
		}
		info, err := os.Stat(binary)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat JVS_BINARY_UNDER_TEST %s: %v\n", binary, err)
			os.Exit(1)
		}
		if info.IsDir() {
			fmt.Fprintf(os.Stderr, "JVS_BINARY_UNDER_TEST %s is a directory, want executable file\n", binary)
			os.Exit(1)
		}
		jvsBinary = binary
		os.Exit(m.Run())
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

func requirePureJSONEnvelope(t *testing.T, stdout, stderr string, wantOK bool) contractSmokeEnvelope {
	t.Helper()
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("JSON command wrote stderr: %q", stderr)
	}
	if !json.Valid([]byte(stdout)) {
		t.Fatalf("stdout is not pure JSON: %q", stdout)
	}
	env := decodeContractEnvelope(t, stdout)
	if env.OK != wantOK {
		t.Fatalf("JSON envelope ok = %t, want %t: %s", env.OK, wantOK, stdout)
	}
	return env
}

func decodeContractEnvelope(t *testing.T, stdout string) contractSmokeEnvelope {
	t.Helper()
	var env contractSmokeEnvelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("decode JSON envelope: %v\n%s", err, stdout)
	}
	if env.SchemaVersion == 0 {
		t.Fatalf("JSON envelope missing schema_version: %s", stdout)
	}
	return env
}

func decodeContractDataMap(t *testing.T, stdout string) map[string]any {
	t.Helper()
	env := requirePureJSONEnvelope(t, stdout, "", true)
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode JSON envelope data object: %v\n%s", err, stdout)
	}
	return data
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
